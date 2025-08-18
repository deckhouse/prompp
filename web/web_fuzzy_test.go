// Copyright 2015 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/config"
	"github.com/prometheus/prometheus/pp-pkg/receiver"
	"github.com/prometheus/prometheus/pp-pkg/scrape"
	pp_pkg_storage "github.com/prometheus/prometheus/pp-pkg/storage"
	relabeler_config "github.com/prometheus/prometheus/pp/go/relabeler/config"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/catalog"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/rules"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/web"
	apiV1 "github.com/prometheus/prometheus/web/api/v1"
	"github.com/prometheus/prometheus/web/mock"

	"github.com/go-kit/log"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
		// func(t TestingT, query string, body []byte) {
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
	// }(f, "lkasjdf", []byte{12, 25, 234})
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
	receiver := makeReceiver(ctx, t, logger, dbDir, headCatalog)
	adminStats := makeTSDBAdminStats(t)
	listener, err := net.Listen("tcp", ":0")
	require.NoError(t, err, "Listen on random port")

	fanoutStorage := storage.NewFanout(logger, pp_pkg_storage.NewQueryableStorage(receiver))

	opts := makeOptions(t, adminStats, fanoutStorage, dbDir, listener.Addr().String())
	webHandler := web.New(logger, opts, receiver)
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

func makeReceiver(ctx context.Context, t TestingT, logger log.Logger, dbDir string, headCatalog *catalog.Catalog) *receiver.Receiver {
	t.Helper()

	transparent := &relabeler_config.InputRelabelerConfig{
		Name: "transparent_relabeler",
	}
	receiver, err := receiver.NewReceiver(
		ctx,
		log.With(logger, "component", "receiver"),
		nil,
		&config.RemoteWriteReceiverConfig{
			NumberOfShards: 2,
			Configs:        []*relabeler_config.InputRelabelerConfig{transparent},
		},
		dbDir,
		nil,
		dbDir,
		receiver.RotationInfo{
			BlockDuration: 2 * time.Hour,
			Seed:          0,
		},
		headCatalog,
		receiver.NewReloadBlocksTriggerNotifier(),
		&mock.ReadyNotifierMock{NotifyReadyFunc: func() {}},
		5*time.Second,
		24*time.Hour,
		4*time.Hour,
		90*time.Second,
		100e3,
	)
	require.NoError(t, err, "create a receiver")
	return receiver
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
