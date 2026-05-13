package querier

import (
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/pool"
)

//
// AggSeriesSet
//

// AggSeriesSet contains a set of aggregated series.
// If grouping is empty, it will return series with labels "__head_id" and "__shard_id".
// If grouping is not empty, it will return series with "__head_id" and "__shard_id" and the grouping labels.
type AggSeriesSet struct {
	serializedData   *cppbridge.DataStorageSerializedData
	labelSetSnapshot *cppbridge.LabelSetSnapshot
	seriesGroups     *cppbridge.SeriesGroups
	mint, maxt       int64
	grouping         []string
	headID           string
	shardID          uint16

	series         []AggSeries
	nextGroupIndex int
}

// NewAggSeriesSet initializes a new [AggSeriesSet].
func NewAggSeriesSet(
	serializedData *cppbridge.DataStorageSerializedData,
	labelSetSnapshot *cppbridge.LabelSetSnapshot,
	seriesGroups *cppbridge.SeriesGroups,
	mint, maxt int64,
	grouping []string,
	headID string,
	shardID uint16,
) *AggSeriesSet {
	return &AggSeriesSet{
		serializedData:   serializedData,
		labelSetSnapshot: labelSetSnapshot,
		seriesGroups:     seriesGroups,
		mint:             mint,
		maxt:             maxt,
		grouping:         grouping,
		headID:           headID,
		shardID:          shardID,
		series:           make([]AggSeries, 0, len(seriesGroups.Groups)),
	}
}

// At returns the current series.
// [storage.SeriesSet] interface implementation.
func (ss *AggSeriesSet) At() storage.Series {
	return &ss.series[len(ss.series)-1]
}

// Err returns the error of the [AggSeriesSet] - always nil.
// [storage.SeriesSet] interface implementation.
func (*AggSeriesSet) Err() error {
	return nil
}

// Next advances the iterator by one and returns false if there are no more values.
// [storage.SeriesSet] interface implementation.
func (ss *AggSeriesSet) Next() bool {
	if ss.serializedData == nil {
		return false
	}

	if ss.nextGroupIndex >= len(ss.seriesGroups.Groups) {
		return false
	}

	builder := builderPool.Get().(*labels.ScratchBuilder)
	builder.Reset()
	ss.series = append(ss.series, NewAggSeries(
		aggLabelSetCtor(
			builder,
			ss.labelSetSnapshot,
			ss.grouping,
			ss.headID,
			ss.seriesGroups.Groups[ss.nextGroupIndex][0], // 0 is the first series ID
			ss.shardID,
		),
		ss.serializedData,
		ss.seriesGroups,
		ss.nextGroupIndex,
		ss.mint,
		ss.maxt,
	))
	builderPool.Put(builder)
	ss.nextGroupIndex++

	return true
}

// Warnings returns the warnings of the [AggSeriesSet] - always nil.
// [storage.SeriesSet] interface implementation.
func (*AggSeriesSet) Warnings() annotations.Annotations {
	return nil
}

//
// AggSeries
//

// AggSeries represents a time series with aggregated samples.
type AggSeries struct {
	labelSet       labels.Labels
	serializedData *cppbridge.DataStorageSerializedData
	seriesGroups   *cppbridge.SeriesGroups
	groupIndex     int
	mint, maxt     int64
}

// NewAggSeries initializes a new [AggSeries].
func NewAggSeries(
	labelSet labels.Labels,
	serializedData *cppbridge.DataStorageSerializedData,
	seriesGroups *cppbridge.SeriesGroups,
	groupIndex int,
	mint, maxt int64,
) AggSeries {
	return AggSeries{
		labelSet:       labelSet,
		serializedData: serializedData,
		seriesGroups:   seriesGroups,
		groupIndex:     groupIndex,
		mint:           mint,
		maxt:           maxt,
	}
}

// Iterator returns an iterator that iterates over the aggregated samples of the [AggSeries].
// [storage.Series] interface implementation.
func (s *AggSeries) Iterator(it chunkenc.Iterator) chunkenc.Iterator {
	chunkIterator, ok := it.(*AggChunkIterator)
	if !ok {
		return NewAggChunkIterator(
			s.serializedData,
			s.seriesGroups,
			s.groupIndex,
			s.mint,
			s.maxt,
		)
	}

	chunkIterator.reset(s.serializedData, s.seriesGroups, s.groupIndex, s.mint, s.maxt)
	return chunkIterator
}

// Labels returns the labels of the [AggSeries].
// [storage.Series] interface implementation.
func (s *AggSeries) Labels() labels.Labels {
	return s.labelSet
}

//
// AggChunkIterator
//

// AggChunkIterator iterates over the aggregated samples of a time series, that can only get the next value.
type AggChunkIterator struct {
	serializedData *cppbridge.DataStorageSerializedData
	seriesGroups   *cppbridge.SeriesGroups
	chunkIterator  cppbridge.DataStorageSerializedDataMultiSeriesIterator
	mint           int64
	maxt           int64
	isInitialized  bool
}

// NewAggChunkIterator initializes a new [AggChunkIterator].
func NewAggChunkIterator(
	serializedData *cppbridge.DataStorageSerializedData,
	seriesGroups *cppbridge.SeriesGroups,
	groupIndex int,
	mint, maxt int64,
) *AggChunkIterator {
	it := &AggChunkIterator{
		serializedData: serializedData,
		seriesGroups:   seriesGroups,
		chunkIterator: cppbridge.NewDataStorageSerializedDataMultiSeriesIterator(
			serializedData,
			seriesGroups.Groups[groupIndex],
		),
		mint: mint,
		maxt: maxt,
	}

	return it
}

