package querysplit

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// ms converts a duration since the epoch to milliseconds.
func ms(d time.Duration) int64 { return d.Milliseconds() }

// everyBound returns the bounds from to to (inclusive) with the given spacing.
func everyBound(from, to, spacing time.Duration) []int64 {
	var bounds []int64
	for t := from; t <= to; t += spacing {
		bounds = append(bounds, ms(t))
	}
	return bounds
}

func TestPlanSubRanges(t *testing.T) {
	const (
		minute = time.Minute
		hour   = time.Hour
		day    = 24 * time.Hour
	)

	for _, tc := range []struct {
		name        string
		start, end  time.Duration
		step        time.Duration
		bounds      []int64
		window      time.Duration
		mergeFactor int
		expected    []subRange
	}{
		{
			name:  "no bounds",
			start: 0, end: 12 * hour, step: minute,
			window: 5 * minute, mergeFactor: 4,
		},
		{
			name:  "split points outside the query range",
			start: 3 * hour, end: 5 * hour, step: minute,
			bounds: []int64{ms(0), ms(2 * hour), ms(6 * hour)}, window: 5 * minute, mergeFactor: 4,
		},
		{
			name:  "query covers several blocks and the head",
			start: hour, end: 7 * hour, step: minute,
			bounds: everyBound(0, 6*hour, 2*hour), window: 5 * minute, mergeFactor: 4,
			expected: []subRange{
				{ms(hour), ms(2*hour + 4*minute)},
				{ms(2*hour + 5*minute), ms(4*hour + 4*minute)},
				{ms(4*hour + 5*minute), ms(6*hour + 4*minute)},
				{ms(6*hour + 5*minute), ms(7 * hour)},
			},
		},
		{
			name:  "split point between two steps",
			start: 30 * time.Second, end: 4 * hour, step: minute,
			bounds: []int64{ms(2 * hour)}, window: 5 * minute, mergeFactor: 4,
			expected: []subRange{
				{ms(30 * time.Second), ms(2*hour + 4*minute + 30*time.Second)},
				// The last sub-range keeps the original end, so the engine evaluates the same steps.
				{ms(2*hour + 5*minute + 30*time.Second), ms(4 * hour)},
			},
		},
		{
			name:  "step longer than a block",
			start: 0, end: 20 * hour, step: 5 * hour,
			bounds: everyBound(0, 20*hour, 2*hour), window: 5 * minute, mergeFactor: 4,
			expected: []subRange{
				{0, 0},
				{ms(5 * hour), ms(5 * hour)},
				{ms(10 * hour), ms(10 * hour)},
				{ms(15 * hour), ms(15 * hour)},
				{ms(20 * hour), ms(20 * hour)},
			},
		},
		{
			name:  "overlapping blocks and equal bounds",
			start: 0, end: 8 * hour, step: minute,
			bounds: []int64{0, ms(2 * hour), ms(2 * hour), ms(4 * hour), ms(6 * hour)}, window: 5 * minute, mergeFactor: 4,
			expected: []subRange{
				{0, ms(2*hour + 4*minute)},
				{ms(2*hour + 5*minute), ms(4*hour + 4*minute)},
				{ms(4*hour + 5*minute), ms(6*hour + 4*minute)},
				{ms(6*hour + 5*minute), ms(8 * hour)},
			},
		},
		{
			name:  "W is zero",
			start: 0, end: 4 * hour, step: minute,
			bounds: []int64{ms(2 * hour)}, window: 0, mergeFactor: 4,
			expected: []subRange{
				{0, ms(2*hour - minute)},
				{ms(2 * hour), ms(4 * hour)},
			},
		},
		{
			name:  "step longer than W and no step near the boundary",
			start: 10 * minute, end: 4*hour + 10*minute, step: 15 * minute,
			bounds: []int64{ms(2 * hour)}, window: 5 * minute, mergeFactor: 4,
			expected: []subRange{
				{ms(10 * minute), ms(hour + 55*minute)},
				{ms(2*hour + 10*minute), ms(4*hour + 10*minute)},
			},
		},
		{
			name:  "step longer than W and one step near the boundary",
			start: 2 * minute, end: 4*hour + 2*minute, step: 15 * minute,
			bounds: []int64{ms(2 * hour)}, window: 5 * minute, mergeFactor: 4,
			expected: []subRange{
				{ms(2 * minute), ms(2*hour + 2*minute)},
				{ms(2*hour + 17*minute), ms(4*hour + 2*minute)},
			},
		},
		{
			name:  "short window keeps all block boundaries",
			start: 0, end: 12 * hour, step: minute,
			bounds: everyBound(0, 12*hour, 2*hour), window: 5 * minute, mergeFactor: 4,
			expected: []subRange{
				{0, ms(2*hour + 4*minute)},
				{ms(2*hour + 5*minute), ms(4*hour + 4*minute)},
				{ms(4*hour + 5*minute), ms(6*hour + 4*minute)},
				{ms(6*hour + 5*minute), ms(8*hour + 4*minute)},
				{ms(8*hour + 5*minute), ms(10*hour + 4*minute)},
				{ms(10*hour + 5*minute), ms(12 * hour)},
			},
		},
		{
			name:  "wide window merges sub-ranges",
			start: 0, end: 10 * day, step: hour,
			bounds: everyBound(0, 10*day, 2*hour), window: day, mergeFactor: 4,
			expected: []subRange{
				{0, ms(4*day - hour)},
				{ms(4 * day), ms(10 * day)},
			},
		},
		{
			name:  "short last sub-range merges into the previous sub-range",
			start: 0, end: 7 * day, step: hour,
			bounds: everyBound(0, 7*day, 2*hour), window: day, mergeFactor: 4,
		},
		{
			name:  "window longer than the query range",
			start: 7 * day, end: 8 * day, step: hour,
			bounds: everyBound(0, 8*day, 2*hour), window: 7 * day, mergeFactor: 4,
		},
		{
			name:  "query range inside one block",
			start: 10 * minute, end: hour + 50*minute, step: minute,
			bounds: []int64{0, ms(2 * hour)}, window: 5 * minute, mergeFactor: 4,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			actual := planSubRanges(ms(tc.start), ms(tc.end), ms(tc.step), tc.bounds, ms(tc.window), tc.mergeFactor)

			require.Equal(t, tc.expected, actual)
		})
	}
}

