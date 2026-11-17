package manager

import (
	"math"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/block"
	"github.com/prometheus/prometheus/pp-pkg/blocks/testutils"
	"github.com/prometheus/prometheus/pp-pkg/blocks/upsampler"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

const (
	// selectionCoarseResolution exceeds the configured resolution to exercise coarsest-source reporting.
	selectionCoarseResolution = int64(120_000)
	// selectionConfiguredResolution enables downsampling independently of the stored resolution.
	selectionConfiguredResolution = int64(60_000)
	// selectionStoredResolution marks the downsampled fixtures with their stored resolution.
	selectionStoredResolution = int64(1_000)
	// selectionMetric identifies the shared series whose values distinguish raw and downsampled blocks.
	selectionMetric = "selection_metric"
)

// selectionSample records an observable timestamp and float value, without block provenance metadata.
type selectionSample struct {
	t int64
	v float64
}

// selectionBlock describes a real TSDB block with optional downsampling metadata.
type selectionBlock struct {
	samples    []selectionSample
	resolution int64
}

// selectionQuery describes the original query range and expected selected samples and resolution.
type selectionQuery struct {
	mint       int64
	maxt       int64
	samples    []selectionSample
	resolution int64
}

// ManagerSelectionSuite exercises version selection through both public query paths and block reloads.
type ManagerSelectionSuite struct {
	suite.Suite

	dir       string
	logger    log.Logger
	chunkPool chunkenc.Pool
}

func TestManagerSelectionSuite(t *testing.T) {
	suite.Run(t, new(ManagerSelectionSuite))
}

func (s *ManagerSelectionSuite) SetupTest() {
	s.dir = s.T().TempDir()
	s.logger = log.NewNopLogger()
	s.chunkPool = chunkenc.NewPool()
}

func (s *ManagerSelectionSuite) TestDownsamplingReportsCoarsestResolution() {
	// Arrange
	s.createBlock(selectionBlock{
		samples: []selectionSample{{1_000, 101}, {2_000, 102}}, resolution: selectionCoarseResolution,
	})
	s.createBlock(selectionBlock{
		samples: []selectionSample{{3_000, 103}, {4_000, 104}}, resolution: selectionStoredResolution,
	})
	m := s.openManager(nil, selectionConfiguredResolution)

	// Act
	// Assert
	s.assertQuery(m, selectionQuery{
		mint: 0, maxt: 5_000,
		samples:    []selectionSample{{1_000, 101}, {2_000, 102}, {3_000, 103}, {4_000, 104}},
		resolution: selectionCoarseResolution,
	})
}

func (s *ManagerSelectionSuite) TestFailedReloadPreservesReadableSelectionAndBlockState() {
	// Arrange
	rawID := s.createBlock(selectionBlock{samples: []selectionSample{{3_000, 30}, {4_000, 40}}})
	dsID := s.createBlock(selectionBlock{
		samples: []selectionSample{{1_000, 101}, {2_000, 102}}, resolution: selectionStoredResolution,
	})
	m := s.openManager(nil, selectionConfiguredResolution)
	query := selectionQuery{
		mint: 0, maxt: 5_000,
		samples: []selectionSample{{1_000, 101}, {2_000, 102}}, resolution: selectionConfiguredResolution,
	}
	s.assertQuery(m, query)
	movedDir := filepath.Join(s.T().TempDir(), "saved-blocks")
	s.Require().NoError(os.Rename(s.dir, movedDir))
	s.T().Cleanup(func() {
		s.Require().NoError(os.Remove(s.dir))
		s.Require().NoError(os.Rename(movedDir, s.dir))
	})
	s.Require().NoError(os.WriteFile(s.dir, []byte("not a directory"), 0o600))

	// Act
	err := m.reloadBlocks()

	// Assert
	s.Require().Error(err)
	s.assertBlockState(m, 3_000, []ulid.ULID{dsID, rawID})
	s.assertQuery(m, query)
	s.assertQuery(m, selectionQuery{
		mint: 3_000, maxt: 5_000, samples: []selectionSample{{3_000, 30}, {4_000, 40}},
	})
}

func (s *ManagerSelectionSuite) TestPredecessorExtensionDoesNotChangeSelectedVersion() {
	// Arrange
	s.createBlock(selectionBlock{samples: []selectionSample{{1_600, 16}, {1_800, 18}}})
	s.createBlock(selectionBlock{samples: []selectionSample{{3_000, 30}, {4_000, 40}}})
	s.createBlock(selectionBlock{
		samples: []selectionSample{{1_000, 101}, {1_500, 115}}, resolution: selectionStoredResolution,
	})
	s.createBlock(selectionBlock{
		samples: []selectionSample{{2_500, 125}, {3_500, 135}}, resolution: selectionStoredResolution,
	})
	m := s.openManager(nil, selectionConfiguredResolution)

	// Act
	// Assert
	s.assertQuery(m, selectionQuery{
		mint: 2_000, maxt: 4_500,
		samples:    []selectionSample{{1_000, 101}, {1_500, 115}, {2_500, 125}, {3_500, 135}},
		resolution: selectionConfiguredResolution,
	})
}

func (s *ManagerSelectionSuite) TestReloadAndRetentionTransitions() {
	// Arrange
	rawID := s.createBlock(selectionBlock{samples: []selectionSample{{3_000, 30}, {4_000, 40}}})
	dsID := s.createBlock(selectionBlock{
		samples: []selectionSample{{1_000, 101}, {2_000, 102}}, resolution: selectionStoredResolution,
	})
	var deletions sync.Map
	retention := func(blocks []*block.Block) map[ulid.ULID]struct{} {
		marked := make(map[ulid.ULID]struct{})

		for _, b := range blocks {
			if _, ok := deletions.Load(b.Meta().ULID); ok {
				marked[b.Meta().ULID] = struct{}{}
			}
		}

		return marked
	}
	m := s.openManager(retention, selectionConfiguredResolution)
	dsQuery := selectionQuery{
		mint: 0, maxt: 5_000,
		samples: []selectionSample{{1_000, 101}, {2_000, 102}}, resolution: selectionConfiguredResolution,
	}
	s.assertBlockState(m, 3_000, []ulid.ULID{dsID, rawID})
	s.assertQuery(m, dsQuery)

	// Act
	earlyRawID := s.createBlock(selectionBlock{samples: []selectionSample{{500, 5}, {750, 7.5}}})
	s.Require().NoError(m.reloadBlocks())

	// Assert
	s.assertBlockState(m, 500, []ulid.ULID{earlyRawID, dsID, rawID})
	s.assertQuery(m, selectionQuery{
		mint: 0, maxt: 5_000, samples: []selectionSample{{500, 5}, {750, 7.5}, {3_000, 30}, {4_000, 40}},
	})

	// Act
	deletions.Store(earlyRawID, struct{}{})
	s.Require().NoError(m.reloadBlocks())
	// All query readers above are closed before waiting for retention to delete their blocks.
	m.deleteBlocksWG.Wait()

	// Assert
	s.assertBlockState(m, 3_000, []ulid.ULID{dsID, rawID})
	s.assertQuery(m, dsQuery)
	s.Require().NoDirExists(filepath.Join(s.dir, earlyRawID.String()))

	// Act
	deletions.Store(rawID, struct{}{})
	s.Require().NoError(m.reloadBlocks())
	m.deleteBlocksWG.Wait()

	// Assert
	s.assertBlockState(m, math.MaxInt64, []ulid.ULID{dsID})
	s.assertQuery(m, dsQuery)
	s.Require().NoDirExists(filepath.Join(s.dir, rawID.String()))

	// Act
	deletions.Store(dsID, struct{}{})
	s.Require().NoError(m.reloadBlocks())
	m.deleteBlocksWG.Wait()

	// Assert
	s.assertBlockState(m, math.MaxInt64, nil)
	s.assertQuery(m, selectionQuery{mint: 0, maxt: 5_000})
	s.Require().NoDirExists(filepath.Join(s.dir, dsID.String()))

	// Act
	newRawID := s.createBlock(selectionBlock{samples: []selectionSample{{2_000, 20}, {4_000, 40}}})
	s.Require().NoError(m.reloadBlocks())

	// Assert
	s.assertBlockState(m, 2_000, []ulid.ULID{newRawID})
	s.assertQuery(m, selectionQuery{
		mint: 0, maxt: 5_000, samples: []selectionSample{{2_000, 20}, {4_000, 40}},
	})
}

func (s *ManagerSelectionSuite) TestSelectsOneOverlappingVersion() {
	// Arrange
	raw := selectionBlock{samples: []selectionSample{{2_000, 20}, {4_000, 40}}}
	ds := selectionBlock{
		samples: []selectionSample{{1_000, 101}, {3_000, 103}}, resolution: selectionStoredResolution,
	}
	cases := []struct {
		name     string
		blocks   []selectionBlock
		disabled bool
		query    selectionQuery
	}{
		{
			name: "raw only wide query", blocks: []selectionBlock{raw},
			query: selectionQuery{mint: 0, maxt: 5_000, samples: raw.samples},
		},
		{
			name: "downsampling starts earlier", blocks: []selectionBlock{raw, ds},
			query: selectionQuery{mint: 0, maxt: 5_000, samples: ds.samples, resolution: selectionConfiguredResolution},
		},
		{
			name: "raw starts at query mint", blocks: []selectionBlock{raw, ds},
			query: selectionQuery{mint: 2_000, maxt: 5_000, samples: raw.samples},
		},
		{
			name: "raw starts before downsampling",
			blocks: []selectionBlock{
				{samples: []selectionSample{{500, 5}, {4_000, 40}}}, ds,
			},
			query: selectionQuery{mint: 0, maxt: 5_000, samples: []selectionSample{{500, 5}, {4_000, 40}}},
		},
		{
			name: "equal starts prefer raw",
			blocks: []selectionBlock{raw, {
				samples: []selectionSample{{2_000, 120}, {4_000, 140}}, resolution: selectionStoredResolution,
			}},
			query: selectionQuery{mint: 0, maxt: 5_000, samples: raw.samples},
		},
		{
			name: "query inside both versions prefers raw", blocks: []selectionBlock{raw, ds},
			query: selectionQuery{mint: 2_500, maxt: 5_000, samples: []selectionSample{{4_000, 40}}},
		},
		{
			name: "partial downsampling does not replace raw A plus B",
			blocks: []selectionBlock{
				{samples: []selectionSample{{1_000, 10}, {2_000, 20}}},
				{samples: []selectionSample{{3_000, 30}, {4_000, 40}}},
				{samples: []selectionSample{{1_000, 110}, {2_000, 120}}, resolution: selectionStoredResolution},
			},
			query: selectionQuery{
				mint: 0, maxt: 5_000, samples: []selectionSample{{1_000, 10}, {2_000, 20}, {3_000, 30}, {4_000, 40}},
			},
		},
		{
			name: "older out of query downsampling does not hide raw",
			blocks: []selectionBlock{
				raw, {samples: []selectionSample{{100, 101}, {200, 102}}, resolution: selectionStoredResolution},
			},
			query: selectionQuery{mint: 1_000, maxt: 5_000, samples: raw.samples},
		},
		{
			name: "out of query raw does not hide downsampling",
			blocks: []selectionBlock{
				{samples: []selectionSample{{100, 1}, {200, 2}}},
				{samples: []selectionSample{{6_000, 60}, {7_000, 70}}},
				ds,
			},
			query: selectionQuery{mint: 500, maxt: 5_000, samples: ds.samples, resolution: selectionConfiguredResolution},
		},
		{
			name: "neither version overlaps", blocks: []selectionBlock{raw, ds},
			query: selectionQuery{mint: 8_000, maxt: 9_000},
		},
		{
			name: "disabled selects raw", blocks: []selectionBlock{raw, ds}, disabled: true,
			query: selectionQuery{mint: 0, maxt: 5_000, samples: raw.samples},
		},
		{
			name: "disabled downsampling only is empty", blocks: []selectionBlock{ds}, disabled: true,
			query: selectionQuery{mint: 0, maxt: 5_000},
		},
		{
			name: "downsampling only", blocks: []selectionBlock{ds},
			query: selectionQuery{mint: 0, maxt: 5_000, samples: ds.samples, resolution: selectionConfiguredResolution},
		},
		{
			name: "earlier downsampling intentionally omits raw right tail",
			blocks: []selectionBlock{raw, {
				samples: []selectionSample{{1_000, 101}, {2_000, 102}}, resolution: selectionStoredResolution,
			}},
			query: selectionQuery{
				mint: 0, maxt: 5_000,
				samples: []selectionSample{{1_000, 101}, {2_000, 102}}, resolution: selectionConfiguredResolution,
			},
		},
	}

	for _, tc := range cases {
		s.Run(tc.name, func() {
			// Arrange
			s.dir = s.T().TempDir()

			for _, fixture := range tc.blocks {
				s.createBlock(fixture)
			}

			configuredResolution := selectionConfiguredResolution

			if tc.disabled {
				configuredResolution = 0
			}

			m := s.openManager(nil, configuredResolution)

			// Act
			// Assert
			s.assertQuery(m, tc.query)
		})
	}
}

func (s *ManagerSelectionSuite) assertBlockState(m *Manager, mint int64, want []ulid.ULID) {
	s.T().Helper()
	s.Require().Equal(mint, m.MinTimeBlocks())
	var got []ulid.ULID
	previousMint := int64(math.MinInt64)

	for _, b := range m.Blocks() {
		s.Require().GreaterOrEqual(b.MinTime(), previousMint)
		previousMint = b.MinTime()
		got = append(got, b.Meta().ULID)
	}

	s.Require().Equal(want, got)
}

func (s *ManagerSelectionSuite) assertQuery(m *Manager, query selectionQuery) {
	s.T().Helper()
	matcher := labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, selectionMetric)
	q, err := m.Querier(query.mint, query.maxt)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(q.Close()) }()

	rq, advertisesResolution := q.(*upsampler.ResolutionQuerier)

	if query.resolution == 0 {
		s.Require().False(advertisesResolution, "raw and empty selections must not advertise downsampling")
	} else {
		s.Require().True(advertisesResolution)
		s.Require().Equal(query.resolution, rq.Resolution())
	}

	s.assertSamples(q.Select(s.T().Context(), true, nil, matcher), query.samples)
	cq, err := m.ChunkQuerier(query.mint, query.maxt)
	s.Require().NoError(err)
	defer func() { s.Require().NoError(cq.Close()) }()

	// Chunk queries preserve encoded chunks, which can contain samples outside the requested interval.
	var chunkSamples []selectionSample

	for _, sample := range query.samples {
		if sample.t >= query.mint && sample.t <= query.maxt {
			chunkSamples = append(chunkSamples, sample)
		}
	}

	set := storage.NewSeriesSetFromChunkSeriesSet(cq.Select(s.T().Context(), true, nil, matcher))
	var got []selectionSample

	for set.Next() {
		series := set.At()
		s.Require().Equal(labels.FromStrings(labels.MetricName, selectionMetric), series.Labels())
		it := series.Iterator(nil)

		for typ := it.Next(); typ != chunkenc.ValNone; typ = it.Next() {
			s.Require().Equal(chunkenc.ValFloat, typ)
			ts, value := it.At()

			if ts >= query.mint && ts <= query.maxt {
				got = append(got, selectionSample{t: ts, v: value})
			}
		}

		s.Require().NoError(it.Err())
	}

	s.Require().NoError(set.Err())
	s.Require().Equal(chunkSamples, got)
}

