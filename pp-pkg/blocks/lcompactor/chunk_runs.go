package lcompactor

import (
	"cmp"
	"slices"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

//
// disjointChunkSeries
//

// disjointChunkSeries retains the original merge order for the fallback iterator.
type disjointChunkSeries struct {
	inputs []storage.ChunkSeries
	merge  storage.VerticalChunkSeriesMergeFunc
}

// Labels returns the common label set of the input series.
func (s *disjointChunkSeries) Labels() labels.Labels { return s.inputs[0].Labels() }

// Iterator uses the regular merge when the collector cannot concatenate the inputs.
func (s *disjointChunkSeries) Iterator(reuse chunks.Iterator) chunks.Iterator {
	return s.merge(s.inputs...).Iterator(reuse)
}

//
// chunkRun
//

// chunkRun holds an input iterator and the first chunk it produced, if any.
type chunkRun struct {
	iter  chunks.Iterator
	first chunks.Meta
	ready bool
}

//
// chunkCollector
//

// chunkCollector buffers one series for the chunk and index writers.
type chunkCollector struct {
	metas []chunks.Meta
	runs  []chunkRun
	iter  chunks.Iterator
}

// appendRuns orders inputs by their first chunk, then reads each in full without a chunk heap.
// It returns (false, nil) if a chunk starts at or before the previous chunk ends,
// whether the chunks come from the same input or different inputs.
//
//revive:disable-next-line:function-length // this is a complex algorithm.
//revive:disable-next-line:cyclomatic // this is a complex algorithm.
//revive:disable-next-line:cognitive-complexity // this is a complex algorithm.
func (c *chunkCollector) appendRuns(inputs []storage.ChunkSeries) (bool, error) {
	if len(c.runs) < len(inputs) {
		c.runs = slices.Grow(c.runs, len(inputs)-len(c.runs))[:len(inputs)]
	}

	runs := c.runs[:len(inputs)]
	for i, series := range inputs {
		run := &runs[i]
		run.iter = series.Iterator(run.iter)
		run.ready = run.iter.Next()
		if run.ready {
			run.first = run.iter.At()
		} else if err := run.iter.Err(); err != nil {
			return false, err
		}
	}

	// The series heap does not return equal label sets in block order.
	slices.SortFunc(runs, func(a, b chunkRun) int {
		if !a.ready {
			if !b.ready {
				return 0
			}

			return 1
		}

		if !b.ready {
			return -1
		}

		return cmp.Compare(a.first.MinTime, b.first.MinTime)
	})

	for i := range runs {
		run := &runs[i]
		if !run.ready {
			break
		}

		meta := run.first
		run.first = chunks.Meta{}
		for {
			if n := len(c.metas); n > 0 && meta.MinTime <= c.metas[n-1].MaxTime {
				return false, nil
			}

			c.metas = append(c.metas, meta)
			if !run.iter.Next() {
				break
			}

			meta = run.iter.At()
		}

		if err := run.iter.Err(); err != nil {
			return false, err
		}
	}

	return true, nil
}

// collect buffers a series before either writer sees it, allowing a retry if the input ranges overlap.
func (c *chunkCollector) collect(series storage.ChunkSeries) error {
	c.metas = c.metas[:0]
	if disjoint, ok := series.(*disjointChunkSeries); ok {
		ordered, err := c.appendRuns(disjoint.inputs)
		if ordered || err != nil {
			return err
		}

		clear(c.metas)
		c.metas = c.metas[:0]
	}

	c.iter = series.Iterator(c.iter)
	for c.iter.Next() {
		c.metas = append(c.metas, c.iter.At())
	}

	return c.iter.Err()
}

// disjointChunkSeriesMerger passes the matching inputs to the collector without opening chunk iterators.
func disjointChunkSeriesMerger(fallback storage.VerticalChunkSeriesMergeFunc) storage.VerticalChunkSeriesMergeFunc {
	return func(inputs ...storage.ChunkSeries) storage.ChunkSeries {
		return &disjointChunkSeries{inputs: inputs, merge: fallback}
	}
}
