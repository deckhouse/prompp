package lcompactor

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/thanos/pkg/block/metadata"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/block"
	"github.com/prometheus/prometheus/pp-pkg/blocks/testutils"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
	"github.com/prometheus/prometheus/tsdb/index"
	"github.com/prometheus/prometheus/tsdb/tombstones"
	"github.com/prometheus/prometheus/tsdb/tsdbutil"
)

func TestCompactionRanges(t *testing.T) {
	t.Parallel()

	t.Run("without max duration", func(t *testing.T) {
		t.Parallel()
		ranges := CompactionRanges(2*60*60*1000, 0)
		require.Equal(t, []int64{
			2 * 60 * 60 * 1000,
			6 * 60 * 60 * 1000,
			18 * 60 * 60 * 1000,
			54 * 60 * 60 * 1000,
			162 * 60 * 60 * 1000,
			486 * 60 * 60 * 1000,
			1458 * 60 * 60 * 1000,
			4374 * 60 * 60 * 1000,
			13122 * 60 * 60 * 1000,
			39366 * 60 * 60 * 1000,
		}, ranges)
	})

	t.Run("with max duration", func(t *testing.T) {
		t.Parallel()
		ranges := CompactionRanges(2*60*60*1000, 31*24*60*60*1000)
		require.Equal(t, []int64{
			2 * 60 * 60 * 1000,
			6 * 60 * 60 * 1000,
			18 * 60 * 60 * 1000,
			54 * 60 * 60 * 1000,
			162 * 60 * 60 * 1000,
			486 * 60 * 60 * 1000,
		}, ranges)
	})

	t.Run("max lower than min is normalized", func(t *testing.T) {
		t.Parallel()
		ranges := CompactionRanges(2*60*60*1000, 60*60*1000)
		require.Equal(t, []int64{2 * 60 * 60 * 1000}, ranges)
	})
}

// See https://github.com/prometheus/prometheus/issues/3064
func TestSplitByRange(t *testing.T) {
	cases := []struct {
		trange int64
		ranges [][2]int64
		output [][][2]int64
	}{
		{
			trange: 60,
			ranges: [][2]int64{{0, 10}},
			output: [][][2]int64{
				{{0, 10}},
			},
		},
		{
			trange: 60,
			ranges: [][2]int64{{0, 60}},
			output: [][][2]int64{
				{{0, 60}},
			},
		},
		{
			trange: 60,
			ranges: [][2]int64{{0, 10}, {9, 15}, {30, 60}},
			output: [][][2]int64{
				{{0, 10}, {9, 15}, {30, 60}},
			},
		},
		{
			trange: 60,
			ranges: [][2]int64{{70, 90}, {125, 130}, {130, 180}, {1000, 1001}},
			output: [][][2]int64{
				{{70, 90}},
				{{125, 130}, {130, 180}},
				{{1000, 1001}},
			},
		},
		// Mis-aligned or too-large blocks are ignored.
		{
			trange: 60,
			ranges: [][2]int64{{50, 70}, {70, 80}},
			output: [][][2]int64{
				{{70, 80}},
			},
		},
		{
			trange: 72,
			ranges: [][2]int64{{0, 144}, {144, 216}, {216, 288}},
			output: [][][2]int64{
				{{144, 216}},
				{{216, 288}},
			},
		},
		// Various awkward edge cases easy to hit with negative numbers.
		{
			trange: 60,
			ranges: [][2]int64{{-10, -5}},
			output: [][][2]int64{
				{{-10, -5}},
			},
		},
		{
			trange: 60,
			ranges: [][2]int64{{-60, -50}, {-10, -5}},
			output: [][][2]int64{
				{{-60, -50}, {-10, -5}},
			},
		},
		{
			trange: 60,
			ranges: [][2]int64{{-60, -50}, {-10, -5}, {0, 15}},
			output: [][][2]int64{
				{{-60, -50}, {-10, -5}},
				{{0, 15}},
			},
		},
	}

	for _, c := range cases {
		// Transform input range tuples into dirMetas.
		blocks := make([]dirMeta, 0, len(c.ranges))
		for _, r := range c.ranges {
			blocks = append(blocks, dirMeta{
				meta: &metadata.Meta{BlockMeta: tsdb.BlockMeta{
					MinTime: r[0],
					MaxTime: r[1],
				}},
			})
		}

		// Transform output range tuples into dirMetas.
		exp := make([][]dirMeta, len(c.output))
		for i, group := range c.output {
			for _, r := range group {
				exp[i] = append(exp[i], dirMeta{
					meta: &metadata.Meta{BlockMeta: tsdb.BlockMeta{MinTime: r[0], MaxTime: r[1]}},
				})
			}
		}

		require.Equal(t, exp, splitByRange(blocks, c.trange))
	}
}

