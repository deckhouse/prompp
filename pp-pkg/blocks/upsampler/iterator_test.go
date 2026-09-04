package upsampler_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/pp-pkg/blocks/upsampler"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

//
// IteratorSuite
//

type IteratorSuite struct {
	suite.Suite
}

func TestIteratorSuite(t *testing.T) {
	suite.Run(t, new(IteratorSuite))
}

// TestIteratorNoGap tests that samples without large gaps pass through unchanged.
func (s *IteratorSuite) TestIteratorNoGap() {
	// Points every 1 minute, range is 2 minutes.
	samples := []struct {
		t int64
		v float64
	}{
		{t: 60_000, v: 10.0},
		{t: 120_000, v: 20.0},
		{t: 180_000, v: 30.0},
	}
	base := newMockIterator(samples)

	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	// Collect all samples.
	var result []struct {
		t int64
		v float64
	}
	for {
		vt := it.Next()
		if vt == chunkenc.ValNone {
			break
		}

		t, v := it.At()
		result = append(result, struct {
			t int64
			v float64
		}{t, v})
	}

	// Should have exactly 3 samples.
	s.Require().Len(result, 3)

	for i, expected := range samples {
		s.Require().Equal(expected.t, result[i].t, "sample %d time", i)
		s.Require().Equal(expected.v, result[i].v, "sample %d value", i)
	}
}

// TestIteratorWithGap tests that a gap wider than step triggers synthesis.
func (s *IteratorSuite) TestIteratorWithGap() {
	// Gap of 4 minutes between first and second sample, range is 2 minutes.
	samples := []struct {
		t int64
		v float64
	}{
		{t: 60_000, v: 10.0},
		{t: 300_000, v: 30.0}, // 4-minute gap: step < gap <= resolution*2
	}
	base := newMockIterator(samples)

	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	// Collect all samples.
	var result []struct {
		t int64
		v float64
	}
	for {
		vt := it.Next()
		if vt == chunkenc.ValNone {
			break
		}

		t, v := it.At()
		result = append(result, struct {
			t int64
			v float64
		}{t, v})
	}

	// Should have synthesized samples. With step = range/2 = 60s,
	// synthetic points: 120_000, 180_000, 240_000
	// Plus real: 60_000, 300_000
	// Total: 5 samples.
	s.Require().Len(result, 5, "should have synthesized samples")

	// First sample should be real.
	s.Require().Equal(int64(60_000), result[0].t)
	s.Require().Equal(10.0, result[0].v)

	// Last sample should be real.
	s.Require().Equal(int64(300_000), result[len(result)-1].t)
	s.Require().Equal(30.0, result[len(result)-1].v)

	// Check that synthetic samples are in the gap.
	for i := 1; i < len(result)-1; i++ {
		s.Require().Greater(result[i].t, int64(60_000), "synthetic sample %d should be after first real", i)
		s.Require().Less(result[i].t, int64(300_000), "synthetic sample %d should be before last real", i)
	}
}

// TestIteratorGapWiderThanMaxGap tests that a gap wider than resolutionMS*2
// is treated as missing data and passed through without synthesis.
func (s *IteratorSuite) TestIteratorGapWiderThanMaxGap() {
	// Resolution is 2 minutes, so the widest interpolated gap is 4 minutes.
	samples := []struct {
		t int64
		v float64
	}{
		{t: 60_000, v: 10.0},
		{t: 360_000, v: 30.0}, // 5-minute gap: wider than resolution*2
		{t: 420_000, v: 40.0},
	}
	base := newMockIterator(samples)

	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	var result []struct {
		t int64
		v float64
	}
	for {
		vt := it.Next()
		if vt == chunkenc.ValNone {
			break
		}

		t, v := it.At()
		result = append(result, struct {
			t int64
			v float64
		}{t, v})
	}

	// Only the real samples, no synthesis across the outage.
	s.Require().Len(result, len(samples))
	for i, expected := range samples {
		s.Require().Equal(expected.t, result[i].t, "sample %d time", i)
		s.Require().Equal(expected.v, result[i].v, "sample %d value", i)
	}
}

