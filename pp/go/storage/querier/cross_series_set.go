package querier

import (
	"errors"
	"fmt"
	"runtime"
	"slices"

	"github.com/prometheus/prometheus/model/histogram"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/util/annotations"
	"github.com/prometheus/prometheus/util/pool"
)

//
// CrossSeriesSet
//

// CrossSeriesSet contains a set of cross series.
// If grouping is empty, it will return series with labels "__head__shard_id".
// If grouping is not empty, it will return series with "__head__shard_id" and the grouping labels.
type CrossSeriesSet struct {
	serializedData   *cppbridge.DataStorageSerializedData
	labelSetSnapshot *cppbridge.LabelSetSnapshot
	seriesGroups     *cppbridge.SeriesGroups
	mint, maxt       int64
	grouping         []string
	headID           string
	shardID          uint16

	series         []CrossSeries
	nextGroupIndex int
}

// NewCrossSeriesSet initializes a new [CrossSeriesSet].
func NewCrossSeriesSet(
	serializedData *cppbridge.DataStorageSerializedData,
	labelSetSnapshot *cppbridge.LabelSetSnapshot,
	seriesGroups *cppbridge.SeriesGroups,
	mint, maxt int64,
	grouping []string,
	headID string,
	shardID uint16,
) *CrossSeriesSet {
	return &CrossSeriesSet{
		serializedData:   serializedData,
		labelSetSnapshot: labelSetSnapshot,
		seriesGroups:     seriesGroups,
		mint:             mint,
		maxt:             maxt,
		grouping:         grouping,
		headID:           headID,
		shardID:          shardID,
		series:           make([]CrossSeries, 0, len(seriesGroups.Groups)),
	}
}

// At returns the current series.
// [storage.SeriesSet] interface implementation.
func (ss *CrossSeriesSet) At() storage.Series {
	return &ss.series[len(ss.series)-1]
}

// Err returns the error of the [CrossSeriesSet] - always nil.
// [storage.SeriesSet] interface implementation.
func (*CrossSeriesSet) Err() error {
	return nil
}