func TestLeveledCompactor_plan(t *testing.T) {
	// This mimics our default ExponentialBlockRanges with min block size equals to 20.
	compactor, err := NewLeveledCompactor(t.Context(), nil, nil, []int64{
		20,
		60,
		180,
		540,
		1620,
	}, nil, nil)
	require.NoError(t, err)

	cases := map[string]struct {
		metas    []dirMeta
		expected []string
	}{
		"Outside Range": {
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
			},
			expected: nil,
		},
		"We should wait for four blocks of size 20 to appear before compacting.": {
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
			},
			expected: nil,
		},
		`We should wait for a next block of size 20 to appear before compacting
		the existing ones. We have three, but we ignore the fresh one from WAl`: {
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 40, 60, nil),
			},
			expected: nil,
		},
		"Block to fill the entire parent range appeared – should be compacted": {
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 40, 60, nil),
				metaRange("4", 60, 80, nil),
			},
			expected: []string{"1", "2", "3"},
		},
		`Block for the next parent range appeared with gap with size 20. Nothing will happen in the first one
		anymore but we ignore fresh one still, so no compaction`: {
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 60, 80, nil),
			},
			expected: nil,
		},
		`Block for the next parent range appeared, and we have a gap with size 20 between second and third block.
		We will not get this missed gap anymore and we should compact just these two.`: {
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 60, 80, nil),
				metaRange("4", 80, 100, nil),
			},
			expected: []string{"1", "2"},
		},
		"We have 20, 20, 20, 60, 60 range blocks. '5' is marked as fresh one": {
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 40, 60, nil),
				metaRange("4", 60, 120, nil),
				metaRange("5", 120, 180, nil),
			},
			expected: []string{"1", "2", "3"},
		},
		"We have 20, 60, 20, 60, 240 range blocks. We can compact 20 + 60 + 60": {
			metas: []dirMeta{
				metaRange("2", 20, 40, nil),
				metaRange("4", 60, 120, nil),
				metaRange("5", 960, 980, nil), // Fresh one.
				metaRange("6", 120, 180, nil),
				metaRange("7", 720, 960, nil),
			},
			expected: []string{"2", "4", "6"},
		},
		"Do not select large blocks that have many tombstones when there is no fresh block": {
			metas: []dirMeta{
				metaRange("1", 0, 540, &tsdb.BlockStats{
					NumSeries:     10,
					NumTombstones: 3,
				}),
			},
			expected: nil,
		},
		"Select large blocks that have many tombstones when fresh appears": {
			metas: []dirMeta{
				metaRange("1", 0, 540, &tsdb.BlockStats{
					NumSeries:     10,
					NumTombstones: 3,
				}),
				metaRange("2", 540, 560, nil),
			},
			expected: []string{"1"},
		},
		"For small blocks, do not compact tombstones, even when fresh appears.": {
			metas: []dirMeta{
				metaRange("1", 0, 60, &tsdb.BlockStats{
					NumSeries:     10,
					NumTombstones: 3,
				}),
				metaRange("2", 60, 80, nil),
			},
			expected: nil,
		},
		`Regression test: we were stuck in a compact loop where we always recompacted
		the same block when tombstones and series counts were zero`: {
			metas: []dirMeta{
				metaRange("1", 0, 540, &tsdb.BlockStats{
					NumSeries:     0,
					NumTombstones: 0,
				}),
				metaRange("2", 540, 560, nil),
			},
			expected: nil,
		},
		`Regression test: we were wrongly assuming that new block is fresh from WAL when its ULID is newest.
		We need to actually look on max time instead.

		With previous, wrong approach "8" block was ignored, so we were wrongly compacting 5 and 7 and introducing
		block overlaps`: {
			metas: []dirMeta{
				metaRange("5", 0, 360, nil),
				metaRange("6", 540, 560, nil), // Fresh one.
				metaRange("7", 360, 420, nil),
				metaRange("8", 420, 540, nil),
			},
			expected: []string{"7", "8"},
		},
		// |--------------|
		//               |----------------|
		//                                |--------------|
		"Overlapping blocks 1": {
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 19, 40, nil),
				metaRange("3", 40, 60, nil),
			},
			expected: []string{"1", "2"},
		},
		// |--------------|
		//                |--------------|
		//                        |--------------|
		"Overlapping blocks 2": {
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 30, 50, nil),
			},
			expected: []string{"2", "3"},
		},
		// |--------------|
		//         |---------------------|
		//                       |--------------|
		"Overlapping blocks 3": {
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 10, 40, nil),
				metaRange("3", 30, 50, nil),
			},
			expected: []string{"1", "2", "3"},
		},
		// |--------------|
		//               |--------------------------------|
		//                |--------------|
		//                               |--------------|
		"Overlapping blocks 4": {
			metas: []dirMeta{
				metaRange("5", 0, 360, nil),
				metaRange("6", 340, 560, nil),
				metaRange("7", 360, 420, nil),
				metaRange("8", 420, 540, nil),
			},
			expected: []string{"5", "6", "7", "8"},
		},
		// |--------------|
		//               |--------------|
		//                                            |--------------|
		//                                                          |--------------|
		"Overlapping blocks 5": {
			metas: []dirMeta{
				metaRange("1", 0, 10, nil),
				metaRange("2", 9, 20, nil),
				metaRange("3", 30, 40, nil),
				metaRange("4", 39, 50, nil),
			},
			expected: []string{"1", "2"},
		},
	}

	for title, c := range cases {
		if !t.Run(title, func(t *testing.T) {
			res, err := compactor.getPlan(c.metas)
			require.NoError(t, err)
			require.Equal(t, c.expected, res)
		}) {
			return
		}
	}
}

func TestRangeWithFailedCompactionWontGetSelected(t *testing.T) {
	compactor, err := NewLeveledCompactor(context.Background(), nil, nil, []int64{
		20,
		60,
		240,
		720,
		2160,
	}, nil, nil)
	require.NoError(t, err)

	cases := []struct {
		metas []dirMeta
	}{
		{
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 40, 60, nil),
				metaRange("4", 60, 80, nil),
			},
		},
		{
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 60, 80, nil),
				metaRange("4", 80, 100, nil),
			},
		},
		{
			metas: []dirMeta{
				metaRange("1", 0, 20, nil),
				metaRange("2", 20, 40, nil),
				metaRange("3", 40, 60, nil),
				metaRange("4", 60, 120, nil),
				metaRange("5", 120, 180, nil),
				metaRange("6", 180, 200, nil),
			},
		},
	}

	for _, c := range cases {
		c.metas[1].meta.Compaction.Failed = true
		res, err := compactor.getPlan(c.metas)
		require.NoError(t, err)

		require.Equal(t, []string(nil), res)
	}
}

func TestCompactionFailWillCleanUpTempDir(t *testing.T) {
	compactor, err := NewLeveledCompactor(context.Background(), nil, log.NewNopLogger(), []int64{
		20,
		60,
		240,
		720,
		2160,
	}, nil, nil)
	require.NoError(t, err)

	tmpdir := t.TempDir()

	require.Error(t, compactor.write(
		tmpdir,
		&tsdb.BlockMeta{},
		DefaultBlockPopulator{},
		block.WriteTSDBMetaFile,
		erringBReader{},
	))

	_, err = os.Stat(filepath.Join(tmpdir, tsdb.BlockMeta{}.ULID.String()) + tmpForCreationBlockDirSuffix)
	require.True(t, os.IsNotExist(err), "directory is not cleaned up")
}