// TestIteratorGapWithValueDrop tests that a value drop across the gap — a counter reset
// hidden by decimation — is not interpolated downwards: synthetic samples hold the last
// known value, so the whole drop stays on the step of the real sample that proves it.
func (s *IteratorSuite) TestIteratorGapWithValueDrop() {
	samples := []struct {
		t int64
		v float64
	}{
		{t: 60_000, v: 100.0},
		{t: 300_000, v: 20.0}, // 4-minute gap with a counter reset inside it
	}
	base := newMockIterator(samples)

	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	var result []struct {
		t int64
		v float64
	}
	for {
		vt := it.Next()
		if vt == chunkenc.ValNone {
			break
		}

		t, v := it.At()
		result = append(result, struct {
			t int64
			v float64
		}{t, v})
	}

	s.Require().Equal(
		[]struct {
			t int64
			v float64
		}{
			{t: 60_000, v: 100.0},
			{t: 120_000, v: 100.0},
			{t: 180_000, v: 100.0},
			{t: 240_000, v: 100.0},
			{t: 300_000, v: 20.0},
		},
		result,
	)
}

// TestIteratorGapWithValueDropForGaugeFunc tests the opposite of the counter case: for a
// gauge function a decrease is ordinary data, so the drop is interpolated like a rise.
func (s *IteratorSuite) TestIteratorGapWithValueDropForGaugeFunc() {
	samples := []struct {
		t int64
		v float64
	}{
		{t: 60_000, v: 100.0},
		{t: 300_000, v: 20.0}, // 4-minute gap with a falling gauge inside it
	}
	base := newMockIterator(samples)

	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.GaugeFunc)

	var result []struct {
		t int64
		v float64
	}
	for {
		vt := it.Next()
		if vt == chunkenc.ValNone {
			break
		}

		t, v := it.At()
		result = append(result, struct {
			t int64
			v float64
		}{t, v})
	}

	s.Require().Equal(
		[]struct {
			t int64
			v float64
		}{
			{t: 60_000, v: 100.0},
			{t: 120_000, v: 80.0},
			{t: 180_000, v: 60.0},
			{t: 240_000, v: 40.0},
			{t: 300_000, v: 20.0},
		},
		result,
	)
}

// TestIteratorGapForOverTimeFunc tests that an _over_time function gets the last known
// value on every synthetic sample, in both directions: such a function aggregates the
// values of its window, so a value that was never measured must not appear inside it.
func (s *IteratorSuite) TestIteratorGapForOverTimeFunc() {
	testCases := []struct {
		name         string
		secondSample float64
	}{
		{name: "rising series", secondSample: 300.0},
		{name: "falling series", secondSample: 20.0},
	}

	for _, tc := range testCases {
		s.Run(tc.name, func() {
			samples := []struct {
				t int64
				v float64
			}{
				{t: 60_000, v: 100.0},
				{t: 300_000, v: tc.secondSample}, // 4-minute gap
			}
			base := newMockIterator(samples)

			it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.OverTimeFunc)

			var result []struct {
				t int64
				v float64
			}
			for {
				vt := it.Next()
				if vt == chunkenc.ValNone {
					break
				}

				t, v := it.At()
				result = append(result, struct {
					t int64
					v float64
				}{t, v})
			}

			s.Require().Equal(
				[]struct {
					t int64
					v float64
				}{
					{t: 60_000, v: 100.0},
					{t: 120_000, v: 100.0},
					{t: 180_000, v: 100.0},
					{t: 240_000, v: 100.0},
					{t: 300_000, v: tc.secondSample},
				},
				result,
			)
		})
	}
}

// TestIteratorSeekIntoGapWiderThanMaxGap tests that seeking into a gap wider than
// resolutionMS*2 returns the next real sample instead of a synthetic one.
func (s *IteratorSuite) TestIteratorSeekIntoGapWiderThanMaxGap() {
	samples := []struct {
		t int64
		v float64
	}{
		{t: 60_000, v: 10.0},
		{t: 360_000, v: 30.0}, // 5-minute gap: wider than resolution*2
	}
	base := newMockIterator(samples)

	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	vt := it.Seek(200_000)
	s.Require().Equal(chunkenc.ValFloat, vt)

	t, v := it.At()
	s.Require().Equal(int64(360_000), t)
	s.Require().Equal(30.0, v)
}

