package tblock

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/thanos-io/thanos/pkg/runutil"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
)

//
// HealthStats
//

// HealthStats contains health statistics about the index.
type HealthStats struct {
	// TotalSeries represents total number of series in block.
	TotalSeries int64
	// OutOfOrderSeries represents number of series that have out of order chunks.
	OutOfOrderSeries int

	// OutOfOrderChunks represents number of chunks that are out of order (older time range is after younger one).
	OutOfOrderChunks int
	// DuplicatedChunks represents number of chunks with same time ranges within same series, potential duplicates.
	DuplicatedChunks int
	// OutsideChunks represents number of all chunks that are before or after time range specified in block meta.
	OutsideChunks int
	// CompleteOutsideChunks is subset of OutsideChunks that will be never accessed.
	// They are completely out of time range specified in block meta.
	CompleteOutsideChunks int
	// Issue347OutsideChunks represents subset of OutsideChunks
	// that are outsiders caused by https://github.com/prometheus/tsdb/issues/347 and is something that Thanos handle.
	//
	// Specifically we mean here chunks with minTime == block.maxTime and maxTime > block.MaxTime. These are
	// are segregated into separate counters.
	// These chunks are safe to be deleted, since they are duplicated across 2 blocks.
	Issue347OutsideChunks int
	// OutOfOrderLabels represents the number of postings that contained out
	// of order labels, a bug present in Prometheus 2.8.0 and below.
	OutOfOrderLabels int

	// Debug Statistics.
	SeriesMinLifeDuration time.Duration
	SeriesAvgLifeDuration time.Duration
	SeriesMaxLifeDuration time.Duration

	SeriesMinLifeDurationWithoutSingleSampleSeries time.Duration
	SeriesAvgLifeDurationWithoutSingleSampleSeries time.Duration
	SeriesMaxLifeDurationWithoutSingleSampleSeries time.Duration

	SeriesMinChunks int64
	SeriesAvgChunks int64
	SeriesMaxChunks int64

	TotalChunks int64

	ChunkMinDuration time.Duration
	ChunkAvgDuration time.Duration
	ChunkMaxDuration time.Duration

	ChunkMinSize int64
	ChunkAvgSize int64
	ChunkMaxSize int64

	SeriesMinSize int64
	SeriesAvgSize int64
	SeriesMaxSize int64

	SingleSampleSeries int64
	SingleSampleChunks int64

	LabelNamesCount        int64
	MetricLabelValuesCount int64
}

// OutOfOrderLabelsErr returns an error if the HealthStats object indicates
// postings with out of order labels.  This is corrected by Prometheus Issue
// #5372 and affects Prometheus versions 2.8.0 and below.
func (i *HealthStats) OutOfOrderLabelsErr() error {
	if i.OutOfOrderLabels > 0 {
		return fmt.Errorf("index contains %d postings with out of order labels", i.OutOfOrderLabels)
	}

	return nil
}

// Issue347OutsideChunksErr returns error if stats indicates issue347 block issue,
// that is repaired explicitly before compaction (on plan block).
func (i *HealthStats) Issue347OutsideChunksErr() error {
	if i.Issue347OutsideChunks > 0 {
		return fmt.Errorf(
			"found %d chunks outside the block time range introduced by https://github.com/prometheus/tsdb/issues/347",
			i.Issue347OutsideChunks,
		)
	}

	return nil
}

// OutOfOrderChunksErr returns an error if the HealthStats object indicates
// series with out of order chunks.
func (i *HealthStats) OutOfOrderChunksErr() error {
	if i.OutOfOrderChunks > 0 {
		return fmt.Errorf(
			"%d/%d series have an average of %.3f out-of-order chunks: "+
				"%.3f of these are exact duplicates (in terms of data and time range)",
			i.OutOfOrderSeries,
			i.TotalSeries,
			float64(i.OutOfOrderChunks)/float64(i.OutOfOrderSeries),
			float64(i.DuplicatedChunks)/float64(i.OutOfOrderChunks),
		)
	}

	return nil
}

// CriticalErr returns error if stats indicates critical block issue, that might solved only by manual repair procedure.
func (i *HealthStats) CriticalErr() error {
	var errMsg []string

	n := i.OutsideChunks - (i.CompleteOutsideChunks + i.Issue347OutsideChunks)
	if n > 0 {
		errMsg = append(errMsg, fmt.Sprintf("found %d chunks non-completely outside the block time range", n))
	}

	if i.CompleteOutsideChunks > 0 {
		errMsg = append(errMsg, fmt.Sprintf(
			"found %d chunks completely outside the block time range",
			i.CompleteOutsideChunks,
		))
	}

	if len(errMsg) > 0 {
		return errors.New(strings.Join(errMsg, ", "))
	}

	return nil
}