//revive:disable-next-line:cognitive-complexity // this is a test function
//revive:disable-next-line:cyclomatic // this is a test function
func TestCompaction_populateBlock(t *testing.T) {
	for _, tc := range []struct {
		title              string
		inputSeriesSamples [][]testutils.SeriesSamplesTest
		compactMinTime     int64
		compactMaxTime     int64 // When not defined the test runner sets a default of math.MaxInt64.
		irPostingsFunc     tsdb.IndexReaderPostingsFunc
		expSeriesSamples   []testutils.SeriesSamplesTest
		expErr             error
	}{
		{
			title:              "Populate block from empty input should return error.",
			inputSeriesSamples: [][]testutils.SeriesSamplesTest{},
			expErr:             errors.New("cannot populate block from no readers"),
		},
		{
			// Populate from single block without chunks. We expect these kind of series being ignored.
			inputSeriesSamples: [][]testutils.SeriesSamplesTest{
				{{Lset: map[string]string{"a": "b"}}},
			},
		},
		{
			title: "Populate from single block. We expect the same samples at the output.",
			inputSeriesSamples: [][]testutils.SeriesSamplesTest{
				{
					{
						Lset:   map[string]string{"a": "b"},
						Chunks: [][]testutils.SampleTest{{{TS: 0}, {TS: 10}}, {{TS: 11}, {TS: 20}}},
					},
				},
			},
			expSeriesSamples: []testutils.SeriesSamplesTest{
				{
					Lset:   map[string]string{"a": "b"},
					Chunks: [][]testutils.SampleTest{{{TS: 0}, {TS: 10}}, {{TS: 11}, {TS: 20}}},
				},
			},
		},
		{
			title: "Populate from two blocks.",
			inputSeriesSamples: [][]testutils.SeriesSamplesTest{
				{
					{
						Lset:   map[string]string{"a": "b"},
						Chunks: [][]testutils.SampleTest{{{TS: 0}, {TS: 10}}, {{TS: 11}, {TS: 20}}},
					},
					{
						Lset:   map[string]string{"a": "c"},
						Chunks: [][]testutils.SampleTest{{{TS: 1}, {TS: 9}}, {{TS: 10}, {TS: 19}}},
					},
					{
						// no-chunk series should be dropped.
						Lset: map[string]string{"a": "empty"},
					},
				},
				{
					{
						Lset:   map[string]string{"a": "b"},
						Chunks: [][]testutils.SampleTest{{{TS: 21}, {TS: 30}}},
					},
					{
						Lset:   map[string]string{"a": "c"},
						Chunks: [][]testutils.SampleTest{{{TS: 40}, {TS: 45}}},
					},
				},
			},
			expSeriesSamples: []testutils.SeriesSamplesTest{
				{
					Lset:   map[string]string{"a": "b"},
					Chunks: [][]testutils.SampleTest{{{TS: 0}, {TS: 10}}, {{TS: 11}, {TS: 20}}, {{TS: 21}, {TS: 30}}},
				},
				{
					Lset:   map[string]string{"a": "c"},
					Chunks: [][]testutils.SampleTest{{{TS: 1}, {TS: 9}}, {{TS: 10}, {TS: 19}}, {{TS: 40}, {TS: 45}}},
				},
			},
		},
		{
			title: "Populate from two blocks; chunks with negative time.",
			inputSeriesSamples: [][]testutils.SeriesSamplesTest{
				{
					{
						Lset:   map[string]string{"a": "b"},
						Chunks: [][]testutils.SampleTest{{{TS: 0}, {TS: 10}}, {{TS: 11}, {TS: 20}}},
					},
					{
						Lset:   map[string]string{"a": "c"},
						Chunks: [][]testutils.SampleTest{{{TS: -11}, {TS: -9}}, {{TS: 10}, {TS: 19}}},
					},
					{
						// no-chunk series should be dropped.
						Lset: map[string]string{"a": "empty"},
					},
				},
				{
					{
						Lset:   map[string]string{"a": "b"},
						Chunks: [][]testutils.SampleTest{{{TS: 21}, {TS: 30}}},
					},
					{
						Lset:   map[string]string{"a": "c"},
						Chunks: [][]testutils.SampleTest{{{TS: 40}, {TS: 45}}},
					},
				},
			},
			compactMinTime: -11,
			expSeriesSamples: []testutils.SeriesSamplesTest{
				{
					Lset:   map[string]string{"a": "b"},
					Chunks: [][]testutils.SampleTest{{{TS: 0}, {TS: 10}}, {{TS: 11}, {TS: 20}}, {{TS: 21}, {TS: 30}}},
				},
				{
					Lset:   map[string]string{"a": "c"},
					Chunks: [][]testutils.SampleTest{{{TS: -11}, {TS: -9}}, {{TS: 10}, {TS: 19}}, {{TS: 40}, {TS: 45}}},
				},
			},
		},
		{
			title: "Populate from two blocks showing that order is maintained.",
			inputSeriesSamples: [][]testutils.SeriesSamplesTest{
				{
					{
						Lset:   map[string]string{"a": "b"},
						Chunks: [][]testutils.SampleTest{{{TS: 0}, {TS: 10}}, {{TS: 11}, {TS: 20}}},
					},
					{
						Lset:   map[string]string{"a": "c"},
						Chunks: [][]testutils.SampleTest{{{TS: 1}, {TS: 9}}, {{TS: 10}, {TS: 19}}},
					},
				},
				{
					{
						Lset:   map[string]string{"a": "b"},
						Chunks: [][]testutils.SampleTest{{{TS: 21}, {TS: 30}}},
					},
					{
						Lset:   map[string]string{"a": "c"},
						Chunks: [][]testutils.SampleTest{{{TS: 40}, {TS: 45}}},
					},
				},
			},
			expSeriesSamples: []testutils.SeriesSamplesTest{
				{
					Lset:   map[string]string{"a": "b"},
					Chunks: [][]testutils.SampleTest{{{TS: 0}, {TS: 10}}, {{TS: 11}, {TS: 20}}, {{TS: 21}, {TS: 30}}},
				},
				{
					Lset:   map[string]string{"a": "c"},
					Chunks: [][]testutils.SampleTest{{{TS: 1}, {TS: 9}}, {{TS: 10}, {TS: 19}}, {{TS: 40}, {TS: 45}}},
				},
			},
		},
		{
			title: "Populate from two blocks showing that order of series is sorted.",
			inputSeriesSamples: [][]testutils.SeriesSamplesTest{
				{
					{
						Lset:   map[string]string{"a": "4"},
						Chunks: [][]testutils.SampleTest{{{TS: 5}, {TS: 7}}},
					},
					{
						Lset:   map[string]string{"a": "3"},
						Chunks: [][]testutils.SampleTest{{{TS: 5}, {TS: 6}}},
					},
					{
						Lset:   map[string]string{"a": "same"},
						Chunks: [][]testutils.SampleTest{{{TS: 1}, {TS: 4}}},
					},
				},
				{
					{
						Lset:   map[string]string{"a": "2"},
						Chunks: [][]testutils.SampleTest{{{TS: 1}, {TS: 3}}},
					},
					{
						Lset:   map[string]string{"a": "1"},
						Chunks: [][]testutils.SampleTest{{{TS: 1}, {TS: 2}}},
					},
					{
						Lset:   map[string]string{"a": "same"},
						Chunks: [][]testutils.SampleTest{{{TS: 5}, {TS: 8}}},
					},
				},
			},
			expSeriesSamples: []testutils.SeriesSamplesTest{
				{
					Lset:   map[string]string{"a": "1"},
					Chunks: [][]testutils.SampleTest{{{TS: 1}, {TS: 2}}},
				},
				{
					Lset:   map[string]string{"a": "2"},
					Chunks: [][]testutils.SampleTest{{{TS: 1}, {TS: 3}}},
				},
				{
					Lset:   map[string]string{"a": "3"},
					Chunks: [][]testutils.SampleTest{{{TS: 5}, {TS: 6}}},
				},
				{
					Lset:   map[string]string{"a": "4"},
					Chunks: [][]testutils.SampleTest{{{TS: 5}, {TS: 7}}},
				},
				{
					Lset:   map[string]string{"a": "same"},
					Chunks: [][]testutils.SampleTest{{{TS: 1}, {TS: 4}}, {{TS: 5}, {TS: 8}}},
				},
			},
		},
		{
			title: "Populate from two blocks 1:1 duplicated chunks; with negative timestamps.",
			inputSeriesSamples: [][]testutils.SeriesSamplesTest{
				{
					{
						Lset:   map[string]string{"a": "1"},
						Chunks: [][]testutils.SampleTest{{{TS: 1}, {TS: 2}}, {{TS: 3}, {TS: 4}}},
					},
					{
						Lset: map[string]string{"a": "2"},
						Chunks: [][]testutils.SampleTest{
							{{TS: -3}, {TS: -2}}, {{TS: 1}, {TS: 3}, {TS: 4}}, {{TS: 5}, {TS: 6}},
						},
					},
				},
				{
					{
						Lset:   map[string]string{"a": "1"},
						Chunks: [][]testutils.SampleTest{{{TS: 3}, {TS: 4}}},
					},
					{
						Lset:   map[string]string{"a": "2"},
						Chunks: [][]testutils.SampleTest{{{TS: 1}, {TS: 3}, {TS: 4}}, {{TS: 7}, {TS: 8}}},
					},
				},
			},
			compactMinTime: -3,
			expSeriesSamples: []testutils.SeriesSamplesTest{
				{
					Lset:   map[string]string{"a": "1"},
					Chunks: [][]testutils.SampleTest{{{TS: 1}, {TS: 2}}, {{TS: 3}, {TS: 4}}},
				},
				{
					Lset: map[string]string{"a": "2"},
					Chunks: [][]testutils.SampleTest{
						{{TS: -3}, {TS: -2}}, {{TS: 1}, {TS: 3}, {TS: 4}}, {{TS: 5}, {TS: 6}}, {{TS: 7}, {TS: 8}},
					},
				},
			},
		},
		{
			// This should not happened because head block is making sure the chunks are not crossing block boundaries.
			// We used to return error, but now chunk is trimmed.
			title: "Populate from single block containing chunk outside of compact meta time range.",
			inputSeriesSamples: [][]testutils.SeriesSamplesTest{
				{
					{
						Lset:   map[string]string{"a": "b"},
						Chunks: [][]testutils.SampleTest{{{TS: 1}, {TS: 2}}, {{TS: 10}, {TS: 30}}},
					},
				},
			},
			compactMinTime: 0,
			compactMaxTime: 20,
			expSeriesSamples: []testutils.SeriesSamplesTest{
				{
					Lset:   map[string]string{"a": "b"},
					Chunks: [][]testutils.SampleTest{{{TS: 1}, {TS: 2}}, {{TS: 10}}},
				},
			},
		},
		{
			// Introduced by https://github.com/prometheus/tsdb/issues/347. We used to return error,
			// but now chunk is trimmed.
			title: "Populate from single block containing extra chunk",
			inputSeriesSamples: [][]testutils.SeriesSamplesTest{
				{
					{
						Lset:   map[string]string{"a": "issue347"},
						Chunks: [][]testutils.SampleTest{{{TS: 1}, {TS: 2}}, {{TS: 10}, {TS: 20}}},
					},
				},
			},
			compactMinTime: 0,
			compactMaxTime: 10,
			expSeriesSamples: []testutils.SeriesSamplesTest{
				{
					Lset:   map[string]string{"a": "issue347"},
					Chunks: [][]testutils.SampleTest{{{TS: 1}, {TS: 2}}},
				},
			},
		},
		{
			// Deduplication expected.
			// Introduced by pull/370 and pull/539.
			title: "Populate from two blocks containing duplicated chunk.",
			inputSeriesSamples: [][]testutils.SeriesSamplesTest{
				{
					{
						Lset:   map[string]string{"a": "b"},
						Chunks: [][]testutils.SampleTest{{{TS: 1}, {TS: 2}}, {{TS: 10}, {TS: 20}}},
					},
				},
				{
					{
						Lset:   map[string]string{"a": "b"},
						Chunks: [][]testutils.SampleTest{{{TS: 10}, {TS: 20}}},
					},
				},
			},
			expSeriesSamples: []testutils.SeriesSamplesTest{
				{
					Lset:   map[string]string{"a": "b"},
					Chunks: [][]testutils.SampleTest{{{TS: 1}, {TS: 2}}, {{TS: 10}, {TS: 20}}},
				},
			},
		},
		{
			// Introduced by https://github.com/prometheus/tsdb/pull/539.
			title: "Populate from three overlapping blocks.",
			inputSeriesSamples: [][]testutils.SeriesSamplesTest{
				{
					{
						Lset:   map[string]string{"a": "overlap-all"},
						Chunks: [][]testutils.SampleTest{{{TS: 19}, {TS: 30}}},
					},
					{
						Lset:   map[string]string{"a": "overlap-beginning"},
						Chunks: [][]testutils.SampleTest{{{TS: 0}, {TS: 5}}},
					},
					{
						Lset:   map[string]string{"a": "overlap-ending"},
						Chunks: [][]testutils.SampleTest{{{TS: 21}, {TS: 30}}},
					},
				},
				{
					{
						Lset:   map[string]string{"a": "overlap-all"},
						Chunks: [][]testutils.SampleTest{{{TS: 0}, {TS: 10}, {TS: 11}, {TS: 20}}},
					},
					{
						Lset:   map[string]string{"a": "overlap-beginning"},
						Chunks: [][]testutils.SampleTest{{{TS: 0}, {TS: 10}, {TS: 12}, {TS: 20}}},
					},
					{
						Lset:   map[string]string{"a": "overlap-ending"},
						Chunks: [][]testutils.SampleTest{{{TS: 0}, {TS: 10}, {TS: 13}, {TS: 20}}},
					},
				},
				{
					{
						Lset:   map[string]string{"a": "overlap-all"},
						Chunks: [][]testutils.SampleTest{{{TS: 27}, {TS: 35}}},
					},
					{
						Lset:   map[string]string{"a": "overlap-ending"},
						Chunks: [][]testutils.SampleTest{{{TS: 27}, {TS: 35}}},
					},
				},
			},
			expSeriesSamples: []testutils.SeriesSamplesTest{
				{
					Lset: map[string]string{"a": "overlap-all"},
					Chunks: [][]testutils.SampleTest{
						{{TS: 0}, {TS: 10}, {TS: 11}, {TS: 19}, {TS: 20}, {TS: 27}, {TS: 30}, {TS: 35}},
					},
				},
				{
					Lset:   map[string]string{"a": "overlap-beginning"},
					Chunks: [][]testutils.SampleTest{{{TS: 0}, {TS: 5}, {TS: 10}, {TS: 12}, {TS: 20}}},
				},
				{
					Lset: map[string]string{"a": "overlap-ending"},
					Chunks: [][]testutils.SampleTest{
						{{TS: 0}, {TS: 10}, {TS: 13}, {TS: 20}}, {{TS: 21}, {TS: 27}, {TS: 30}, {TS: 35}},
					},
				},
			},
		},
		{
			title: "Populate from three partially overlapping blocks with few full chunks.",
			inputSeriesSamples: [][]testutils.SeriesSamplesTest{
				{
					{
						Lset:   map[string]string{"a": "1", "b": "1"},
						Chunks: samplesForRange(0, 659, 120), // 5 chunks and half.
					},
					{
						Lset:   map[string]string{"a": "1", "b": "2"},
						Chunks: samplesForRange(0, 659, 120),
					},
				},
				{
					{
						Lset: map[string]string{"a": "1", "b": "2"},
						// two chunks overlapping with previous, two non overlapping and two overlapping with next block
						Chunks: samplesForRange(480, 1199, 120),
					},
					{
						Lset:   map[string]string{"a": "1", "b": "3"},
						Chunks: samplesForRange(480, 1199, 120),
					},
				},
				{
					{
						Lset:   map[string]string{"a": "1", "b": "2"},
						Chunks: samplesForRange(960, 1499, 120), // 5 chunks and half.
					},
					{
						Lset:   map[string]string{"a": "1", "b": "4"},
						Chunks: samplesForRange(960, 1499, 120),
					},
				},
			},
			expSeriesSamples: []testutils.SeriesSamplesTest{
				{
					Lset:   map[string]string{"a": "1", "b": "1"},
					Chunks: samplesForRange(0, 659, 120),
				},
				{
					Lset:   map[string]string{"a": "1", "b": "2"},
					Chunks: samplesForRange(0, 1499, 120),
				},
				{
					Lset:   map[string]string{"a": "1", "b": "3"},
					Chunks: samplesForRange(480, 1199, 120),
				},
				{
					Lset:   map[string]string{"a": "1", "b": "4"},
					Chunks: samplesForRange(960, 1499, 120),
				},
			},
		},
		{
			title: "Populate from three partially overlapping blocks with chunks that " +
				"are expected to merge into single big chunks.",
			inputSeriesSamples: [][]testutils.SeriesSamplesTest{
				{
					{
						Lset:   map[string]string{"a": "1", "b": "2"},
						Chunks: [][]testutils.SampleTest{{{TS: 0}, {TS: 6902464}}, {{TS: 6961968}, {TS: 7080976}}},
					},
				},
				{
					{
						Lset: map[string]string{"a": "1", "b": "2"},
						Chunks: [][]testutils.SampleTest{
							{{TS: 3600000}, {TS: 13953696}}, {{TS: 14042952}, {TS: 14221464}},
						},
					},
				},
				{
					{
						Lset: map[string]string{"a": "1", "b": "2"},
						Chunks: [][]testutils.SampleTest{
							{{TS: 10800000}, {TS: 14251232}}, {{TS: 14280984}, {TS: 14340488}},
						},
					},
				},
			},
			expSeriesSamples: []testutils.SeriesSamplesTest{
				{
					Lset: map[string]string{"a": "1", "b": "2"},
					Chunks: [][]testutils.SampleTest{
						{
							{TS: 0},
							{TS: 3600000},
							{TS: 6902464},
							{TS: 6961968},
							{TS: 7080976},
							{TS: 10800000},
							{TS: 13953696},
							{TS: 14042952},
							{TS: 14221464},
							{TS: 14251232},
						}, {
							{TS: 14280984}, {TS: 14340488},
						},
					},
				},
			},
		},
		{
			// Regression test for populateWithDelChunkSeriesIterator failing to set minTime on chunks.
			title:          "Populate from mixed type series and expect sample inside the interval only.",
			compactMinTime: 1,
			compactMaxTime: 11,
			inputSeriesSamples: [][]testutils.SeriesSamplesTest{
				{
					{
						Lset: map[string]string{"a": "1"},
						Chunks: [][]testutils.SampleTest{
							{
								{TS: 0, HM: tsdbutil.GenerateTestHistogram(0)},
								{TS: 1, HM: tsdbutil.GenerateTestHistogram(1)},
							},
							{{TS: 10, V: 1}, {TS: 11, V: 2}},
						},
					},
				},
			},
			expSeriesSamples: []testutils.SeriesSamplesTest{
				{
					Lset: map[string]string{"a": "1"},
					Chunks: [][]testutils.SampleTest{
						{{TS: 1, HM: tsdbutil.GenerateTestHistogram(1)}},
						{{TS: 10, V: 1}},
					},
				},
			},
		},
		{
			title: "Populate from single block with index reader postings function selecting different series. " +
				"Expect empty block.",
			inputSeriesSamples: [][]testutils.SeriesSamplesTest{
				{
					{
						Lset:   map[string]string{"a": "b"},
						Chunks: [][]testutils.SampleTest{{{TS: 0}, {TS: 10}}, {{TS: 11}, {TS: 20}}},
					},
				},
			},
			irPostingsFunc: func(ctx context.Context, reader tsdb.IndexReader) index.Postings {
				p, err := reader.Postings(ctx, "a", "c")
				if err != nil {
					return index.EmptyPostings()
				}
				return reader.SortedPostings(p)
			},
		},
		{
			title: "Populate from single block with index reader postings function selecting one series. " +
				"Expect partial block.",
			inputSeriesSamples: [][]testutils.SeriesSamplesTest{
				{
					{
						Lset:   map[string]string{"a": "b"},
						Chunks: [][]testutils.SampleTest{{{TS: 0}, {TS: 10}}, {{TS: 11}, {TS: 20}}},
					},
					{
						Lset:   map[string]string{"a": "c"},
						Chunks: [][]testutils.SampleTest{{{TS: 0}, {TS: 10}}, {{TS: 11}, {TS: 20}}},
					},
					{
						Lset:   map[string]string{"a": "d"},
						Chunks: [][]testutils.SampleTest{{{TS: 0}, {TS: 10}}, {{TS: 11}, {TS: 20}}},
					},
				},
			},
			irPostingsFunc: func(ctx context.Context, reader tsdb.IndexReader) index.Postings {
				p, err := reader.Postings(ctx, "a", "c", "d")
				if err != nil {
					return index.EmptyPostings()
				}
				return reader.SortedPostings(p)
			},
			expSeriesSamples: []testutils.SeriesSamplesTest{
				{
					Lset:   map[string]string{"a": "c"},
					Chunks: [][]testutils.SampleTest{{{TS: 0}, {TS: 10}}, {{TS: 11}, {TS: 20}}},
				},
				{
					Lset:   map[string]string{"a": "d"},
					Chunks: [][]testutils.SampleTest{{{TS: 0}, {TS: 10}}, {{TS: 11}, {TS: 20}}},
				},
			},
		},
	} {
		t.Run(tc.title, func(t *testing.T) {
			blocks := make([]tsdb.BlockReader, 0, len(tc.inputSeriesSamples))
			for _, b := range tc.inputSeriesSamples {
				ir, cr, mint, maxt := createIdxChkReaders(t, b)
				blocks = append(blocks, &mockBReader{ir: ir, cr: cr, mint: mint, maxt: maxt})
			}

			c, err := NewLeveledCompactor(context.Background(), nil, nil, []int64{0}, nil, nil)
			require.NoError(t, err)

			meta := &tsdb.BlockMeta{
				MinTime: tc.compactMinTime,
				MaxTime: tc.compactMaxTime,
			}
			if meta.MaxTime == 0 {
				meta.MaxTime = math.MaxInt64
			}

			iw := &mockIndexWriter{}
			blockPopulator := DefaultBlockPopulator{}
			irPostingsFunc := AllSortedPostings
			if tc.irPostingsFunc != nil {
				irPostingsFunc = tc.irPostingsFunc
			}
			err = blockPopulator.PopulateBlock(
				c.ctx,
				c.metrics,
				c.logger,
				c.chunkPool,
				c.mergeFunc,
				blocks,
				meta,
				iw,
				nopChunkWriter{},
				irPostingsFunc,
			)
			if tc.expErr != nil {
				require.Error(t, err)
				require.Equal(t, tc.expErr.Error(), err.Error())
				return
			}
			require.NoError(t, err)

			// Check if response is expected and chunk is valid.
			var raw []testutils.SeriesSamplesTest
			for _, s := range iw.seriesChunks {
				ss := testutils.SeriesSamplesTest{Lset: s.l.Map()}
				var iter chunkenc.Iterator
				for _, chk := range s.chunks {
					var (
						samples       = make([]testutils.SampleTest, 0, chk.Chunk.NumSamples())
						iter          = chk.Chunk.Iterator(iter) //nolint:govet // reuse iterator
						firstTs int64 = math.MaxInt64
						s       testutils.SampleTest
					)
					for vt := iter.Next(); vt != chunkenc.ValNone; vt = iter.Next() {
						switch vt {
						case chunkenc.ValFloat:
							s.TS, s.V = iter.At()
							samples = append(samples, s)
						case chunkenc.ValHistogram:
							s.TS, s.HM = iter.AtHistogram(nil)
							samples = append(samples, s)
						case chunkenc.ValFloatHistogram:
							s.TS, s.FHM = iter.AtFloatHistogram(nil)
							samples = append(samples, s)
						default:
							require.Fail(t, "unexpected value type")
						}
						if firstTs == math.MaxInt64 {
							firstTs = s.TS
						}
					}

					// Check if chunk has correct min, max times.
					require.Equal(
						t,
						firstTs,
						chk.MinTime,
						"chunk Meta %v does not match the first encoded sample timestamp: %v", chk, firstTs,
					)
					require.Equal(
						t,
						s.TS,
						chk.MaxTime,
						"chunk Meta %v does not match the last encoded sample timestamp %v", chk, s.TS,
					)

					require.NoError(t, iter.Err())
					ss.Chunks = append(ss.Chunks, samples)
				}
				raw = append(raw, ss)
			}
			require.Equal(t, tc.expSeriesSamples, raw)

			// Check if stats are calculated properly.
			s := tsdb.BlockStats{NumSeries: uint64(len(tc.expSeriesSamples))}
			for _, series := range tc.expSeriesSamples {
				s.NumChunks += uint64(len(series.Chunks))
				for _, chk := range series.Chunks {
					s.NumSamples += uint64(len(chk))
				}
			}

			require.Equal(t, s, meta.Stats)
		})
	}
}