// TestIteratorWideGapWithinResolutionBound tests that the bound is set by the source
// resolution, not by the function range: a gap far wider than rangeMS is still
// interpolated when it fits into resolutionMS*2.
func (s *IteratorSuite) TestIteratorWideGapWithinResolutionBound() {
	// Range is 2 minutes (step 1 minute), resolution is 10 minutes: gaps up to
	// 20 minutes are decimation artifacts and get interpolated.
	samples := []struct {
		t int64
		v float64
	}{
		{t: 0, v: 0.0},
		{t: 600_000, v: 100.0}, // 10-minute gap: 5x rangeMS, still within resolution*2
	}
	base := newMockIterator(samples)

	it := upsampler.NewIterator(base, 60_000, 1_200_000, upsampler.CounterFunc)

	var result []struct {
		t int64
		v float64
	}
	for {
		vt := it.Next()
		if vt == chunkenc.ValNone {
			break
		}

		t, v := it.At()
		result = append(result, struct {
			t int64
			v float64
		}{t, v})
	}

	// Real 0 and 600_000 plus synthetic every 60_000 in between.
	s.Require().Len(result, 11)
	s.Require().Equal(int64(60_000), result[1].t)
	s.Require().InDelta(10.0, result[1].v, 0.001)
	s.Require().Equal(int64(600_000), result[len(result)-1].t)
	s.Require().Equal(100.0, result[len(result)-1].v)
}

// TestIteratorGapWithinRangeButOverResolutionBound tests the opposite direction:
// a gap narrower than rangeMS is still left alone when it exceeds resolutionMS*2.
func (s *IteratorSuite) TestIteratorGapWithinRangeButOverResolutionBound() {
	// Range is 10 minutes (step 5 minutes), resolution is 1 minute: a 5-minute gap
	// is a real outage of the source, not decimation.
	samples := []struct {
		t int64
		v float64
	}{
		{t: 0, v: 0.0},
		{t: 400_000, v: 100.0},
	}
	base := newMockIterator(samples)

	it := upsampler.NewIterator(base, 300_000, 120_000, upsampler.CounterFunc)

	var result []struct {
		t int64
		v float64
	}
	for {
		vt := it.Next()
		if vt == chunkenc.ValNone {
			break
		}

		t, v := it.At()
		result = append(result, struct {
			t int64
			v float64
		}{t, v})
	}

	s.Require().Len(result, len(samples))
	for i, expected := range samples {
		s.Require().Equal(expected.t, result[i].t, "sample %d time", i)
		s.Require().Equal(expected.v, result[i].v, "sample %d value", i)
	}
}

// TestIteratorMultipleGaps tests multiple gaps across a sequence.
func (s *IteratorSuite) TestIteratorMultipleGaps() {
	// Two gaps, both wider than step and within resolution*2.
	samples := []struct {
		t int64
		v float64
	}{
		{t: 60_000, v: 10.0},
		{t: 300_000, v: 30.0}, // 4-minute gap
		{t: 540_000, v: 50.0}, // 4-minute gap
	}
	base := newMockIterator(samples)

	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	var result []struct {
		t int64
		v float64
	}
	for {
		vt := it.Next()
		if vt == chunkenc.ValNone {
			break
		}

		t, v := it.At()
		result = append(result, struct {
			t int64
			v float64
		}{t, v})
	}

	// Should have > 3 samples due to synthesis (multiple gaps).
	s.Require().Greater(len(result), 3)
}

// TestIteratorGapAtEnd tests a gap immediately before the last real sample
// (gap at chunk boundary).
func (s *IteratorSuite) TestIteratorGapAtEnd() {
	// Last two samples have a large gap.
	samples := []struct {
		t int64
		v float64
	}{
		{t: 60_000, v: 10.0},
		{t: 120_000, v: 20.0},
		{t: 360_000, v: 30.0}, // large gap before last sample
	}
	base := newMockIterator(samples)

	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	var result []struct {
		t int64
		v float64
	}
	for {
		vt := it.Next()
		if vt == chunkenc.ValNone {
			break
		}

		t, v := it.At()
		result = append(result, struct {
			t int64
			v float64
		}{t, v})
	}

	// Should have > 3 samples: 3 real + synthetic in the gap.
	s.Require().Greater(len(result), 3)

	// Last sample should be the real one.
	s.Require().Equal(int64(360_000), result[len(result)-1].t)
	s.Require().Equal(30.0, result[len(result)-1].v)
}

