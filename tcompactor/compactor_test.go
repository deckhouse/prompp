package tcompactor_test

import (
	"context"
	"testing"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"
	"github.com/thanos-io/objstore"
	"github.com/thanos-io/objstore/client"
	objstoretracing "github.com/thanos-io/objstore/tracing/opentracing"
	"github.com/thanos-io/thanos/pkg/block/metadata"
	"github.com/thanos-io/thanos/pkg/compact"
	"github.com/thanos-io/thanos/pkg/extprom"

	"github.com/prometheus/prometheus/tsdb"
)

const component = "prompp_thanos"

func TestThanosCompactor(t *testing.T) {
	confContentYaml := []byte(`
type: FILESYSTEM
config:
  directory: "./data"
prefix: test
`)
	l := log.NewNopLogger()
	bkt, err := client.NewBucket(l, confContentYaml, component, nil)
	require.NoError(t, err)

	insBkt := objstoretracing.WrapWithTraces(objstore.WrapWithMetrics(
		bkt,
		extprom.WrapRegistererWithPrefix(component+"_", prometheus.DefaultRegisterer),
		bkt.Name(),
	))

	blockMetaFetchConcurrency := 10
	noCompactMarkerFilter := compact.NewGatherNoCompactionMarkFilter(l, insBkt, blockMetaFetchConcurrency)

	ranges := []int64{20, 60, 180, 540, 1620}
	tsdbPlanner := compact.NewPlanner(l, ranges, noCompactMarkerFilter)

	t.Log(noCompactMarkerFilter.NoCompactMarkedBlocks())

	metasByMinTime := []*metadata.Meta{
		{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(1, nil), MinTime: 0, MaxTime: 20}},
		{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(2, nil), MinTime: 20, MaxTime: 40}},
		{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(3, nil), MinTime: 40, MaxTime: 60}},
		{BlockMeta: tsdb.BlockMeta{Version: 1, ULID: ulid.MustNew(4, nil), MinTime: 60, MaxTime: 80}},
	}
	m, err := tsdbPlanner.Plan(context.Background(), metasByMinTime, nil, nil)
	require.NoError(t, err)
	t.Log(m)
}