func TestCompactBlockMetas(t *testing.T) {
	parent1 := ulid.MustNew(100, nil)
	parent2 := ulid.MustNew(200, nil)
	parent3 := ulid.MustNew(300, nil)
	parent4 := ulid.MustNew(400, nil)

	input := []*tsdb.BlockMeta{
		{ULID: parent1, MinTime: 1000, MaxTime: 2000, Compaction: tsdb.BlockMetaCompaction{
			Level: 2, Sources: []ulid.ULID{ulid.MustNew(1, nil), ulid.MustNew(10, nil)},
		}},
		{ULID: parent2, MinTime: 200, MaxTime: 500, Compaction: tsdb.BlockMetaCompaction{Level: 1}},
		{ULID: parent3, MinTime: 500, MaxTime: 2500, Compaction: tsdb.BlockMetaCompaction{
			Level: 3, Sources: []ulid.ULID{ulid.MustNew(5, nil), ulid.MustNew(6, nil)},
		}},
		{ULID: parent4, MinTime: 100, MaxTime: 900, Compaction: tsdb.BlockMetaCompaction{Level: 1}},
	}

	outUlid := ulid.MustNew(1000, nil)
	output := CompactBlockMetas(outUlid, input...)

	expected := &tsdb.BlockMeta{
		ULID:    outUlid,
		MinTime: 100,
		MaxTime: 2500,
		Stats:   tsdb.BlockStats{},
		Compaction: tsdb.BlockMetaCompaction{
			Level: 4,
			Sources: []ulid.ULID{
				ulid.MustNew(1, nil), ulid.MustNew(5, nil), ulid.MustNew(6, nil), ulid.MustNew(10, nil),
			},
			Parents: []tsdb.BlockDesc{
				{ULID: parent1, MinTime: 1000, MaxTime: 2000},
				{ULID: parent2, MinTime: 200, MaxTime: 500},
				{ULID: parent3, MinTime: 500, MaxTime: 2500},
				{ULID: parent4, MinTime: 100, MaxTime: 900},
			},
		},
	}

	require.Equal(t, expected, output)
}