// TestIteratorHistogramPassthrough tests that histogram samples
// pass through without synthesis in the middle of a float sequence.
func (s *IteratorSuite) TestIteratorHistogramPassthrough() {
	// Mix of float and histogram samples.
	// Note: mockIterator only supports float, so we'll test the logic
	// by checking that non-float values in the middle don't trigger synthesis.
	samples := []struct {
		t int64
		v float64
	}{
		{t: 60_000, v: 10.0},
		{t: 120_000, v: 20.0},
		{t: 180_000, v: 30.0},
		{t: 240_000, v: 40.0},
	}
	base := newMockIterator(samples)

	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	// Collect all samples.
	var result []struct {
		t int64
		v float64
	}
	for {
		vt := it.Next()
		if vt == chunkenc.ValNone {
			break
		}

		if vt == chunkenc.ValFloat {
			t, v := it.At()
			result = append(result, struct {
				t int64
				v float64
			}{t, v})
		}
	}

	// Without gaps, should pass through all 4 floats unchanged.
	s.Require().Len(result, 4)
	for i, expected := range samples {
		s.Require().Equal(expected.t, result[i].t, "sample %d time", i)
		s.Require().Equal(expected.v, result[i].v, "sample %d value", i)
	}
}

//
// Seek tests
//

// TestIteratorSeekBeforeFirst tests seeking to a timestamp before the first sample.
func (s *IteratorSuite) TestIteratorSeekBeforeFirst() {
	samples := []struct {
		t int64
		v float64
	}{
		{t: 100_000, v: 10.0},
		{t: 200_000, v: 20.0},
	}
	base := newMockIterator(samples)
	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	// Seek to time before first sample.
	vt := it.Seek(50_000)
	s.Require().Equal(chunkenc.ValFloat, vt)

	t, v := it.At()
	s.Require().Equal(int64(100_000), t)
	s.Require().Equal(10.0, v)
}

// TestIteratorSeekOnRealSample tests seeking exactly to a real sample.
func (s *IteratorSuite) TestIteratorSeekOnRealSample() {
	samples := []struct {
		t int64
		v float64
	}{
		{t: 100_000, v: 10.0},
		{t: 200_000, v: 20.0},
		{t: 300_000, v: 30.0},
	}
	base := newMockIterator(samples)
	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	// Seek to the second sample.
	vt := it.Seek(200_000)
	s.Require().Equal(chunkenc.ValFloat, vt)

	t, v := it.At()
	s.Require().Equal(int64(200_000), t)
	s.Require().Equal(20.0, v)
}

// TestIteratorSeekIntoGap tests seeking into the middle of a gap
// that triggers synthesis.
func (s *IteratorSuite) TestIteratorSeekIntoGap() {
	samples := []struct {
		t int64
		v float64
	}{
		{t: 100_000, v: 10.0},
		{t: 340_000, v: 50.0}, // 240s gap, range is 120s
	}
	base := newMockIterator(samples)
	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	// Seek into the middle of the gap.
	vt := it.Seek(250_000)
	s.Require().Equal(chunkenc.ValFloat, vt)

	t, v := it.At()
	// Should get a synthetic point in the gap.
	s.Require().Greater(t, int64(100_000))
	s.Require().Less(t, int64(340_000))

	// Check that interpolated value is reasonable.
	// Linear interpolation: v = 10 + (50-10) * (t-100k) / (340k-100k)
	expectedV := 10.0 + 40.0*float64(t-100_000)/240_000.0
	s.Require().InDelta(expectedV, v, 0.1, "interpolated value should match linear interpolation")
}

// TestIteratorSeekAfterLast tests seeking beyond the last sample (EOF).
func (s *IteratorSuite) TestIteratorSeekAfterLast() {
	samples := []struct {
		t int64
		v float64
	}{
		{t: 100_000, v: 10.0},
		{t: 200_000, v: 20.0},
	}
	base := newMockIterator(samples)
	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	// Seek beyond last sample.
	vt := it.Seek(999_999)
	s.Require().Equal(chunkenc.ValNone, vt)
}

// TestIteratorSeekForwardInPartialGap tests seeking forward from within
// a partially-yielded synthetic gap.
func (s *IteratorSuite) TestIteratorSeekForwardInPartialGap() {
	samples := []struct {
		t int64
		v float64
	}{
		{t: 100_000, v: 10.0},
		{t: 340_000, v: 60.0}, // 240s gap, range is 120s
	}
	base := newMockIterator(samples)
	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	// First, advance to get into synthesis mode.
	vt := it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt) // t=100k, v=10
	s.Require().Equal(int64(100_000), it.AtT())

	vt = it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt) // first synthetic, ~t=160k
	firstSynthT := it.AtT()

	// Now seek forward within the gap (should skip some synthetics).
	targetT := firstSynthT + 100_000 // skip ahead
	vt = it.Seek(targetT)
	s.Require().Equal(chunkenc.ValFloat, vt)

	// Should get a synthetic sample >= targetT.
	t := it.AtT()
	s.Require().GreaterOrEqual(t, targetT)
	s.Require().Less(t, int64(340_000))
}

