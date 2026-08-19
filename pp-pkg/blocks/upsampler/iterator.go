package upsampler

import (
	"math"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

const (
	// synthesisStepDivisor makes the synthesis step half of the function range, so that
	// any window of that width holds at least two samples regardless of alignment.
	synthesisStepDivisor = 2
	// maxGapResolutions limits, in source resolutions, how wide a real gap may be to
	// still be interpolated. A gap of one resolution is what decimation itself leaves;
	// beyond maxGapResolutions the data is genuinely missing, and filling it in would
	// hide the outage instead of restoring dropped points.
	maxGapResolutions = 2
)

// Iterator wraps a [chunkenc.Iterator] and injects synthetic samples via linear
// interpolation between two real samples when the gap between them exceeds stepMS
// but stays within maxGapMS. Wider gaps are passed through untouched.
type Iterator struct {
	base chunkenc.Iterator

	// Anchor pair of real samples and state.
	t0, t1     int64
	v0, v1     float64
	nextSynthT int64 // next synthetic sample time, or 0 if no synthesis in progress

	// Both thresholds are durations, not timestamps, so 32 bits are enough for any
	// sane range/resolution and keep the iterator one word smaller.
	stepMS   uint32 // synthesis step and lower gap threshold: rangeMS / 2
	maxGapMS uint32 // upper gap threshold: resolutionMS * maxGapResolutions

	counterFunc bool // the query function reads the series as a counter
	haveT1      bool // t1/v1 already read from base, not yet yielded
	initialized bool // first call to Next/Seek
}

// NewIterator wraps a base [chunkenc.Iterator] for interpolation of gaps wider than
// stepMS and no wider than maxGapMS, both produced by [gapThresholds]. counterFunc keeps
// a value drop inside a gap flat, see [Iterator.synthesizeAt].
func NewIterator(base chunkenc.Iterator, stepMS, maxGapMS uint32, counterFunc bool) *Iterator {
	it := &Iterator{}
	it.Reset(base, stepMS, maxGapMS, counterFunc)

	return it
}

// At returns the current timestamp/value pair for float samples.
func (it *Iterator) At() (int64, float64) {
	return it.t0, it.v0
}

// AtT returns the current timestamp.
func (it *Iterator) AtT() int64 {
	return it.t0
}

// AtFloatHistogram returns nil — we don't synthesize histograms in v1.
func (*Iterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return 0, nil
}

// AtHistogram returns nil — we don't synthesize histograms in v1.
func (*Iterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	return 0, nil
}

// Err returns any accumulated error.
func (it *Iterator) Err() error {
	return it.base.Err()
}

// Next advances the iterator by one and returns the type of the value.
// Implements the state machine:
//  1. If synthetic samples remain, yield the next one.
//  2. Else if a buffered real sample exists, yield it and advance anchor.
//  3. Else read the next real sample from base and decide: synthesize or yield directly.
func (it *Iterator) Next() chunkenc.ValueType {
	// Step 1: yield synthetic sample if in progress.
	if it.nextSynthT > 0 && it.nextSynthT < it.t1 {
		vt := it.synthesizeAt(it.nextSynthT)
		it.nextSynthT += int64(it.stepMS)
		return vt
	}

	// Step 2: yield buffered real sample if exists.
	if it.haveT1 {
		it.yieldT1()
		return chunkenc.ValFloat
	}

	// Step 3: read next sample from base.
	valType := it.base.Next()
	if valType == chunkenc.ValNone || it.base.Err() != nil {
		return chunkenc.ValNone
	}

	t, v := it.base.At()

	// On first real sample, just anchor t0.
	if !it.initialized {
		it.t0, it.v0 = t, v
		it.initialized = true
		return valType
	}

	it.t1, it.v1 = t, v

	// Non-float samples (histograms): pass through without synthesis.
	if valType != chunkenc.ValFloat {
		it.t0, it.v0 = it.t1, it.v1
		it.nextSynthT = 0
		return valType
	}

	// Float samples: check gap and decide.
	it.armSynthesis()
	if it.nextSynthT > 0 {
		it.haveT1 = true
		return it.Next() // Yield first synthetic sample.
	}

	// Nothing to interpolate: yield t1 directly.
	it.t0, it.v0 = it.t1, it.v1
	return chunkenc.ValFloat
}

// Seek advances the iterator forward to the first sample with timestamp >= t.
// Does NOT delegate to base.Seek() except on the very first call, because
// chunkenc.Iterator doesn't provide a way to retrieve the sample before the
// sought position, which we need to anchor (t0, v0) for interpolation.
func (it *Iterator) Seek(target int64) chunkenc.ValueType {
	// First call: use Next() to initialize with proper anchor pair.
	if !it.initialized {
		for {
			valType := it.Next()
			if valType == chunkenc.ValNone {
				return chunkenc.ValNone
			}
			if it.t0 >= target {
				return valType
			}
		}
	}

	// After initialization: check if target is already covered by current state.
	// (t0, t1] range covers our current position through the next real sample.
	if target <= it.t0 {
		// Target is before or at the current point; iterate through synthetic samples.
		return it.seekWithinState(target)
	}

	// if t1 not initialized, try to initialize
	if !it.haveT1 {
		valType := it.base.Next()
		if valType == chunkenc.ValNone {
			return chunkenc.ValNone
		}

		it.t1, it.v1 = it.base.At()
		it.haveT1 = true
		it.armSynthesis()
	}

	if target <= it.t1 {
		// Target is within the gap (or at the next real point): walk the synthetic
		// queue, falling back to the buffered real sample when there is none.
		return it.seekWithinState(target)
	}

	// target > t1: sequentially read from base until we find it.
	return it.seekAdvanceBase(target)
}

// seekWithinState iterates through the current state (synthetic or buffered real)
// without touching base, returning the first sample with ts >= target.
func (it *Iterator) seekWithinState(target int64) chunkenc.ValueType {
	// If target is at or before current t0, return t0.
	if target <= it.t0 {
		return chunkenc.ValFloat
	}

	for {
		// Try to yield synthetic sample if in progress.
		if it.nextSynthT > 0 && it.nextSynthT < it.t1 {
			if it.nextSynthT >= target {
				vt := it.synthesizeAt(it.nextSynthT)
				it.nextSynthT += int64(it.stepMS)
				return vt
			}

			it.nextSynthT += int64(it.stepMS)

			continue
		}

		// Try to yield buffered real sample.
		// Note: seekWithinState is only called with target <= t1 (or target <= t0).
		// Since target <= t1 and we only reach here if target > t0,
		// we have t0 < target <= t1. So t1 >= target is always true.
		if it.haveT1 {
			it.yieldT1()
			return chunkenc.ValFloat
		}

		// Unreachable: seekWithinState is only called with a buffered t1,
		// so we always return above. Return as safeguard.
		return chunkenc.ValNone
	}
}

// seekAdvanceBase reads sequentially from base until we find target.
func (it *Iterator) seekAdvanceBase(target int64) chunkenc.ValueType {
	for {
		valType := it.base.Next()
		if valType == chunkenc.ValNone {
			return chunkenc.ValNone
		}

		t, v := it.base.At()

		// Advance anchor.
		it.t0, it.v0 = it.t1, it.v1
		it.t1, it.v1 = t, v

		if t > target {
			it.haveT1 = true
			it.armSynthesis()
			return it.seekWithinState(target)
		}
	}
}

// armSynthesis schedules synthetic samples between the current anchor pair when the
// gap between them leaves windows of rangeMS with less than two samples, yet is narrow
// enough to be explained by decimation of the source. Otherwise synthesis is disarmed
// and the pair is yielded as is.
func (it *Iterator) armSynthesis() {
	if gap := it.t1 - it.t0; gap <= int64(it.stepMS) || gap > int64(it.maxGapMS) {
		it.nextSynthT = 0
		return
	}

	it.nextSynthT = it.t0 + int64(it.stepMS)
}

// yieldT1 moves the anchor onto the buffered real sample and drops synthesis state.
func (it *Iterator) yieldT1() {
	it.t0, it.v0 = it.t1, it.v1
	it.haveT1 = false
	it.nextSynthT = 0
}

// synthesizeAt returns ValFloat for a synthetic sample at time t,
// with value interpolated linearly from (t0, v0) to (t1, v1).
func (it *Iterator) synthesizeAt(t int64) chunkenc.ValueType {
	// Linear interpolation: v = v0 + (v1 - v0) * (t - t0) / (t1 - t0)
	dt := float64(t - it.t0)
	dv := it.v1 - it.v0
	dT := float64(it.t1 - it.t0)
	// For a counter function a drop across the gap is a counter reset that decimation hid.
	// Interpolating down to it would decrease on every synthetic step, so the function would
	// see as many resets as there are synthetic samples and correct for each of them. Holding
	// the last known value instead keeps the single reset on the step where the real sample
	// proves it. For gauge functions a decrease is ordinary data and is interpolated as is.
	if it.counterFunc && dv < 0 {
		dv = 0
	}

	interpolated := it.v0 + dv*(dt/dT)
	it.t0 = t
	it.v0 = interpolated

	return chunkenc.ValFloat
}

// Reset resets the iterator to a clean state. Used by Series.Iterator()
// to reuse the same Iterator across multiple calls.
func (it *Iterator) Reset(base chunkenc.Iterator, stepMS, maxGapMS uint32, counterFunc bool) {
	it.base = base
	it.stepMS = stepMS
	it.maxGapMS = maxGapMS
	it.counterFunc = counterFunc

	it.t0 = 0
	it.t1 = 0
	it.v0 = 0
	it.v1 = 0
	it.haveT1 = false
	it.nextSynthT = 0
	it.initialized = false
}

// gapThresholds converts the millisecond query parameters into the pair of gap thresholds
// the iterator works with: the synthesis step and the widest gap still interpolated. They
// are 32-bit because both are durations, not timestamps. Parameters that don't fit — a range
// above ~99 days or a resolution above ~24 days — yield zeros, which disarms synthesis
// instead of wrapping around into a step that means nothing.
func gapThresholds(rangeMS, resolutionMS int64) (stepMS, maxGapMS uint32) {
	step := rangeMS / synthesisStepDivisor
	maxGap := resolutionMS * maxGapResolutions
	if step < 0 || step > math.MaxUint32 || maxGap < 0 || maxGap > math.MaxUint32 {
		return 0, 0
	}

	return uint32(step), uint32(maxGap) // #nosec G115 // both values are range-checked right above
}