//
// Benchmark
//

func BenchmarkCompaction(b *testing.B) {
	cases := []struct {
		ranges         [][2]int64
		compactionType string
	}{
		{
			ranges:         [][2]int64{{0, 100}, {200, 300}, {400, 500}, {600, 700}},
			compactionType: "normal",
		},
		{
			ranges:         [][2]int64{{0, 1000}, {2000, 3000}, {4000, 5000}, {6000, 7000}},
			compactionType: "normal",
		},
		{
			ranges:         [][2]int64{{0, 2000}, {3000, 5000}, {6000, 8000}, {9000, 11000}},
			compactionType: "normal",
		},
		{
			ranges:         [][2]int64{{0, 5000}, {6000, 11000}, {12000, 17000}, {18000, 23000}},
			compactionType: "normal",
		},
		// 40% overlaps.
		{
			ranges:         [][2]int64{{0, 100}, {60, 160}, {120, 220}, {180, 280}},
			compactionType: "vertical",
		},
		{
			ranges:         [][2]int64{{0, 1000}, {600, 1600}, {1200, 2200}, {1800, 2800}},
			compactionType: "vertical",
		},
		{
			ranges:         [][2]int64{{0, 2000}, {1200, 3200}, {2400, 4400}, {3600, 5600}},
			compactionType: "vertical",
		},
		{
			ranges:         [][2]int64{{0, 5000}, {3000, 8000}, {6000, 11000}, {9000, 14000}},
			compactionType: "vertical",
		},
	}

	nSeries := 10000
	for _, c := range cases {
		nBlocks := len(c.ranges)
		b.Run(fmt.Sprintf(
			"type=%s,blocks=%d,series=%d,samplesPerSeriesPerBlock=%d",
			c.compactionType,
			nBlocks,
			nSeries,
			c.ranges[0][1]-c.ranges[0][0]+1),
			func(b *testing.B) {
				dir := b.TempDir()
				blockDirs := make([]string, 0, len(c.ranges))
				var blocks []*block.Block
				for _, r := range c.ranges {
					block, err := block.OpenBlock(
						nil,
						testutils.CreateBlock(b, dir, testutils.GenSeries(nSeries, 10, r[0], r[1])),
						nil,
					)
					require.NoError(b, err)
					blocks = append(blocks, block)
					defer func() {
						require.NoError(b, block.Close())
					}()
					blockDirs = append(blockDirs, block.Dir())
				}

				c, err := NewLeveledCompactor(context.Background(), nil, log.NewNopLogger(), []int64{0}, nil, nil)
				require.NoError(b, err)

				b.ResetTimer()
				b.ReportAllocs()
				for i := 0; i < b.N; i++ {
					_, err = c.Compact(dir, blockDirs, blocks)
					require.NoError(b, err)
				}
			})
	}
}