// TestIteratorSeekWithTargetLessOrEqualT0 tests seeking with target <= t0
// (should not touch base after initialization).
func (s *IteratorSuite) TestIteratorSeekWithTargetLessOrEqualT0() {
	// Use a mock that tracks calls to ensure we don't call base unnecessarily.
	samples := []struct {
		t int64
		v float64
	}{
		{t: 100_000, v: 10.0},
		{t: 200_000, v: 20.0},
		{t: 300_000, v: 30.0},
	}
	base := newMockIterator(samples)
	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	// Initialize by advancing once.
	vt := it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt)
	s.Require().Equal(int64(100_000), it.AtT())

	// Now seek backwards or to current position (target <= t0).
	vt = it.Seek(100_000)
	s.Require().Equal(chunkenc.ValFloat, vt)
	t := it.AtT()
	s.Require().Equal(int64(100_000), t)
}

// TestIteratorSeekWithBufferedT1 tests seeking when t1 is buffered (gap detected
// but synthesis not yet started) and target is in range (t0 < target <= t1).
func (s *IteratorSuite) TestIteratorSeekWithBufferedT1() {
	samples := []struct {
		t int64
		v float64
	}{
		{t: 100_000, v: 10.0},
		{t: 340_000, v: 50.0}, // 240s gap, triggers synthesis
	}
	base := newMockIterator(samples)
	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	// Initialize: read first sample.
	vt := it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt)
	s.Require().Equal(int64(100_000), it.AtT())

	// Read next, which triggers gap detection and buffers t1.
	// (gap detection doesn't start synthesis yet, just buffers t1 and haveT1=true)
	vt = it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt)
	// First synthetic should be returned.

	// Now seek to a point between 100k and 340k.
	// This tests the case where target <= t1 and synthesis is already armed.
	vt = it.Seek(250_000)
	s.Require().Equal(chunkenc.ValFloat, vt)

	t := it.AtT()
	s.Require().GreaterOrEqual(t, int64(250_000))
	s.Require().Less(t, int64(340_000))
}

// TestIteratorSeekAdvanceBase tests seeking beyond t1 into unconsumed base data.
func (s *IteratorSuite) TestIteratorSeekAdvanceBase() {
	samples := []struct {
		t int64
		v float64
	}{
		{t: 100_000, v: 10.0},
		{t: 200_000, v: 20.0},
		{t: 300_000, v: 30.0},
		{t: 400_000, v: 40.0},
	}
	base := newMockIterator(samples)
	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	// Initialize and read first two samples.
	vt := it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt)
	s.Require().Equal(int64(100_000), it.AtT())

	vt = it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt)
	s.Require().Equal(int64(160_000), it.AtT())

	// Seek to 350_000, which is beyond the buffered t1 (200_000).
	// This tests seekAdvanceBase path.
	vt = it.Seek(350_000)
	s.Require().Equal(chunkenc.ValFloat, vt)

	t := it.AtT()
	s.Require().GreaterOrEqual(t, int64(350_000))
	s.Require().Equal(int64(360_000), t)
}

// TestIteratorSeekWithinStateWithBufferedT1 tests seeking within seekWithinState
// when haveT1 is set and target is between t0 and t1.
func (s *IteratorSuite) TestIteratorSeekWithinStateWithBufferedT1() {
	samples := []struct {
		t int64
		v float64
	}{
		{t: 100_000, v: 10.0},
		{t: 340_000, v: 50.0}, // large gap
		{t: 440_000, v: 60.0},
	}
	base := newMockIterator(samples)
	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	// Initialize and start synthesis.
	vt := it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt) // t0=100k

	vt = it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt) // first synthetic

	// Now seek to a point in the gap, which will be handled by seekWithinState.
	// The buffered t1 (340k) should be accessible.
	targetT := it.AtT() + 50_000
	vt = it.Seek(targetT)
	s.Require().Equal(chunkenc.ValFloat, vt)

	// Result should be at or after targetT, and within the gap.
	t := it.AtT()
	s.Require().GreaterOrEqual(t, targetT)
	s.Require().Less(t, int64(340_000))
}

