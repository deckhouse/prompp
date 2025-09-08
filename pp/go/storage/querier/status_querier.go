package querier

import (
	"context"
	"errors"
	"math"
	"sort"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
	"github.com/prometheus/prometheus/pp/go/storage/logger"
	"github.com/prometheus/prometheus/pp/go/util/locker"
)

const (
	// dsHeadStatus name of task.
	dsHeadStatus = "data_storage_head_status"

	// lssHeadStatus name of task.
	lssHeadStatus = "lss_head_status"
)

// QueryHeadStatus returns [HeadStatus] holds information about all shards from [Head].
func QueryHeadStatus[
	TTask Task,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TTask, TDataStorage, TLSS, TShard],
](
	ctx context.Context,
	head THead,
	limit int,
) (*HeadStatus, error) {
	shardStatuses := make([]*cppbridge.HeadStatus, head.NumberOfShards())
	for i := range shardStatuses {
		shardStatuses[i] = cppbridge.NewHeadStatus()
	}

	tw := task.NewTaskWaiter[TTask](2) //revive:disable-line:add-constant // 2 task for wait

	release, err := head.AcquireQuery(ctx)
	if err != nil {
		if !errors.Is(err, locker.ErrSemaphoreClosed) {
			logger.Warnf("[HeadStatusQuerier]: query status failed on the capture of the read lock query: %s", err)
		}

		return nil, err
	}
	defer release()

	tLSSHeadStatus := head.CreateTask(
		lssHeadStatus,
		func(shard TShard) error {
			return shard.LSS().WithRLock(func(target, _ *cppbridge.LabelSetStorage) error {
				shardStatuses[shard.ShardID()].FromLSS(target, limit)

				return nil
			})
		},
	)
	head.Enqueue(tLSSHeadStatus)

	if limit != 0 {
		tDataStorageHeadStatus := head.CreateTask(
			dsHeadStatus,
			func(shard TShard) error {
				return shard.DataStorage().WithRLock(func(ds *cppbridge.HeadDataStorage) error {
					shardStatuses[shard.ShardID()].FromDataStorage(ds)

					return nil
				})
			},
		)
		head.Enqueue(tDataStorageHeadStatus)
		tw.Add(tDataStorageHeadStatus)
	}

	tw.Add(tLSSHeadStatus)
	_ = tw.Wait()

	return sumStatuses(shardStatuses, limit), nil
}

// sumStatuses summarize the statuses received from the shards.
func sumStatuses(shardStatuses []*cppbridge.HeadStatus, limit int) *HeadStatus {
	seriesStats := make(map[string]uint64)
	labelsStats := make(map[string]uint64)
	memoryStats := make(map[string]uint64)
	countStats := make(map[string]uint64)

	headStatus := &HeadStatus{HeadStats: HeadStats{MinTime: math.MaxInt64, MaxTime: math.MinInt64}}

	for _, shardStatus := range shardStatuses {
		headStatus.HeadStats.NumSeries += uint64(shardStatus.NumSeries)
		if limit == 0 {
			continue
		}

		headStatus.HeadStats.ChunkCount += int64(shardStatus.ChunkCount)
		if headStatus.HeadStats.MaxTime < shardStatus.TimeInterval.Max {
			headStatus.HeadStats.MaxTime = shardStatus.TimeInterval.Max
		}
		if headStatus.HeadStats.MinTime > shardStatus.TimeInterval.Min {
			headStatus.HeadStats.MinTime = shardStatus.TimeInterval.Min
		}

		headStatus.HeadStats.NumLabelPairs += int(shardStatus.NumLabelPairs)

		for _, stat := range shardStatus.SeriesCountByMetricName {
			seriesStats[stat.Name] += uint64(stat.Count)
		}
		for _, stat := range shardStatus.LabelValueCountByLabelName {
			labelsStats[stat.Name] += uint64(stat.Count)
		}
		for _, stat := range shardStatus.MemoryInBytesByLabelName {
			memoryStats[stat.Name] += uint64(stat.Size)
		}
		for _, stat := range shardStatus.SeriesCountByLabelValuePair {
			countStats[stat.Name+"="+stat.Value] += uint64(stat.Count)
		}
	}

	if limit == 0 {
		return headStatus
	}

	headStatus.SeriesCountByMetricName = getSortedStats(seriesStats, limit)
	headStatus.LabelValueCountByLabelName = getSortedStats(labelsStats, limit)
	headStatus.MemoryInBytesByLabelName = getSortedStats(memoryStats, limit)
	headStatus.SeriesCountByLabelValuePair = getSortedStats(countStats, limit)

	return headStatus
}

// getSortedStats returns sorted statistics for the [Head].
func getSortedStats(stats map[string]uint64, limit int) []HeadStat {
	result := make([]HeadStat, 0, len(stats))
	for k, v := range stats {
		result = append(result, HeadStat{
			Name:  k,
			Value: v,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Value > result[j].Value
	})

	if len(result) > limit {
		return result[:limit]
	}

	return result
}

//
// HeadStatus
//

// HeadStatus holds information about all shards.
type HeadStatus struct {
	HeadStats                   HeadStats  `json:"headStats"`
	SeriesCountByMetricName     []HeadStat `json:"seriesCountByMetricName"`
	LabelValueCountByLabelName  []HeadStat `json:"labelValueCountByLabelName"`
	MemoryInBytesByLabelName    []HeadStat `json:"memoryInBytesByLabelName"`
	SeriesCountByLabelValuePair []HeadStat `json:"seriesCountByLabelValuePair"`
}

// NumSeries returns number of series.
func (hs *HeadStatus) NumSeries() uint64 {
	return hs.HeadStats.NumSeries
}

// HeadStat holds the information about individual cardinality.
type HeadStat struct {
	Name  string `json:"name"`
	Value uint64 `json:"value"`
}

// HeadStats has information about the head.
type HeadStats struct {
	NumSeries     uint64 `json:"numSeries"`
	NumLabelPairs int    `json:"numLabelPairs"`
	ChunkCount    int64  `json:"chunkCount"`
	MinTime       int64  `json:"minTime"`
	MaxTime       int64  `json:"maxTime"`
}