//
// Test Helpers
//

// metaRange is a helper function to create a dirMeta with a given name, mint, maxt, and stats.
func metaRange(name string, mint, maxt int64, stats *tsdb.BlockStats) dirMeta {
	meta := &metadata.Meta{BlockMeta: tsdb.BlockMeta{MinTime: mint, MaxTime: maxt}}
	if stats != nil {
		meta.Stats = *stats
	}

	return dirMeta{
		dir:  name,
		meta: meta,
	}
}

// samplesForRange is a helper function to create a slice of samples for a given range and max samples per chunk.
func samplesForRange(minTime, maxTime int64, maxSamplesPerChunk int) (ret [][]testutils.SampleTest) {
	var curr []testutils.SampleTest
	for i := minTime; i <= maxTime; i++ {
		curr = append(curr, testutils.SampleTest{TS: i})
		if len(curr) >= maxSamplesPerChunk {
			ret = append(ret, curr)
			curr = []testutils.SampleTest{}
		}
	}
	if len(curr) > 0 {
		ret = append(ret, curr)
	}
	return ret
}

//
// erringBReader
//

// erringBReader is a mock block reader that returns errors for all methods.
type erringBReader struct{}

// Chunks returns an error.
func (erringBReader) Chunks() (tsdb.ChunkReader, error) { return nil, errors.New("chunks") }

