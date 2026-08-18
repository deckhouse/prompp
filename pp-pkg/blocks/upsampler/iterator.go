package upsampler

import (
	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// Iterator wraps a [chunkenc.Iterator] and injects synthetic samples via linear
// interpolation between real samples when the gap between them exceeds rangeMS.
type Iterator struct {
	base    chunkenc.Iterator
	rangeMS int64

	// Anchor pair of real samples and state.
	t0, t1     int64
	v0, v1     float64
	step       int64 // rangeMS / 2
	nextSynthT int64 // next synthetic sample time, or 0 if no synthesis in progress

	haveT1      bool // t1/v1 already read from base, not yet yielded
	initialized bool // first call to Next/Seek
}

// NewIterator wraps a base [chunkenc.Iterator] for interpolation when gaps exceed rangeMS.
func NewIterator(base chunkenc.Iterator, rangeMS int64) *Iterator {
	return &Iterator{
		base:    base,
		rangeMS: rangeMS,
		//revive:disable-next-line:add-constant // half the range is a reasonable default step for interpolation
		step: rangeMS / 2,
	}
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
		it.nextSynthT += it.step
		return vt
	}

	// Step 2: yield buffered real sample if exists.
	if it.haveT1 {
		it.t0, it.v0 = it.t1, it.v1
		it.haveT1 = false
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
		return valType
	}

	// Float samples: check gap and decide.
	gap := it.t1 - it.t0
	if gap > it.step {
		// Gap exceeds step: synthesize.
		it.nextSynthT = it.t0 + it.step
		it.haveT1 = true
		return it.Next() // Yield first synthetic sample.
	}

	// Gap is acceptable: yield t1 directly.
	it.t0, it.v0 = it.t1, it.v1
	it.nextSynthT = it.t0 + it.step
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
		it.nextSynthT = it.t0 + it.step
	}

	if target <= it.t1 {
		// Target is within the gap (or at the next real point).
		// If we're in synthetic mode, iterate forward.
		if it.nextSynthT > 0 {
			return it.seekWithinState(target)
		}
		// Note: if nextSynthT==0 and target<=t1, then t0==t1 (no synthesis, gap < rangeMS).
		// In this case, t0 >= target, so we return the current t0.
		// However, after seekWithinState call, we always return, so we never reach here
		// in normal flow. This path is logically unreachable.
		// fallthrough to seekAdvanceBase as safety.
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
				it.nextSynthT += it.step
				return vt
			}

			it.nextSynthT += it.step

			continue
		}

		// Try to yield buffered real sample.
		// Note: seekWithinState is only called with target <= t1 (or target <= t0).
		// Since target <= t1 and we only reach here if target > t0,
		// we have t0 < target <= t1. So t1 >= target is always true.
		if it.haveT1 {
			it.t0, it.v0 = it.t1, it.v1
			it.haveT1 = false
			return chunkenc.ValFloat
		}

		// Unreachable: seekWithinState is only called with nextSynthT > 0,
		// which guarantees haveT1 == true, so we always return above.
		// Return as safeguard.
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
			it.nextSynthT = it.t0 + it.step
			return it.seekWithinState(target)
		}
	}
}

// synthesizeAt returns ValFloat for a synthetic sample at time t,
// with value interpolated linearly from (t0, v0) to (t1, v1).
func (it *Iterator) synthesizeAt(t int64) chunkenc.ValueType {
	// Linear interpolation: v = v0 + (v1 - v0) * (t - t0) / (t1 - t0)
	dt := float64(t - it.t0)
	dv := it.v1 - it.v0
	dT := float64(it.t1 - it.t0)

	interpolated := it.v0 + dv*(dt/dT)
	it.t0 = t
	it.v0 = interpolated

	return chunkenc.ValFloat
}

// Reset resets the iterator to a clean state. Used by Series.Iterator()
// to reuse the same Iterator across multiple calls.
func (it *Iterator) Reset(base chunkenc.Iterator, rangeMS int64) {
	it.base = base
	it.rangeMS = rangeMS
	//revive:disable-next-line:add-constant // half the range is a reasonable default step for interpolation
	it.step = rangeMS / 2

	it.t0 = 0
	it.t1 = 0
	it.v0 = 0
	it.v1 = 0
	it.haveT1 = false
	it.nextSynthT = 0
	it.initialized = false
}