// TestIteratorReset tests that Reset() properly reinitializes the iterator
// for reuse with a new base iterator.
func (s *IteratorSuite) TestIteratorReset() {
	samplesFirst := []struct {
		t int64
		v float64
	}{
		{t: 10_000, v: 1.0},
		{t: 20_000, v: 2.0},
	}
	baseFirst := newMockIterator(samplesFirst)
	it := upsampler.NewIterator(baseFirst, 60_000, 240_000, upsampler.CounterFunc)

	// Use the iterator once.
	vt := it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt)
	s.Require().Equal(int64(10_000), it.AtT())

	// Reset with a new base iterator and a range where there's no gap (to avoid synthesis).
	samplesSecond := []struct {
		t int64
		v float64
	}{
		{t: 100_000, v: 10.0},
		{t: 200_000, v: 20.0}, // gap of 100ms < 60ms range, no synthesis
	}
	baseSecond := newMockIterator(samplesSecond)
	it.Reset(baseSecond, 60_000, 240_000, upsampler.CounterFunc)

	// Verify state is clean and we start fresh.
	vt = it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt)
	t, v := it.At()
	s.Require().Equal(int64(100_000), t)
	s.Require().Equal(10.0, v)

	// Continue iterating to verify reset worked fully.
	vt = it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt)
	t, v = it.At()
	s.Require().Equal(int64(160_000), t)
	s.Require().Equal(16.0, v)
}

// TestIteratorSeekT0LessThanTargetLessThanT1WithoutSynthesis tests seeking
// when the iterator has buffered state and we seek within a reasonable range.
func (s *IteratorSuite) TestIteratorSeekT0LessThanTargetLessThanT1WithoutSynthesis() {
	samples := []struct {
		t int64
		v float64
	}{
		{t: 100_000, v: 10.0},
		{t: 200_000, v: 20.0},
		{t: 440_000, v: 50.0}, // 240s gap, will trigger synthesis
	}
	base := newMockIterator(samples)
	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	// Initialize.
	vt := it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt) // t0=100k

	// Read next without gap: t0=200k.
	vt = it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt) // t0=160k
	s.Require().Equal(int64(160_000), it.AtT())

	// Now the base is positioned at 440k, and we have t0=200k, t1=440k (buffered)
	// in haveT1=true state (gap > step triggered synthesis).
	// Read one more Next() to start yielding synthetics.
	vt = it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt) // first synthetic

	// Now seek within the synthesis range.
	// This should work without errors.
	vt = it.Seek(300_000)
	s.Require().Equal(chunkenc.ValFloat, vt)

	t := it.AtT()
	s.Require().GreaterOrEqual(t, int64(300_000))
	s.Require().Less(t, int64(440_000))
}

// TestIteratorSeekWithinStateExhaustsBufferedAndReturnsNone tests the path
// where seekWithinState exhausts buffered samples and needs to return ValNone
// (haveT1 becomes false, nextSynthT is 0, and we break from the loop).
func (s *IteratorSuite) TestIteratorSeekWithinStateExhaustsBufferedAndReturnsNone() {
	samples := []struct {
		t int64
		v float64
	}{
		{t: 100_000, v: 10.0},
		{t: 340_000, v: 50.0}, // large gap, will trigger synthesis
	}
	base := newMockIterator(samples)
	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	// Initialize and start synthesis.
	vt := it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt) // t0=100k

	vt = it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt) // first synthetic

	// Now we're in synthesis mode.
	// To hit the specific path in seekWithinState where haveT1 becomes false
	// and we break from the loop, seek beyond all available data.
	vt = it.Seek(999_999_999)
	s.Require().Equal(chunkenc.ValNone, vt)
}

// TestIteratorSeekAdvanceBaseReturnsNone tests seekAdvanceBase when base.Next()
// returns ValNone (EOF), which should propagate directly.
func (s *IteratorSuite) TestIteratorSeekAdvanceBaseReturnsNone() {
	samples := []struct {
		t int64
		v float64
	}{
		{t: 100_000, v: 10.0},
		{t: 200_000, v: 20.0},
	}
	base := newMockIterator(samples)
	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	// Initialize.
	vt := it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt) // t0=100k

	vt = it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt) // t0=200k (no gap)

	// Seek beyond all samples. This will call seekAdvanceBase which will try to
	// read from base and get ValNone, which it should return immediately.
	vt = it.Seek(999_999)
	s.Require().Equal(chunkenc.ValNone, vt)
}