// Index returns an error.
func (erringBReader) Index() (tsdb.IndexReader, error) { return nil, errors.New("index") }

// Meta returns a zero block meta.
func (erringBReader) Meta() tsdb.BlockMeta { return tsdb.BlockMeta{} }

// Size returns 0.
func (erringBReader) Size() int64 { return 0 }

// Tombstones returns an error.
func (erringBReader) Tombstones() (tombstones.Reader, error) { return nil, errors.New("tombstones") }

// Index: labels -> postings -> chunkMetas -> chunkRef.
// ChunkReader: ref -> vals.
//
//revive:disable-next-line:cognitive-complexity // this is a test function
//revive:disable-next-line:cyclomatic // this is a test function
//revive:disable-next-line:function-result-limit // this is a test function
//nolint:gocritic // unnamedResult // this is a test function
func createIdxChkReaders(
	t *testing.T,
	tc []testutils.SeriesSamplesTest,
) (tsdb.IndexReader, tsdb.ChunkReader, int64, int64) { //revive:disable-line:confusing-results // this is a test
	sort.Slice(tc, func(i, _ int) bool {
		return labels.Compare(labels.FromMap(tc[i].Lset), labels.FromMap(tc[i].Lset)) < 0
	})

	postings := index.NewMemPostings()
	chkReader := mockChunkReader(make(map[chunks.ChunkRef]chunkenc.Chunk))
	lblIdx := make(map[string]map[string]struct{})
	mi := newMockIndex()
	blockMint := int64(math.MaxInt64)
	blockMaxt := int64(math.MinInt64)

	var chunkRef chunks.ChunkRef
	for i, s := range tc {
		i++ // 0 is not a valid posting.
		metas := make([]chunks.Meta, 0, len(s.Chunks))
		for _, chk := range s.Chunks {
			if chk[0].TS < blockMint {
				blockMint = chk[0].TS
			}
			if chk[len(chk)-1].TS > blockMaxt {
				blockMaxt = chk[len(chk)-1].TS
			}

			metas = append(metas, chunks.Meta{
				MinTime: chk[0].TS,
				MaxTime: chk[len(chk)-1].TS,
				Ref:     chunkRef,
			})

			switch {
			case chk[0].FHM != nil:
				chunk := chunkenc.NewFloatHistogramChunk()
				app, _ := chunk.Appender()
				for _, smpl := range chk {
					require.NotNil(t, smpl.FHM, "chunk can only contain one type of sample")
					_, _, _, err := app.AppendFloatHistogram(nil, smpl.TS, smpl.FHM, true)
					require.NoError(t, err, "chunk should be appendable")
				}
				chkReader[chunkRef] = chunk
			case chk[0].HM != nil:
				chunk := chunkenc.NewHistogramChunk()
				app, _ := chunk.Appender()
				for _, smpl := range chk {
					require.NotNil(t, smpl.HM, "chunk can only contain one type of sample")
					_, _, _, err := app.AppendHistogram(nil, smpl.TS, smpl.HM, true)
					require.NoError(t, err, "chunk should be appendable")
				}
				chkReader[chunkRef] = chunk
			default:
				chunk := chunkenc.NewXORChunk()
				app, _ := chunk.Appender()
				for _, smpl := range chk {
					require.Nil(t, smpl.HM, "chunk can only contain one type of sample")
					require.Nil(t, smpl.FHM, "chunk can only contain one type of sample")
					app.Append(smpl.TS, smpl.V)
				}
				chkReader[chunkRef] = chunk
			}
			chunkRef++
		}
		ls := labels.FromMap(s.Lset)
		require.NoError(t, mi.AddSeries(storage.SeriesRef(i), ls, metas...))

		postings.Add(storage.SeriesRef(i), ls)

		ls.Range(func(l labels.Label) {
			vs, present := lblIdx[l.Name]
			if !present {
				vs = map[string]struct{}{}
				lblIdx[l.Name] = vs
			}
			vs[l.Value] = struct{}{}
		})
	}

	require.NoError(t, postings.Iter(func(l labels.Label, p index.Postings) error {
		return mi.WritePostings(l.Name, l.Value, p)
	}))
	return mi, chkReader, blockMint, blockMaxt
}

//
// mockChunkReader
//

// mockChunkReader is a mock chunk reader.
// It returns a chunk or an error if the chunk is not found.
type mockChunkReader map[chunks.ChunkRef]chunkenc.Chunk

// ChunkOrIterable returns a chunk or an error if the chunk is not found.
func (cr mockChunkReader) ChunkOrIterable(meta chunks.Meta) (chunkenc.Chunk, chunkenc.Iterable, error) {
	chk, ok := cr[meta.Ref]
	if ok {
		return chk, nil, nil
	}

	return nil, nil, errors.New("Chunk with ref not found")
}

// Close closes the chunk reader.
func (mockChunkReader) Close() error {
	return nil
}

//
// series
//

// series is a mock series.
// It stores labels and chunks.
type series struct {
	l      labels.Labels
	chunks []chunks.Meta
}

//
// mockIndex
//

// mockIndex is a mock index.
// It stores series and postings in memory.
type mockIndex struct {
	series   map[storage.SeriesRef]series
	postings map[labels.Label][]storage.SeriesRef
	symbols  map[string]struct{}
}

// newMockIndex creates a new [mockIndex].
func newMockIndex() mockIndex {
	ix := mockIndex{
		series:   make(map[storage.SeriesRef]series),
		postings: make(map[labels.Label][]storage.SeriesRef),
		symbols:  make(map[string]struct{}),
	}
	return ix
}

// AddSeries adds a series to the index.
func (m *mockIndex) AddSeries(ref storage.SeriesRef, l labels.Labels, chs ...chunks.Meta) error {
	if _, ok := m.series[ref]; ok {
		return fmt.Errorf("series with reference %d already added", ref)
	}
	l.Range(func(lbl labels.Label) {
		m.symbols[lbl.Name] = struct{}{}
		m.symbols[lbl.Value] = struct{}{}
	})

	s := series{l: l}
	// Actual chunk data is not stored in the index.
	for _, c := range chs {
		c.Chunk = nil
		s.chunks = append(s.chunks, c)
	}
	m.series[ref] = s

	return nil
}

// Close closes the index.
func (mockIndex) Close() error { return nil }

// LabelNames returns the sorted label names for a given matchers.
func (m mockIndex) LabelNames(_ context.Context, matchers ...*labels.Matcher) ([]string, error) {
	names := map[string]struct{}{}
	if len(matchers) == 0 {
		for l := range m.postings {
			names[l.Name] = struct{}{}
		}
	} else {
		for _, series := range m.series {
			matches := true
			for _, matcher := range matchers {
				matches = matches || matcher.Matches(series.l.Get(matcher.Name))
				if !matches {
					break
				}
			}
			if matches {
				series.l.Range(func(lbl labels.Label) {
					names[lbl.Name] = struct{}{}
				})
			}
		}
	}
	l := make([]string, 0, len(names))
	for name := range names {
		l = append(l, name)
	}
	sort.Strings(l)
	return l, nil
}