// AnyErr returns error if stats indicates any block issue.
func (i *HealthStats) AnyErr() error {
	var errMsg []string

	if err := i.CriticalErr(); err != nil {
		errMsg = append(errMsg, err.Error())
	}

	if err := i.Issue347OutsideChunksErr(); err != nil {
		errMsg = append(errMsg, err.Error())
	}

	if err := i.OutOfOrderLabelsErr(); err != nil {
		errMsg = append(errMsg, err.Error())
	}

	if err := i.OutOfOrderChunksErr(); err != nil {
		errMsg = append(errMsg, err.Error())
	}

	if len(errMsg) > 0 {
		return errors.New(strings.Join(errMsg, ", "))
	}

	return nil
}

//
// minMaxSumInt64
//

// minMaxSumInt64 is a helper struct to calculate the min, max, and average of a set of int64 values.
type minMaxSumInt64 struct {
	sum int64
	min int64
	max int64

	cnt int64
}

// newMinMaxSumInt64 returns a new [minMaxSumInt64] struct.
func newMinMaxSumInt64() minMaxSumInt64 {
	return minMaxSumInt64{
		min: math.MaxInt64,
		max: math.MinInt64,
	}
}

// Add adds a value to the minMaxSumInt64 struct.
func (n *minMaxSumInt64) Add(v int64) {
	n.cnt++
	n.sum += v
	if n.min > v {
		n.min = v
	}
	if n.max < v {
		n.max = v
	}
}

// Avg returns the average of the values in the minMaxSumInt64 struct.
func (n *minMaxSumInt64) Avg() int64 {
	if n.cnt == 0 {
		return 0
	}
	return n.sum / n.cnt
}

// MaxMillisecond returns the maximum value in the minMaxSumInt64 struct as a time.Duration.
func (n *minMaxSumInt64) MaxMillisecond() time.Duration {
	return time.Duration(n.max) * time.Millisecond
}

// MinMillisecond returns the minimum value in the minMaxSumInt64 struct as a time.Duration.
func (n *minMaxSumInt64) MinMillisecond() time.Duration {
	return time.Duration(n.min) * time.Millisecond
}

// AvgMillisecond returns the average value in the minMaxSumInt64 struct as a time.Duration.
func (n *minMaxSumInt64) AvgMillisecond() time.Duration {
	return time.Duration(n.Avg()) * time.Millisecond
}

//
// Functions
//

