package querier_test

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	ppstorage "github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

const (
	// mergeLifetimeMetric matches every series in the two-head fixture.
	mergeLifetimeMetric = "merge_lifetime_metric"
	// mergeLifetimeSeriesIDs defines the expected series order after merging the two heads.
	mergeLifetimeSeriesIDs = "abcdef"
	// mergeLifetimeMaxt is the inclusive upper bound of the fixture's queries.
	mergeLifetimeMaxt = 10
)

// querierWithCloseError reports a child close failure after closing the real head querier.
type querierWithCloseError struct {
	storage.Querier
	err error
}

// Close releases the underlying query resources before reporting the injected error.
func (q querierWithCloseError) Close() error {
	return errors.Join(q.Querier.Close(), q.err)
}

func TestMultiQuerierClose(t *testing.T) {
	t.Parallel()

	// Arrange
	leftHead, rightHead := newMergeLifetimeHeads(t)
	leftError := errors.New("left close failed")
	rightError := errors.New("right close failed")
	left := querier.NewMultiQuerier([]storage.Querier{
		querierWithCloseError{Querier: newMergeLifetimeQuerier(leftHead), err: leftError},
	})
	right := querier.NewMultiQuerier([]storage.Querier{
		querierWithCloseError{Querier: newMergeLifetimeQuerier(rightHead), err: rightError},
	})
	merged := querier.NewMultiQuerier([]storage.Querier{left, right})
	t.Cleanup(func() {
		if err := merged.Close(); err != nil {
			require.ErrorIs(t, err, leftError)
			require.ErrorIs(t, err, rightError)
		}
	})

	ctx := t.Context()
	match := labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, mergeLifetimeMetric)
	leftResult := left.Select(ctx, true, nil, match)
	rightResult := right.Select(ctx, true, nil, match)

	// Act
	unstarted := merged.Select(ctx, false, nil, match)
	prefetched := merged.Select(ctx, false, nil, match)
	require.True(t, prefetched.Next())
	series := prefetched.At()
	retainedLabels := series.Labels()
	iterator := series.Iterator(nil)
	require.Equal(t, chunkenc.ValFloat, iterator.Next())

	var ids []byte
	full := merged.Select(ctx, false, nil, match)
	for full.Next() {
		ids = append(ids, full.At().Labels().Get("id")...)
	}
	require.NoError(t, full.Err())

	closeErr := merged.Close()

	// Assert
	require.Equal(t, mergeLifetimeSeriesIDs, string(ids))
	require.ErrorIs(t, closeErr, leftError)
	require.ErrorIs(t, closeErr, rightError)
	require.NoError(t, merged.Close())
	require.False(t, unstarted.Next())
	require.False(t, prefetched.Next())
	require.Nil(t, prefetched.At())
	require.False(t, leftResult.Next())
	require.False(t, rightResult.Next())
	require.Equal(t, mergeLifetimeLabels("a"), retainedLabels)
	require.Equal(t, chunkenc.ValNone, iterator.Next())
	require.Equal(t, chunkenc.ValNone, series.Iterator(nil).Next())
}

func mergeLifetimeLabels(id string) labels.Labels {
	return labels.FromStrings(labels.MetricName, mergeLifetimeMetric, "id", id)
}

func newMergeLifetimeHeads(t *testing.T) (*ppstorage.Head, *ppstorage.Head) {
	t.Helper()

	dataDir := t.TempDir()
	fileLog, err := catalog.NewFileLogV2(filepath.Join(dataDir, "catalog.log"))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, fileLog.Close()) })

	headCatalog, err := catalog.New(
		clockwork.NewFakeClock(),
		fileLog,
		catalog.DefaultIDGenerator{},
		catalog.DefaultMaxLogFileSize,
		nil,
	)
	require.NoError(t, err)
	builder := ppstorage.NewBuilder(
		headCatalog,
		dataDir,
		storagetest.MaxSegmentSize,
		prometheus.NewRegistry(),
		storagetest.UnloadDataStorageInterval,
	)
	var heads [2]*ppstorage.Head
	for i := range heads {
		head, buildErr := builder.Build(uint64(i), 1) // #nosec G115 -- two fixture heads.
		require.NoError(t, buildErr)
		t.Cleanup(func() { require.NoError(t, head.Close()) })
		heads[i] = head
	}

	var rows [2][]storagetest.TimeSeries
	for i, id := range mergeLifetimeSeriesIDs {
		rows[i%2] = append(rows[i%2], storagetest.TimeSeries{
			Labels: mergeLifetimeLabels(string(id)),
			Samples: []cppbridge.Sample{
				{Timestamp: 1, Value: float64(i + 1)},
				{Timestamp: 2, Value: float64(i + 2)},
			},
		})
	}
	requireNoError := require.New(t).NoError
	for i, head := range heads {
		storagetest.MustAppendTimeSeries(
			t.Context(),
			requireNoError,
			head,
			rows[i],
		)
	}

	return heads[0], heads[1]
}

func newMergeLifetimeQuerier(head *ppstorage.Head) storage.Querier {
	return querier.NewQuerierWithOutSelectFuncOptimize(
		head,
		querier.NewNoOpShardedDeduplicator,
		0,
		mergeLifetimeMaxt,
		1,
		1,
		nil,
	)
}