func TestBlockBounds(t *testing.T) {
	// Arrange: the max time of a block is a few ms before the min time of the next block.
	minTimes := []int64{ms(6 * time.Hour), 0, ms(12 * time.Hour)}
	maxTime := ms(14*time.Hour) - 3

	// Act
	bounds := BlockBounds(minTimes, maxTime)

	// Assert
	require.Equal(t, []int64{0, ms(6 * time.Hour), ms(12 * time.Hour), ms(14*time.Hour) - 3}, bounds)
	require.Equal(t, 4, countBlocks(bounds, 0, ms(15*time.Hour)))
	require.Nil(t, BlockBounds(nil, 0))
}

func TestCountBlocks(t *testing.T) {
	bounds := everyBound(0, 12*time.Hour, 2*time.Hour)
	for _, tc := range []struct {
		name     string
		from     time.Duration
		to       time.Duration
		expected int
	}{
		{name: "inside one block", from: 10 * time.Minute, to: time.Hour, expected: 1},
		{name: "ends at a bound", from: 10 * time.Minute, to: 2 * time.Hour, expected: 2},
		{name: "starts at a bound", from: 2 * time.Hour, to: 3 * time.Hour, expected: 1},
		{name: "several blocks", from: -5 * time.Minute, to: 8 * time.Hour, expected: 6},
		{name: "head after the last bound", from: 11 * time.Hour, to: 14 * time.Hour, expected: 2},
		{name: "before the first bound", from: -3 * time.Hour, to: -time.Hour, expected: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, countBlocks(bounds, ms(tc.from), ms(tc.to)))
		})
	}
}

func TestPlanSubRangesCoversEachStepOnce(t *testing.T) {
	bounds := everyBound(0, 30*24*time.Hour, 2*time.Hour)
	for _, step := range []time.Duration{15 * time.Second, 7 * time.Minute, 3 * time.Hour, 5 * time.Hour} {
		start, end := ms(37*time.Minute), ms(10*24*time.Hour)
		end = start + (end-start)/ms(step)*ms(step)

		ranges := planSubRanges(start, end, ms(step), bounds, ms(5*time.Minute), 4)

		require.GreaterOrEqual(t, len(ranges), 2, "step %s", step)
		next := start
		for _, r := range ranges {
			require.Equal(t, next, r.start, "step %s", step)
			require.LessOrEqual(t, r.start, r.end, "step %s", step)
			require.Zero(t, (r.end-start)%ms(step), "step %s", step)
			next = r.end + ms(step)
		}
		require.Equal(t, end+ms(step), next, "step %s", step)
	}
}