// GatherIndexHealthStats returns useful counters as well as outsider chunks (chunks outside of block time range) that
// helps to assess index health.
// It considers https://github.com/prometheus/tsdb/issues/347 as something that Thanos can handle.
// See HealthStats.Issue347OutsideChunks for details.
//
//revive:disable-next-line:cyclomatic // calculating health stats is a complex task
//revive:disable-next-line:function-length // calculating health stats is a complex task
//revive:disable-next-line:cognitive-complexity // calculating health stats is a complex task
func GatherIndexHealthStats(
	ctx context.Context,
	logger log.Logger,
	fn string,
	minTime, maxTime int64,
) (stats HealthStats, err error) {
	r, err := index.NewFileReader(fn)
	if err != nil {
		return stats, fmt.Errorf("open index file: %w", err)
	}
	defer runutil.CloseWithErrCapture(&err, r, "gather index issue file reader")

	key, value := index.AllPostingsKey()
	p, err := r.Postings(ctx, key, value)
	if err != nil {
		return stats, fmt.Errorf("get all postings: %w", err)
	}

	var (
		lset     labels.Labels
		prevLset labels.Labels
		builder  labels.ScratchBuilder

		chks []chunks.Meta

		seriesLifeDuration                          = newMinMaxSumInt64()
		seriesLifeDurationWithoutSingleSampleSeries = newMinMaxSumInt64()
		seriesChunks                                = newMinMaxSumInt64()
		chunkDuration                               = newMinMaxSumInt64()
		chunkSize                                   = newMinMaxSumInt64()
		seriesSize                                  = newMinMaxSumInt64()
	)

	lnames, err := r.LabelNames(ctx)
	if err != nil {
		return stats, fmt.Errorf("label names: %w", err)
	}
	stats.LabelNamesCount = int64(len(lnames))

	lvals, err := r.LabelValues(ctx, "__name__")
	if err != nil {
		return stats, fmt.Errorf("metric label values: %w", err)
	}
	stats.MetricLabelValuesCount = int64(len(lvals))

	// As of version two all series entries are 16 byte padded. All references
	// we get have to account for that to get the correct offset.
	offsetMultiplier := 1
	version := r.Version()
	if version >= index.FormatV2 {
		offsetMultiplier = 16 //revive:disable-line:add-constant // offset multiplier is constant
	}

	// Per series.
	var prevID storage.SeriesRef
	for p.Next() {
		prevLset.CopyFrom(lset)

		id := p.At()
		if prevID != 0 {
			// Approximate size.
			seriesSize.Add(int64(id-prevID) * int64(offsetMultiplier)) // #nosec G115 // no overflow
		}
		prevID = id
		stats.TotalSeries++

		if err = r.Series(id, &builder, &chks); err != nil {
			return stats, fmt.Errorf("read series: %w", err)
		}

		lset = builder.Labels()
		if lset.IsEmpty() {
			return stats, fmt.Errorf("empty label set detected for series %d", id)
		}

		if !prevLset.IsEmpty() && labels.Compare(prevLset, lset) >= 0 {
			return stats, fmt.Errorf("series %v out of order; previous %v", lset, prevLset)
		}

		var l0 *labels.Label
		lset.Range(func(l labels.Label) {
			if l0 != nil {
				if l.Name < l0.Name {
					stats.OutOfOrderLabels++
					_ = level.Warn(logger).Log(
						"msg", "out-of-order label set: known bug in Prometheus 2.8.0 and below",
						"labelset", lset.String(),
						"series", fmt.Sprintf("%d", id),
					)
				}
			}

			l0 = &l
		})

		if len(chks) == 0 {
			return stats, fmt.Errorf("empty chunks for series %d", id)
		}

		ooo := 0
		seriesLifeTimeMs := int64(0)
		// Per chunk in series.
		for i, c := range chks {
			stats.TotalChunks++

			chkDur := c.MaxTime - c.MinTime
			seriesLifeTimeMs += chkDur
			chunkDuration.Add(chkDur)
			if chkDur == 0 {
				stats.SingleSampleChunks++
			}

			// Approximate size.
			if i < len(chks)-2 { //revive:disable-line:add-constant // prev check is for the last chunk
				sgmIndex, chkStart := chunks.BlockChunkRef(c.Ref).Unpack()
				sgmIndex2, chkStart2 := chunks.BlockChunkRef(chks[i+1].Ref).Unpack()
				// Skip the case where two chunks are spread into 2 files.
				if sgmIndex == sgmIndex2 {
					chunkSize.Add(int64(chkStart2 - chkStart))
				}
			}

			// Chunk vs the block ranges.
			if c.MinTime < minTime || c.MaxTime > maxTime {
				stats.OutsideChunks++
				if c.MinTime > maxTime || c.MaxTime < minTime {
					stats.CompleteOutsideChunks++
				} else if c.MinTime == maxTime {
					stats.Issue347OutsideChunks++
				}
			}

			if i == 0 {
				continue
			}

			c0 := chks[i-1]

			// Chunk order within block.
			if c.MinTime > c0.MaxTime {
				continue
			}

			if c.MinTime == c0.MinTime && c.MaxTime == c0.MaxTime {
				// TODO(bplotka): Calc and check checksum from chunks itself.
				// The chunks can overlap 1:1 in time, but does not have same data.
				// We assume same data for simplicity, but it can be a symptom of error.
				stats.DuplicatedChunks++
				continue
			}

			// Chunks partly overlaps or out of order.
			ooo++
		}
		if ooo > 0 {
			stats.OutOfOrderSeries++
			stats.OutOfOrderChunks += ooo
			_ = level.Debug(logger).Log("msg", "found out of order series", "labels", lset)
		}

		seriesChunks.Add(int64(len(chks)))
		seriesLifeDuration.Add(seriesLifeTimeMs)

		if seriesLifeTimeMs == 0 {
			stats.SingleSampleSeries++
		} else {
			seriesLifeDurationWithoutSingleSampleSeries.Add(seriesLifeTimeMs)
		}
	}
	if p.Err() != nil {
		return stats, fmt.Errorf("walk postings: %w", err)
	}

	stats.SeriesMaxLifeDuration = seriesLifeDuration.MaxMillisecond()
	stats.SeriesAvgLifeDuration = seriesLifeDuration.AvgMillisecond()
	stats.SeriesMinLifeDuration = seriesLifeDuration.MinMillisecond()

	stats.SeriesMaxLifeDurationWithoutSingleSampleSeries = seriesLifeDurationWithoutSingleSampleSeries.MaxMillisecond()
	stats.SeriesAvgLifeDurationWithoutSingleSampleSeries = seriesLifeDurationWithoutSingleSampleSeries.AvgMillisecond()
	stats.SeriesMinLifeDurationWithoutSingleSampleSeries = seriesLifeDurationWithoutSingleSampleSeries.MinMillisecond()

	stats.SeriesMaxChunks = seriesChunks.max
	stats.SeriesAvgChunks = seriesChunks.Avg()
	stats.SeriesMinChunks = seriesChunks.min

	stats.ChunkMaxSize = chunkSize.max
	stats.ChunkAvgSize = chunkSize.Avg()
	stats.ChunkMinSize = chunkSize.min

	stats.SeriesMaxSize = seriesSize.max
	stats.SeriesAvgSize = seriesSize.Avg()
	stats.SeriesMinSize = seriesSize.min

	stats.ChunkMaxDuration = chunkDuration.MaxMillisecond()
	stats.ChunkAvgDuration = chunkDuration.AvgMillisecond()
	stats.ChunkMinDuration = chunkDuration.MinMillisecond()

	return stats, nil
}
