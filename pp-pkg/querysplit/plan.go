package querysplit

import "slices"

// BoundsFunc returns the sorted, de-duplicated time bounds (ms) of the persisted blocks.
type BoundsFunc func() []int64

// subRange is a part of the query range. Start and end are the first and the last
// evaluation steps (ms) of the sub-range, both inclusive.
type subRange struct {
	start, end int64
}

// planSubRanges splits the steps start + i*step in [start, end] into sub-ranges. Each block
// bound b gives a split point b + window. A sub-range starts at the first step at or after
// its split point. Sub-ranges shorter than mergeFactor*window are merged with the next one,
// and a short last sub-range is merged with the previous one. It returns nil if fewer than
// two sub-ranges remain.
func planSubRanges(start, end, step int64, bounds []int64, window int64, mergeFactor int) []subRange {
	if step <= 0 || end <= start {
		return nil
	}

	minLength := int64(mergeFactor) * window
	cuts := make([]int64, 0, 8)
	last := start
	for _, b := range bounds {
		p := b + window
		if p <= start || p > end {
			continue
		}

		// s is the first step at or after p. s <= last means that two split points are
		// between the same pair of steps.
		s := start + (p-start+step-1)/step*step
		if s > end || s <= last || s-last < minLength {
			continue
		}

		cuts = append(cuts, s)
		last = s
	}

	if len(cuts) > 0 && end+step-last < minLength {
		cuts = cuts[:len(cuts)-1]
	}

	if len(cuts) == 0 {
		return nil
	}

	ranges := make([]subRange, 0, len(cuts)+1)
	from := start
	for _, c := range cuts {
		ranges = append(ranges, subRange{start: from, end: c - step})
		from = c
	}

	return append(ranges, subRange{start: from, end: end})
}

// countBlocks returns the number of blocks that the data range (from, to] touches. The bounds
// divide the time axis into segments. The segment after the last bound (the head) is also
// counted as a block.
func countBlocks(bounds []int64, from, to int64) int {
	first, _ := slices.BinarySearch(bounds, from+1)
	last, _ := slices.BinarySearch(bounds, to+1)
	return last - first + 1
}

// BlockBounds returns the sorted bounds of blocks from the min time of each block and the
// max time of the newest block. The max time of an older block is not a bound: it is the time
// after its last sample, a few ms before the min time of the next block.
// If minTimes is empty, BlockBounds returns nil.
func BlockBounds(minTimes []int64, maxTime int64) []int64 {
	if len(minTimes) == 0 {
		return nil
	}

	return SortBounds(append(minTimes, maxTime))
}

// SortBounds sorts the block bounds and removes duplicates in place. It returns the result.
func SortBounds(bounds []int64) []int64 {
	slices.Sort(bounds)
	return slices.Compact(bounds)
}
