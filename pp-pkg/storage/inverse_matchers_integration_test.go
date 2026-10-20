package storage

import (
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/block"
	"github.com/prometheus/prometheus/pp-pkg/blocks/testutils"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	ppstorage "github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
	"github.com/prometheus/prometheus/pp/go/storage/storagetest"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
	"github.com/prometheus/prometheus/tsdb/chunks"
)

// inverseMixedSuite checks selection and metadata against a block and the real Prom++ head merge.
// Newline values live in the block; the C++ head supplies overlapping and head-only series.
type inverseMixedSuite struct {
	suite.Suite
	blockQuerier storage.Querier
	mixedQuerier storage.Querier
	blockSeries  []storagetest.TimeSeries
	mixedSeries  []storagetest.TimeSeries
}

func TestInverseMixedStorage(t *testing.T) {
	suite.Run(t, new(inverseMixedSuite))
}

func (s *inverseMixedSuite) SetupSuite() {
	dir := s.T().TempDir()
	log, err := catalog.NewFileLogV2(filepath.Join(dir, "catalog.log"))
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.Require().NoError(log.Close()) })
	cat, err := catalog.New(clockwork.NewFakeClock(), log, catalog.DefaultIDGenerator{}, 1024, nil)
	s.Require().NoError(err)
	head, err := ppstorage.NewBuilder(cat, dir, 1024, prometheus.NewRegistry(), 0).Build(0, 2)
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.Require().NoError(head.Close()) })
	var blockEntries []storage.Series
	var headEntries []storagetest.TimeSeries

	for i, value := range []string{"", "a", "b", "\n", "a\nb", "\n\n", "a\n", "a.b", "c"} {
		builder := labels.NewBuilder(labels.FromStrings("__name__", "inverse_metric", "id", strconv.Itoa(i)))
		builder.Set("target", value)

		if strings.Contains(value, "\n") {
			builder.Set("newline", "yes")
		}

		if value == "c" {
			builder.Set("head_only", "yes")
		}

		ls := builder.Labels()
		all := storagetest.TimeSeries{Labels: ls}

		if value != "c" {
			all.Samples = append(all.Samples, cppbridge.Sample{Timestamp: 100, Value: float64(i)})
			s.blockSeries = append(s.blockSeries, all)
			blockEntries = append(blockEntries, storage.NewListSeries(ls, []chunks.Sample{
				testutils.SampleTest{TS: 100, V: float64(i)},
			}))
		}

		if !strings.Contains(value, "\n") {
			point := cppbridge.Sample{Timestamp: 200, Value: float64(i + 10)}
			headEntries = append(headEntries, storagetest.TimeSeries{Labels: ls, Samples: []cppbridge.Sample{point}})
			all.Samples = append(slices.Clone(all.Samples), point)
		}

		s.mixedSeries = append(s.mixedSeries, all)
	}

	storagetest.MustAppendTimeSeries(s.T().Context(), s.Require().NoError, head, headEntries)
	b, err := block.OpenBlock(nil, testutils.CreateBlock(s.T(), s.T().TempDir(), blockEntries), nil)
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.Require().NoError(b.Close()) })
	s.blockQuerier, err = tsdb.NewBlockQuerier(b, 0, 300)
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.Require().NoError(s.blockQuerier.Close()) })
	blockQ, err := tsdb.NewBlockQuerier(b, 0, 300)
	s.Require().NoError(err)
	headQ := querier.NewQuerier(head, querier.NewNoOpShardedDeduplicator, 0, 300, 30, 0, nil)
	s.mixedQuerier = querier.NewMultiQuerier([]storage.Querier{blockQ, headQ}, nil)
	s.T().Cleanup(func() { s.Require().NoError(s.mixedQuerier.Close()) })
}

func (s *inverseMixedSuite) TestBlockMetadata() {
	s.checkMetadata(s.blockQuerier, s.blockSeries)
}

func (s *inverseMixedSuite) TestMixedMetadata() {
	s.checkMetadata(s.mixedQuerier, s.mixedSeries)
}

func (s *inverseMixedSuite) TestMixedSelectSamples() {
	for _, typ := range []labels.MatchType{labels.MatchEqual, labels.MatchNotEqual, labels.MatchRegexp, labels.MatchNotRegexp} {
		for _, value := range []string{"", ".+", ".*", "a.*", "a|b", "(?s:.+)", "no-match"} {
			s.Run(fmt.Sprintf("%v_%q", typ, value), func() {
				ms := s.matchers(typ, value)
				expected := make(map[string][]cppbridge.Sample)

				for _, series := range s.mixedSeries {
					if s.matches(series.Labels, ms) {
						expected[series.Labels.Get("id")] = series.Samples
					}
				}

				set := s.mixedQuerier.Select(s.T().Context(), true, &storage.SelectHints{
					Start: 0, End: 300, Step: 30,
				}, ms...)
				actual := make(map[string][]cppbridge.Sample)

				for set.Next() {
					series := set.At()
					id := series.Labels().Get("id")
					it := series.Iterator(nil)
					s.NotContains(actual, id, "overlapping head/block series must merge")

					for it.Next() != chunkenc.ValNone {
						ts, sampleValue := it.At()
						actual[id] = append(actual[id], cppbridge.Sample{Timestamp: ts, Value: sampleValue})
					}

					s.Require().NoError(it.Err())
				}

				s.Require().NoError(set.Err())
				s.Equal(expected, actual)
			})
		}
	}
}

func (s *inverseMixedSuite) checkMetadata(q storage.Querier, allSeries []storagetest.TimeSeries) {
	for _, typ := range []labels.MatchType{labels.MatchEqual, labels.MatchNotEqual, labels.MatchRegexp, labels.MatchNotRegexp} {
		for _, value := range []string{"", ".+", ".*", "a.*", "a|b", "(?s:.+)", "no-match"} {
			s.Run(fmt.Sprintf("%v_%q", typ, value), func() {
				ms := s.matchers(typ, value)
				expectedIDs := []string{}
				expectedNames := make(map[string]struct{})

				for _, series := range allSeries {
					if !s.matches(series.Labels, ms) {
						continue
					}

					expectedIDs = append(expectedIDs, series.Labels.Get("id"))
					series.Labels.Range(func(label labels.Label) { expectedNames[label.Name] = struct{}{} })
				}

				values, _, err := q.LabelValues(s.T().Context(), "id", &storage.LabelHints{}, ms...)
				s.Require().NoError(err)
				s.ElementsMatch(expectedIDs, values)
				s.True(slices.IsSorted(values))
				names, _, err := q.LabelNames(s.T().Context(), &storage.LabelHints{}, ms...)
				s.Require().NoError(err)
				s.ElementsMatch(slices.Sorted(maps.Keys(expectedNames)), names)
				s.True(slices.IsSorted(names))
			})
		}
	}
}

func (s *inverseMixedSuite) matchers(typ labels.MatchType, value string) []*labels.Matcher {
	return []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, "__name__", "inverse_metric"),
		labels.MustNewMatcher(typ, "target", value),
	}
}

func (s *inverseMixedSuite) matches(ls labels.Labels, matchers []*labels.Matcher) bool {
	for _, matcher := range matchers {
		if !matcher.Matches(ls.Get(matcher.Name)) {
			return false
		}
	}

	return true
}