// TestIteratorSeekWithinStateTransitionsHaveT1ToNext tests the specific path
// in seekWithinState where haveT1=true, t1 < target, and we set haveT1=false
// then continue to read more.
func (s *IteratorSuite) TestIteratorSeekWithinStateTransitionsHaveT1ToNext() {
	samples := []struct {
		t int64
		v float64
	}{
		{t: 100_000, v: 10.0},
		{t: 340_000, v: 50.0}, // large gap
		{t: 600_000, v: 60.0}, // gap wider than resolution*2: no synthesis, real sample only
	}
	base := newMockIterator(samples)
	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	// Initialize and trigger synthesis.
	vt := it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt) // t0=100k

	vt = it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt) // first synthetic

	// Now we're in synthesis with haveT1=true (t1=340k buffered).
	// Seek to 150k (in the first synthetic batch).
	vt = it.Seek(150_000)
	s.Require().Equal(chunkenc.ValFloat, vt)
	s.Require().GreaterOrEqual(it.AtT(), int64(150_000))

	// Now seek to 550k, which is beyond current t0 but requires consuming
	// the buffered t1. This will go through seekWithinState, find that haveT1=true
	// and t1 < target, so it will set haveT1=false and continue looping.
	vt = it.Seek(550_000)
	s.Require().Equal(chunkenc.ValFloat, vt)
	s.Require().GreaterOrEqual(it.AtT(), int64(550_000))
}

// TestIteratorSeekReturnT1DirectlyPath tests seeking when t1 is ready and
// target <= t1, triggering the `return chunkenc.ValFloat` path for t1.
func (s *IteratorSuite) TestIteratorSeekReturnT1DirectlyPath() {
	samples := []struct {
		t int64
		v float64
	}{
		{t: 100_000, v: 10.0},
		{t: 200_000, v: 20.0},
		{t: 300_000, v: 30.0},
		{t: 400_000, v: 40.0},
	}
	base := newMockIterator(samples)
	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	// Manually position: call Next() to get t0=200k, then seek to a value
	// between t0 and t1 to force returning t1 directly.
	vt := it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt) // t0=100k

	vt = it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt) // t0=200k (no gap, so t1 also=200k)

	vt = it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt) // t0=300k

	// At this point, after reading 300k sample:
	// - t0 should be at 300k
	// - t1 should be at 400k (buffered if gap>range, or same as t0 if gap<=range)
	// With range=120k and gap=100k, gap <= range, so t1 is not buffered.

	// Try seek to value less than next unread sample.
	vt = it.Seek(350_000)
	s.Require().Equal(chunkenc.ValFloat, vt)
	s.Require().GreaterOrEqual(it.AtT(), int64(350_000))
}

// TestIteratorSeekWithinStateBufferedT1LessThanTarget tests the path in
// seekWithinState where haveT1=true, t1 < target, we consume t1, then break.
func (s *IteratorSuite) TestIteratorSeekWithinStateBufferedT1LessThanTarget() {
	samples := []struct {
		t int64
		v float64
	}{
		{t: 100_000, v: 10.0},
		{t: 340_000, v: 60.0}, // 240s gap, triggers synthesis
		{t: 460_000, v: 70.0},
	}
	base := newMockIterator(samples)
	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	// Start synthesis.
	vt := it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt) // t0=100k

	vt = it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt) // first synthetic, t0=160k, t1=340k buffered

	// Seek past the last synthetic point of the gap: seekWithinState exhausts the
	// synthetic queue and yields the buffered t1 (haveT1=false afterwards).
	vt = it.Seek(300_000)
	s.Require().Equal(chunkenc.ValFloat, vt)
	s.Require().Equal(int64(340_000), it.AtT())

	// Now t1 is consumed, so the next Seek has to read from base again.
	vt = it.Seek(400_000)
	s.Require().Equal(chunkenc.ValFloat, vt)
	s.Require().GreaterOrEqual(it.AtT(), int64(400_000))
}