// LabelNamesFor returns the sorted label names for a given postings.
func (m mockIndex) LabelNamesFor(_ context.Context, postings index.Postings) ([]string, error) {
	namesMap := make(map[string]bool)
	for postings.Next() {
		m.series[postings.At()].l.Range(func(lbl labels.Label) {
			namesMap[lbl.Name] = true
		})
	}
	if err := postings.Err(); err != nil {
		return nil, err
	}
	names := make([]string, 0, len(namesMap))
	for name := range namesMap {
		names = append(names, name)
	}
	return names, nil
}

// LabelValueFor returns the label value for a given series ref and label name.
func (m mockIndex) LabelValueFor(_ context.Context, id storage.SeriesRef, label string) (string, error) {
	return m.series[id].l.Get(label), nil
}

// LabelValues returns the sorted label values for a given name and matchers.
func (m mockIndex) LabelValues(_ context.Context, name string, matchers ...*labels.Matcher) ([]string, error) {
	var values []string

	if len(matchers) == 0 {
		for l := range m.postings {
			if l.Name == name {
				values = append(values, l.Value)
			}
		}
		return values, nil
	}

	for _, series := range m.series {
		for _, matcher := range matchers {
			if matcher.Matches(series.l.Get(matcher.Name)) {
				values = append(values, series.l.Get(name))
			}
		}
	}

	return values, nil
}

// Postings returns the postings for a given name and values.
func (m mockIndex) Postings(ctx context.Context, name string, values ...string) (index.Postings, error) {
	res := make([]index.Postings, 0, len(values))
	for _, value := range values {
		l := labels.Label{Name: name, Value: value}
		res = append(res, index.NewListPostings(m.postings[l]))
	}
	return index.Merge(ctx, res...), nil
}

// PostingsForLabelMatching returns the postings for a given name and match function.
func (m mockIndex) PostingsForLabelMatching(ctx context.Context, name string, match func(string) bool) index.Postings {
	var res []index.Postings
	for l, srs := range m.postings {
		if l.Name == name && match(l.Value) {
			res = append(res, index.NewListPostings(srs))
		}
	}
	return index.Merge(ctx, res...)
}

// Series returns the series for a given series ref.
func (m mockIndex) Series(ref storage.SeriesRef, builder *labels.ScratchBuilder, chks *[]chunks.Meta) error {
	s, ok := m.series[ref]
	if !ok {
		return storage.ErrNotFound
	}
	builder.Assign(s.l)
	*chks = append((*chks)[:0], s.chunks...)

	return nil
}

// ShardedPostings returns the sharded postings for a given postings, shard index and shard count.
func (m mockIndex) ShardedPostings(p index.Postings, shardIndex, shardCount uint64) index.Postings {
	out := make([]storage.SeriesRef, 0, 128)

	for p.Next() {
		ref := p.At()
		s, ok := m.series[ref]
		if !ok {
			continue
		}

		// Check if the series belong to the shard.
		if s.l.Hash()%shardCount != shardIndex {
			continue
		}

		out = append(out, ref)
	}

	return index.NewListPostings(out)
}

// SortedLabelValues returns the sorted label values for a given name and matchers.
func (m mockIndex) SortedLabelValues(ctx context.Context, name string, matchers ...*labels.Matcher) ([]string, error) {
	values, _ := m.LabelValues(ctx, name, matchers...)
	sort.Strings(values)
	return values, nil
}

// SortedPostings returns the sorted postings for a given postings.
func (m mockIndex) SortedPostings(p index.Postings) index.Postings {
	ep, err := index.ExpandPostings(p)
	if err != nil {
		return index.ErrPostings(fmt.Errorf("expand postings: %w", err))
	}

	sort.Slice(ep, func(i, j int) bool {
		return labels.Compare(m.series[ep[i]].l, m.series[ep[j]].l) < 0
	})
	return index.NewListPostings(ep)
}

// Symbols returns an iterator over the symbols in the index.
func (m mockIndex) Symbols() index.StringIter {
	l := []string{}
	for s := range m.symbols {
		l = append(l, s)
	}
	sort.Strings(l)
	return index.NewStringListIter(l)
}

// WritePostings writes postings to the index.
func (m mockIndex) WritePostings(name, value string, it index.Postings) error {
	l := labels.Label{Name: name, Value: value}
	if _, ok := m.postings[l]; ok {
		return fmt.Errorf("postings for %s already added", l)
	}
	ep, err := index.ExpandPostings(it)
	if err != nil {
		return err
	}
	m.postings[l] = ep
	return nil
}

//
// mockIndexWriter
//

// copyChunk copies a chunk and returns a new chunk.
func copyChunk(c chunkenc.Chunk) (chunkenc.Chunk, error) {
	b := c.Bytes()
	nb := make([]byte, len(b))
	copy(nb, b)
	return chunkenc.FromData(c.Encoding(), nb)
}

// mockIndexWriter is a mock index writer.
// It writes series and chunks to an in-memory index.
type mockIndexWriter struct {
	seriesChunks []series
}

// AddSeries adds a series to the index.
func (m *mockIndexWriter) AddSeries(_ storage.SeriesRef, l labels.Labels, chks ...chunks.Meta) error {
	// Copy chunks as their bytes are pooled.
	chksNew := make([]chunks.Meta, len(chks))
	for i, chk := range chks {
		c, err := copyChunk(chk.Chunk)
		if err != nil {
			return fmt.Errorf("mockIndexWriter: copy chunk: %w", err)
		}
		chksNew[i] = chunks.Meta{MaxTime: chk.MaxTime, MinTime: chk.MinTime, Chunk: c}
	}

	// We don't combine multiple same series together, by design as `AddSeries` requires full series to be saved.
	m.seriesChunks = append(m.seriesChunks, series{l: l, chunks: chksNew})
	return nil
}

// AddSymbol adds a symbol to the index.
func (mockIndexWriter) AddSymbol(string) error { return nil }

// Close closes the index writer.
func (mockIndexWriter) Close() error { return nil }

// WriteLabelIndex writes a label index to the index.
func (mockIndexWriter) WriteLabelIndex([]string, []string) error { return nil }

//
// mockBReader
//

// mockBReader is a mock block reader.
// It returns an index reader, a chunk reader, a mint time and a maxt time.
type mockBReader struct {
	ir   tsdb.IndexReader
	cr   tsdb.ChunkReader
	mint int64
	maxt int64
}

// Chunks returns the chunk reader.
func (r *mockBReader) Chunks() (tsdb.ChunkReader, error) { return r.cr, nil }

// Index returns the index reader.
func (r *mockBReader) Index() (tsdb.IndexReader, error) { return r.ir, nil }

// Meta returns the block meta.
func (r *mockBReader) Meta() tsdb.BlockMeta { return tsdb.BlockMeta{MinTime: r.mint, MaxTime: r.maxt} }

// Size returns the size of the block.
func (*mockBReader) Size() int64 { return 0 }

// Tombstones returns the tombstones reader.
func (*mockBReader) Tombstones() (tombstones.Reader, error) {
	return tombstones.NewMemTombstones(), nil
}

//
// nopChunkWriter
//

// nopChunkWriter is a mock chunk writer.
type nopChunkWriter struct{}

// Close closes the chunk writer.
func (nopChunkWriter) Close() error { return nil }

// WriteChunks writes chunks to the chunk writer.
func (nopChunkWriter) WriteChunks(...chunks.Meta) error { return nil }