// At returns the current timestamp/value pair if the value is a float.
// [chunkenc.Iterator] interface implementation.
//
//nolint:gocritic // unnamedResult not need
func (it *AggChunkIterator) At() (int64, float64) {
	return it.chunkIterator.Timestamp(), it.chunkIterator.Value()
}

// AtFloatHistogram returns the current timestamp/value pair if the value is a histogram with floating-point counts.
// [chunkenc.Iterator] interface implementation.
func (*AggChunkIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return 0, nil
}

// AtHistogram returns the current timestamp/value pair if the value is a histogram with integer counts.
// [chunkenc.Iterator] interface implementation.
func (*AggChunkIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	return 0, nil
}

// AtT returns the current timestamp.
// [chunkenc.Iterator] interface implementation.
func (it *AggChunkIterator) AtT() int64 {
	return it.chunkIterator.Timestamp()
}

// Err returns the current error - always nil.
// [chunkenc.Iterator] interface implementation.
func (*AggChunkIterator) Err() error {
	return nil
}

// Next advances the iterator by one and returns the type of the value.
// [chunkenc.Iterator] interface implementation.
func (it *AggChunkIterator) Next() chunkenc.ValueType {
	if it.nextValue() == chunkenc.ValNone {
		return chunkenc.ValNone
	}

	if it.AtT() > it.maxt {
		return chunkenc.ValNone
	}

	return chunkenc.ValFloat
}

// Seek advances the iterator forward to the first sample with a timestamp equal or greater than t.
// [chunkenc.Iterator] interface implementation.
func (it *AggChunkIterator) Seek(t int64) chunkenc.ValueType {
	// adjust lower limit.
	if t < it.mint {
		t = it.mint
	}

	ts := it.AtT()
	if !it.isInitialized || ts < t {
		panic(fmt.Sprintf("Seek: timestamp(%d) < mint(%d)", ts, t))
		// 	it.chunkIterator.Seek(t)
		// 	it.isInitialized = true
		// 	if !it.chunkIterator.HasData() {
		// 		return chunkenc.ValNone
		// 	}
		// 	ts = it.AtT()
	}

	if ts > it.maxt {
		return chunkenc.ValNone
	}

	return chunkenc.ValFloat
}

// nextValue advances the iterator by one and returns the type of the value.
func (it *AggChunkIterator) nextValue() chunkenc.ValueType {
	if !it.isInitialized {
		if !it.chunkIterator.HasData() {
			return chunkenc.ValNone
		}

		it.isInitialized = true
		return chunkenc.ValFloat
	}

	it.chunkIterator.Next()
	if !it.chunkIterator.HasData() {
		return chunkenc.ValNone
	}

	return chunkenc.ValFloat
}

// reset resets the iterator to the beginning of the serialized data.
func (it *AggChunkIterator) reset(
	serializedData *cppbridge.DataStorageSerializedData,
	seriesGroups *cppbridge.SeriesGroups,
	groupIndex int,
	mint, maxt int64,
) {
	it.serializedData = serializedData
	it.seriesGroups = seriesGroups
	it.mint = mint
	it.maxt = maxt
	it.isInitialized = false
	// it.chunkIterator.Reset(serializedData, chunkRef)
	it.chunkIterator = cppbridge.NewDataStorageSerializedDataMultiSeriesIterator(
		serializedData,
		seriesGroups.Groups[groupIndex],
	)
}

//
// aggLabelSetCtor
//

const (
	// labelHeadID is the label name for the head ID.
	labelHeadID = "__head_id"

	// labelShardID is the label name for the shard ID.
	labelShardID = "__shard_id"
)

var (
	// groupingPool is a pool of slices for sorted grouping.
	groupingPool = pool.NewSlicePool[string]([]int{2, 3, 5})

	// errGroupingLabelsIsEnough is the error returned when the grouping labels is enough.
	errGroupingLabelsIsEnough = errors.New("grouping labels is enough")
)

// aggLabelSetCtor constructs the label set for an aggregated series.
func aggLabelSetCtor(
	sb *labels.ScratchBuilder,
	snapshot *cppbridge.LabelSetSnapshot,
	grouping []string,
	headID string,
	seriesID uint32,
	shardID uint16,
) labels.Labels {
	sb.Add(labelHeadID, headID)
	sb.Add(labelShardID, strconv.FormatUint(uint64(shardID), 10)) //revive:disable-line:add-constant it's base 10

	if len(grouping) == 0 {
		return sb.Labels()
	}

	// grouping must be sorted
	var sortedGrouping []string
	if len(grouping) == 1 {
		sortedGrouping = grouping
	} else {
		sortedGrouping = groupingPool.Get(len(grouping))
		defer groupingPool.Put(sortedGrouping)

		copy(sortedGrouping, grouping)
		slices.Sort(sortedGrouping)
	}

	i := 0
	_ = snapshot.RangeLabelSet(seriesID, func(l cppbridge.Label) error {
		if i >= len(sortedGrouping) {
			// fast exit if the grouping labels is enough
			return errGroupingLabelsIsEnough
		}

		if l.Name > sortedGrouping[i] {
			i++

			if i >= len(sortedGrouping) {
				// fast exit if the grouping labels is enough
				return errGroupingLabelsIsEnough
			}
		}

		if l.Name == sortedGrouping[i] {
			sb.Add(l.Name, l.Value)
			i++
		}

		return nil
	})
	sb.Sort()

	return sb.Labels()
}