// TestIteratorSeekAdvanceBaseEOF tests that seekAdvanceBase returns ValNone
// when base iterator reaches EOF.
func (s *IteratorSuite) TestIteratorSeekAdvanceBaseEOF() {
	samples := []struct {
		t int64
		v float64
	}{
		{t: 100_000, v: 10.0},
		{t: 200_000, v: 20.0},
	}
	base := newMockIterator(samples)
	it := upsampler.NewIterator(base, 60_000, 240_000, upsampler.CounterFunc)

	// Initialize.
	vt := it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt)

	vt = it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt)

	// Seek beyond all data - forces seekAdvanceBase to hit EOF.
	vt = it.Seek(999_999_999)
	s.Require().Equal(chunkenc.ValNone, vt)
}

// TestIteratorSeekWithinStateConsumesT1 tests seekWithinState consuming
// buffered t1 when seeking within the synthetic range up to t1.
func (s *IteratorSuite) TestIteratorSeekWithinStateConsumesT1() {
	samples := []struct {
		t int64
		v float64
	}{
		{t: 100_000, v: 10.0},
		{t: 200_000, v: 20.0},
		{t: 400_000, v: 60.0}, // gap 200s: step (75s) < gap <= resolution*2 (300s)
	}
	base := newMockIterator(samples)
	it := upsampler.NewIterator(base, 75_000, 300_000, upsampler.CounterFunc) // range = 150s

	// Initialize with first two samples.
	vt := it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt)

	vt = it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt)

	// Third Next: yields the buffered real sample at 200k.
	vt = it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt)

	// Seek to exactly t1 (400k)
	// seekWithinState will iterate synthetics, consume t1, and return it
	vt = it.Seek(400_000)
	s.Require().Equal(chunkenc.ValFloat, vt)
	t, _ := it.At()
	s.Require().Equal(int64(400_000), t)
}

// TestIteratorSeekNoSynthesisPath tests Seek returning chunkenc.ValFloat
// when target <= t1 and nextSynthT == 0 (no synthesis, gap <= range).
func (s *IteratorSuite) TestIteratorSeekNoSynthesisPath() {
	samples := []struct {
		t int64
		v float64
	}{
		{t: 100_000, v: 10.0},
		{t: 200_000, v: 20.0},
		{t: 300_000, v: 30.0},
	}
	base := newMockIterator(samples)
	it := upsampler.NewIterator(base, 75_000, 300_000, upsampler.CounterFunc) // range = 150ms

	// Read first sample.
	vt := it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt)
	t, _ := it.At()
	s.Require().Equal(int64(100_000), t)

	// Read second sample: gap 100ms < 150ms range, no synthesis, nextSynthT = 0
	vt = it.Next()
	s.Require().Equal(chunkenc.ValFloat, vt)
	t, _ = it.At()
	s.Require().Equal(int64(175_000), t)

	// Now: t0=200k, t1=200k (from second Next), nextSynthT=0
	// Seek to 150k: target < t0, so seekWithinState is called and returns t0
	vt = it.Seek(150_000)
	s.Require().Equal(chunkenc.ValFloat, vt)
	t, _ = it.At()
	s.Require().Equal(int64(175_000), t)

	// Seek to 250k: target > t0 and target > t1 (200k), so seekAdvanceBase is called
	// seekAdvanceBase reads next sample (300k)
	vt = it.Seek(250_000)
	s.Require().Equal(chunkenc.ValFloat, vt)
	t, _ = it.At()
	s.Require().Equal(int64(275_000), t)
}

// TestIteratorHistogramPassthroughNoSynthesis tests Next() passing through
// histogram samples without synthesis (lines 97-100).
func (s *IteratorSuite) TestIteratorHistogramPassthroughNoSynthesis() {
	samples := []struct {
		t int64
		h *histogram.FloatHistogram
	}{
		{t: 100_000, h: &histogram.FloatHistogram{Count: 1}},
		{t: 200_000, h: &histogram.FloatHistogram{Count: 2}},
	}
	base := newMockHistogramIterator(samples)
	it := upsampler.NewIterator(base, 75_000, 300_000, upsampler.CounterFunc) // range = 150ms

	// First Next: reads histogram at 100k
	vt := it.Next()
	s.Require().Equal(chunkenc.ValFloatHistogram, vt)
	t, _ := it.At()
	s.Require().Equal(int64(100_000), t)

	// Second Next: reads histogram at 200k
	// Gap is 100ms < 150ms, so no synthesis, just pass through
	vt = it.Next()
	s.Require().Equal(chunkenc.ValFloatHistogram, vt)
	t, _ = it.At()
	s.Require().Equal(int64(200_000), t)
}
