package web_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/web"
	apiV1 "github.com/prometheus/prometheus/web/api/v1"
	"github.com/prometheus/prometheus/web/mock"

	"github.com/prometheus/prometheus/pp-pkg/rules" // PP_CHANGES.md: rebuild on cpp
	"github.com/prometheus/prometheus/pp-pkg/scrape"
	pp_pkg_storage "github.com/prometheus/prometheus/pp-pkg/storage"

	pp_storage "github.com/prometheus/prometheus/pp/go/storage"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
)

func FuzzWeb(f *testing.F) {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	f.Log("Run service")
	listener := startService(ctx, f)

	baseURL := "http://" + listener.Addr().String()

	f.Log("Wait service to start")
	assert.EventuallyWithT(f, func(t *assert.CollectT) {
		resp, err := http.Get(baseURL + "/-/ready")
		if assert.NoError(t, err) {
			assert.Equal(t, http.StatusOK, resp.StatusCode)
		}
	}, 5*time.Second, 100*time.Millisecond, "Server hasn't start")

	f.Fuzz(func(t *testing.T, query string, body []byte) {
		t.Log("fuzzing")
		type Endpoint struct {
			path    string
			methods []string
		}
		for _, e := range []Endpoint{
			{"/api/v1/query", []string{http.MethodGet, http.MethodPost}},
			{"/api/v1/query_range", []string{http.MethodGet, http.MethodPost}},
			{"/api/v1/query_exemplars", []string{http.MethodGet, http.MethodPost}},
			{"/api/v1/format_query", []string{http.MethodGet, http.MethodPost}},
			{"/api/v1/labels", []string{http.MethodGet, http.MethodPost}},
			{"/api/v1/targets", []string{http.MethodGet}},
			{"/api/v1/targets/metadata", []string{http.MethodGet}},
			{"/api/v1/write", []string{http.MethodPost}},
		} {
			for _, method := range e.methods {
				var payload io.Reader
				if slices.Contains([]string{http.MethodPatch, http.MethodPost, http.MethodPut}, method) {
					payload = bytes.NewReader(body)
				}
				req, err := http.NewRequest(method, baseURL+e.path, payload)
				require.NoError(t, err, "Make request")
				res, err := http.DefaultClient.Do(req)
				require.NoError(t, err, "Execute request")
				t.Logf("done request (%s: %s) with %d", method, e.path, res.StatusCode)
				_ = res.Body.Close()
			}
		}
	})
}

type TestingT interface {
	TempDir() string
	Helper()
	Log(args ...any)
	Errorf(format string, args ...interface{})
	FailNow()
}

func startService(ctx context.Context, t TestingT) net.Listener {
	t.Helper()

	dbDir := t.TempDir()
	logger := log.NewLogfmtLogger(os.Stderr)
	clock := clockwork.NewRealClock()
	headCatalog := makeCatalog(t, clock, dbDir)
	hManager := makeManager(t, clock, dbDir, headCatalog)
	adapter := pp_pkg_storage.NewAdapter(
		clock,
		hManager.Proxy(),
		hManager.Builder(),
		hManager.MergeOutOfOrderChunks,
		prometheus.DefaultRegisterer,
	)

	adminStats := makeTSDBAdminStats(t)
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "Listen on random port")

	fanoutStorage := storage.NewFanout(logger, adapter)

	opts := makeOptions(t, adminStats, fanoutStorage, dbDir, listener.Addr().String())
	webHandler := web.New(logger, opts, adapter)
	webHandler.SetReady(true)

	go func() {
		err := webHandler.Run(ctx, []net.Listener{listener}, "")
		if err != nil {
			panic(fmt.Sprintf("Can't start web handler:%s", err))
		}
	}()

	return listener
}

func makeCatalog(t TestingT, clock clockwork.Clock, dbDir string) *catalog.Catalog {
	t.Helper()

	fileLog, err := catalog.NewFileLogV2(filepath.Join(dbDir, "head.log"))
	require.NoError(t, err, "create catalog file log")

	headCatalog, err := catalog.New(clock, fileLog, catalog.DefaultIDGenerator{}, int(4*1<<20), nil)
	require.NoError(t, err, "init catalog")

	return headCatalog
}

func makeManager(
	t TestingT,
	clock clockwork.Clock,
	dbDir string,
	headCatalog *catalog.Catalog,
) *pp_storage.Manager {
	t.Helper()

	hManager, err := pp_storage.NewManager(
		&pp_storage.Options{
			Seed:                0,
			BlockDuration:       2 * time.Hour,
			CommitInterval:      5 * time.Second,
			MaxRetentionPeriod:  24 * time.Hour,
			HeadRetentionPeriod: 4 * time.Hour,
			KeeperCapacity:      2,
			DataDir:             dbDir,
			MaxSegmentSize:      100e3,
			NumberOfShards:      2,
		},
		clock,
		headCatalog,
		pp_storage.NewMultiTriggerNotifier(pp_storage.NewTriggerNotifier()),
		pp_storage.NewTriggerNotifier(),
		&mock.ReadyNotifierMock{NotifyReadyFunc: func() {}},
		prometheus.DefaultRegisterer,
	)
	require.NoError(t, err, "create a head manager")

	return hManager
}

func makeTSDBAdminStats(t TestingT) apiV1.TSDBAdminStats {
	t.Helper()

	headStats := tsdb.NewHeadStats()
	return &mock.TSDBAdminStatsMock{
		CleanTombstonesFunc: func() error { return nil },
		DeleteFunc:          func(context.Context, int64, int64, ...*labels.Matcher) error { return nil },
		SnapshotFunc:        func(string, bool) error { return nil },
		StatsFunc: func(statsByLabelName string, limit int) (*tsdb.Stats, error) {
			return &tsdb.Stats{}, nil
		},
		WALReplayStatusFunc: func() (tsdb.WALReplayStatus, error) {
			return headStats.WALReplayStatus.GetWALReplayStatus(), nil
		},
	}
}

func makeOptions(t TestingT, adminStats apiV1.TSDBAdminStats, st storage.Storage, dbDir, addr string) *web.Options {
	t.Helper()

	return &web.Options{
		ListenAddresses: []string{addr},
		ReadTimeout:     30 * time.Second,
		MaxConnections:  512,
		Context:         nil,
		Storage:         st,
		LocalStorage:    adminStats,
		TSDBDir:         dbDir,
		QueryEngine: promql.NewEngine(promql.EngineOpts{
			LookbackDelta: 90 * time.Second,
		}),
		ScrapeManager:             &scrape.Manager{},
		RuleManager:               &rules.Manager{},
		Notifier:                  nil,
		RoutePrefix:               "/",
		EnableAdminAPI:            true,
		EnableRemoteWriteReceiver: true,
		ExternalURL: &url.URL{
			Scheme: "http",
			Host:   addr,
			Path:   "/",
		},
		Version:  &web.PrometheusVersion{},
		Gatherer: prometheus.DefaultGatherer,
	}
}

// RandomUnprivilegedPort returns valid unprivileged random port number which can be used for testing.
func RandomUnprivilegedPort(f *testing.F) int {
	f.Helper()

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		f.Fatalf("Listening on random port: %v", err)
	}

	if err := listener.Close(); err != nil {
		f.Fatalf("Closing listener: %v", err)
	}

	return listener.Addr().(*net.TCPAddr).Port
}