func (s *ManagerSelectionSuite) assertSamples(set storage.SeriesSet, want []selectionSample) {
	s.T().Helper()
	var got []selectionSample

	for set.Next() {
		series := set.At()
		s.Require().Equal(labels.FromStrings(labels.MetricName, selectionMetric), series.Labels())
		it := series.Iterator(nil)

		for typ := it.Next(); typ != chunkenc.ValNone; typ = it.Next() {
			s.Require().Equal(chunkenc.ValFloat, typ)
			ts, value := it.At()
			got = append(got, selectionSample{t: ts, v: value})
		}

		s.Require().NoError(it.Err())
	}

	s.Require().NoError(set.Err())
	s.Require().Equal(want, got)
}

func (s *ManagerSelectionSuite) createBlock(fixture selectionBlock) ulid.ULID {
	s.T().Helper()
	samples := make([]chunks.Sample, 0, len(fixture.samples))

	for _, sample := range fixture.samples {
		samples = append(samples, testutils.SampleTest{TS: sample.t, V: sample.v})
	}

	series := storage.NewListSeries(labels.FromStrings(labels.MetricName, selectionMetric), samples)
	dir, err := tsdb.CreateBlock([]storage.Series{series}, s.dir, 0, s.logger)
	s.Require().NoError(err)

	if fixture.resolution > 0 {
		meta, _, readErr := block.ReadFromDir(dir)
		s.Require().NoError(readErr)
		meta.Thanos.Downsample.Resolution = fixture.resolution
		_, err = block.WriteThanosMetaFile(s.logger, dir, meta)
		s.Require().NoError(err)
	}

	id, err := ulid.Parse(filepath.Base(dir))
	s.Require().NoError(err)

	return id
}

func (s *ManagerSelectionSuite) openManager(retention BlocksToDeleteFunc, resolution int64) *Manager {
	s.T().Helper()
	m, err := NewManager(s.dir, &Options{DownsamplingMS: resolution}, noopCompactor{}, retention,
		s.chunkPool, nil, s.logger, nil)
	s.Require().NoError(err)
	s.T().Cleanup(m.Close)

	return m
}