// Next advances the iterator by one and returns false if there are no more values.
// [storage.SeriesSet] interface implementation.
func (ss *CrossSeriesSet) Next() bool {
	if ss.serializedData == nil {
		return false
	}

	if ss.nextGroupIndex >= len(ss.seriesGroups.Groups) {
		return false
	}

	builder := builderPool.Get().(*labels.ScratchBuilder)
	builder.Reset()
	ss.series = append(ss.series, NewCrossSeries(
		crossLabelSetCtor(
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

// Warnings returns the warnings of the [CrossSeriesSet] - always nil.
// [storage.SeriesSet] interface implementation.
func (*CrossSeriesSet) Warnings() annotations.Annotations {
	return nil
}

//
// CrossSeries
//

// CrossSeries represents a time series with cross samples.
type CrossSeries struct {
	labelSet       labels.Labels
	serializedData *cppbridge.DataStorageSerializedData
	seriesGroups   *cppbridge.SeriesGroups
	groupIndex     int
	mint, maxt     int64
}

// NewCrossSeries initializes a new [CrossSeries].
func NewCrossSeries(
	labelSet labels.Labels,
	serializedData *cppbridge.DataStorageSerializedData,
	seriesGroups *cppbridge.SeriesGroups,
	groupIndex int,
	mint, maxt int64,
) CrossSeries {
	return CrossSeries{
		labelSet:       labelSet,
		serializedData: serializedData,
		seriesGroups:   seriesGroups,
		groupIndex:     groupIndex,
		mint:           mint,
		maxt:           maxt,
	}
}

// Iterator returns an iterator that iterates over the cross samples of the [CrossSeries].
// [storage.Series] interface implementation.
func (s *CrossSeries) Iterator(it chunkenc.Iterator) chunkenc.Iterator {
	chunkIterator, ok := it.(*CrossChunkIterator)
	if !ok {
		return NewCrossChunkIterator(
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

// Labels returns the labels of the [CrossSeries].
// [storage.Series] interface implementation.
func (s *CrossSeries) Labels() labels.Labels {
	return s.labelSet
}

//
// CrossChunkIterator
//

// CrossChunkIterator iterates over the cross samples of a time series, that can only get the next value.
type CrossChunkIterator struct {
	serializedData *cppbridge.DataStorageSerializedData
	seriesGroups   *cppbridge.SeriesGroups
	chunkIterator  cppbridge.DataStorageSerializedDataMultiSeriesIterator
	mint           int64
	maxt           int64
	isInitialized  bool
}

// NewCrossChunkIterator initializes a new [CrossChunkIterator].
func NewCrossChunkIterator(
	serializedData *cppbridge.DataStorageSerializedData,
	seriesGroups *cppbridge.SeriesGroups,
	groupIndex int,
	mint, maxt int64,
) *CrossChunkIterator {
	it := &CrossChunkIterator{
		serializedData: serializedData,
		seriesGroups:   seriesGroups,
		chunkIterator: cppbridge.NewDataStorageSerializedDataMultiSeriesIterator(
			serializedData,
			seriesGroups.Groups[groupIndex],
		),
		mint: mint,
		maxt: maxt,
	}

	runtime.SetFinalizer(it, func(iter *CrossChunkIterator) {
		iter.chunkIterator.Close()
		iter.serializedData = nil
		iter.seriesGroups = nil
	})

	return it
}

// At returns the current timestamp/value pair if the value is a float.
// [chunkenc.Iterator] interface implementation.
//
//nolint:gocritic // unnamedResult not need
func (it *CrossChunkIterator) At() (int64, float64) {
	return it.chunkIterator.Timestamp(), it.chunkIterator.Value()
}

// AtFloatHistogram returns the current timestamp/value pair if the value is a histogram with floating-point counts.
// [chunkenc.Iterator] interface implementation.
func (*CrossChunkIterator) AtFloatHistogram(*histogram.FloatHistogram) (int64, *histogram.FloatHistogram) {
	return 0, nil
}

// AtHistogram returns the current timestamp/value pair if the value is a histogram with integer counts.
// [chunkenc.Iterator] interface implementation.
func (*CrossChunkIterator) AtHistogram(*histogram.Histogram) (int64, *histogram.Histogram) {
	return 0, nil
}

// AtT returns the current timestamp.
// [chunkenc.Iterator] interface implementation.
func (it *CrossChunkIterator) AtT() int64 {
	return it.chunkIterator.Timestamp()
}

// Err returns the current error - always nil.
// [chunkenc.Iterator] interface implementation.
func (*CrossChunkIterator) Err() error {
	return nil
}

// Next advances the iterator by one and returns the type of the value.
// [chunkenc.Iterator] interface implementation.
func (it *CrossChunkIterator) Next() chunkenc.ValueType {
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
func (it *CrossChunkIterator) Seek(t int64) chunkenc.ValueType {
	it.isInitialized = true
	if t > it.AtT() {
		return it.Next()
	}

	if it.AtT() > it.maxt {
		return chunkenc.ValNone
	}

	return chunkenc.ValFloat
}

// nextValue advances the iterator by one and returns the type of the value.
func (it *CrossChunkIterator) nextValue() chunkenc.ValueType {
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
func (it *CrossChunkIterator) reset(
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
	it.chunkIterator.Reset(serializedData, seriesGroups.Groups[groupIndex])
}

//
// crossLabelSetCtor
//

const (
	// labelHeadIDShardID is the label name for the head ID and shard ID.
	labelHeadIDShardID = "__head__shard_id"
)

var (
	// groupingPool is a pool of slices for sorted grouping.
	groupingPool = pool.NewSlicePool[string]([]int{2, 3, 5})

	// errGroupingLabelsIsEnough is the error returned when the grouping labels is enough.
	errGroupingLabelsIsEnough = errors.New("grouping labels is enough")
)

// crossLabelSetCtor constructs the label set for a cross series.
func crossLabelSetCtor(
	sb *labels.ScratchBuilder,
	snapshot *cppbridge.LabelSetSnapshot,
	grouping []string,
	headID string,
	seriesID uint32,
	shardID uint16,
) labels.Labels {
	sb.Add(labelHeadIDShardID, fmt.Sprintf("%s__%d", headID, shardID))

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
