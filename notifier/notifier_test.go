// Copyright 2013 The Prometheus Authors
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

package notifier

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery"
	_ "github.com/prometheus/prometheus/discovery/file"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
)

func TestPostPath(t *testing.T) {
	cases := []struct {
		in, out string
	}{
		{
			in:  "",
			out: "/api/v1/alerts",
		},
		{
			in:  "/",
			out: "/api/v1/alerts",
		},
		{
			in:  "/prefix",
			out: "/prefix/api/v1/alerts",
		},
		{
			in:  "/prefix//",
			out: "/prefix/api/v1/alerts",
		},
		{
			in:  "prefix//",
			out: "/prefix/api/v1/alerts",
		},
	}
	for _, c := range cases {
		require.Equal(t, c.out, postPath(c.in, config.AlertmanagerAPIVersionV1))
	}
}

func alertsEqual(a, b []*Alert) error {
	if len(a) != len(b) {
		return fmt.Errorf("length mismatch: %v != %v", a, b)
	}
	for i, alert := range a {
		if !labels.Equal(alert.Labels, b[i].Labels) {
			return fmt.Errorf("label mismatch at index %d: %s != %s", i, alert.Labels, b[i].Labels)
		}
	}
	return nil
}

func newTestHTTPServerBuilder(expected *[]*Alert, errc chan<- error, u, p string, status *atomic.Int32) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		defer func() {
			if err == nil {
				return
			}
			select {
			case errc <- err:
			default:
			}
		}()
		user, pass, _ := r.BasicAuth()
		if user != u || pass != p {
			err = fmt.Errorf("unexpected user/password: %s/%s != %s/%s", user, pass, u, p)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		b, err := io.ReadAll(r.Body)
		if err != nil {
			err = fmt.Errorf("error reading body: %w", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		var alerts []*Alert
		err = json.Unmarshal(b, &alerts)
		if err == nil {
			err = alertsEqual(*expected, alerts)
		}
		w.WriteHeader(int(status.Load()))
	}))
}

func TestHandlerSendAll(t *testing.T) {
	var (
		errc                      = make(chan error, 1)
		expected                  = make([]*Alert, 0, maxBatchSize)
		status1, status2, status3 atomic.Int32
	)
	status1.Store(int32(http.StatusOK))
	status2.Store(int32(http.StatusOK))
	status3.Store(int32(http.StatusOK))

	server1 := newTestHTTPServerBuilder(&expected, errc, "prometheus", "testing_password", &status1)
	server2 := newTestHTTPServerBuilder(&expected, errc, "", "", &status2)
	server3 := newTestHTTPServerBuilder(&expected, errc, "", "", &status3)
	defer server1.Close()
	defer server2.Close()
	defer server3.Close()

	h := NewManager(&Options{}, nil)

	authClient, _ := config_util.NewClientFromConfig(
		config_util.HTTPClientConfig{
			BasicAuth: &config_util.BasicAuth{
				Username: "prometheus",
				Password: "testing_password",
			},
		}, "auth_alertmanager")

	am1Cfg := config.DefaultAlertmanagerConfig
	am1Cfg.Timeout = model.Duration(time.Second)

	am2Cfg := config.DefaultAlertmanagerConfig
	am2Cfg.Timeout = model.Duration(time.Second)

	am3Cfg := config.DefaultAlertmanagerConfig
	am3Cfg.Timeout = model.Duration(time.Second)

	opts := &Options{Do: do, QueueCapacity: 10_000}
	logger := log.NewNopLogger()

	h.alertmanagers = map[string]*alertmanagerSet{
		"1": {
			ams: []alertmanager{
				alertmanagerMock{
					urlf: func() string { return server1.URL },
				},
			},
			cfg:    &am1Cfg,
			client: authClient,
			sendLoops: map[string]*sendLoop{
				server1.URL: newSendLoop(server1.URL, authClient, &am1Cfg, opts, logger, h.metrics),
			},
			opts:    opts,
			loops:   &h.loops,
			metrics: h.metrics,
			logger:  logger,
		},
		"2": {
			ams: []alertmanager{
				alertmanagerMock{
					urlf: func() string { return server2.URL },
				},
				alertmanagerMock{
					urlf: func() string { return server3.URL },
				},
			},
			cfg: &am2Cfg,
			sendLoops: map[string]*sendLoop{
				server2.URL: newSendLoop(server2.URL, nil, &am2Cfg, opts, logger, h.metrics),
				server3.URL: newSendLoop(server3.URL, nil, &am3Cfg, opts, logger, h.metrics),
			},
			opts:    opts,
			loops:   &h.loops,
			metrics: h.metrics,
			logger:  logger,
		},
		// Empty list of Alertmanager endpoints.
		"3": {
			ams:       []alertmanager{},
			cfg:       &am3Cfg,
			sendLoops: make(map[string]*sendLoop),
			opts:      opts,
			loops:     &h.loops,
			metrics:   h.metrics,
			logger:    logger,
		},
	}

	var alerts []*Alert
	for i := range make([]struct{}, maxBatchSize) {
		alerts = append(alerts, &Alert{
			Labels: labels.FromStrings("alertname", strconv.Itoa(i)),
		})
		expected = append(expected, &Alert{
			Labels: labels.FromStrings("alertname", strconv.Itoa(i)),
		})
	}

	checkNoErr := func() {
		t.Helper()
		select {
		case err := <-errc:
			require.NoError(t, err)
		default:
		}
	}

	for _, ams := range h.alertmanagers {
		ams.startSendLoops(ams.ams)
	}
	defer func() {
		for _, ams := range h.alertmanagers {
			ams.cleanSendLoops(ams.ams...)
		}
	}()

	sent := func(server *httptest.Server) float64 {
		return prom_testutil.ToFloat64(h.metrics.sent.WithLabelValues(server.URL))
	}
	failed := func(server *httptest.Server) float64 {
		return prom_testutil.ToFloat64(h.metrics.errors.WithLabelValues(server.URL))
	}

	// Each Alertmanager has its own send loop, so every batch has to be
	// accounted for before a server changes its answer: an earlier batch that
	// is still queued would otherwise fail along with the current one.
	// sent_total counts the attempts, which makes it the marker of a batch that
	// a send loop is done with.
	batchesDone := func(expected float64) func() bool {
		return func() bool {
			return sent(server1) == expected*maxBatchSize &&
				sent(server2) == expected*maxBatchSize &&
				sent(server3) == expected*maxBatchSize
		}
	}

	// All Alertmanagers are up.
	h.Send(alerts...)
	require.Eventually(t, batchesDone(1), 10*time.Second, 10*time.Millisecond)
	require.Zero(t, failed(server1))
	checkNoErr()

	// The only Alertmanager of set 1 is down.
	status1.Store(int32(http.StatusNotFound))
	h.Send(alerts...)
	require.Eventually(t, batchesDone(2), 10*time.Second, 10*time.Millisecond)
	require.Equal(t, float64(maxBatchSize), failed(server1))
	checkNoErr()

	status1.Store(int32(http.StatusOK))

	// One of the two Alertmanagers of set 2 is down, the other one still gets the alerts.
	status2.Store(int32(http.StatusInternalServerError))
	h.Send(alerts...)
	require.Eventually(t, batchesDone(3), 10*time.Second, 10*time.Millisecond)
	require.Equal(t, float64(maxBatchSize), failed(server2))
	require.Zero(t, failed(server3))
	checkNoErr()

	// Both Alertmanagers of set 2 are down.
	status3.Store(int32(http.StatusInternalServerError))
	h.Send(alerts...)
	require.Eventually(t, batchesDone(4), 10*time.Second, 10*time.Millisecond)
	require.Equal(t, float64(2*maxBatchSize), failed(server2))
	require.Equal(t, float64(maxBatchSize), failed(server3))
	checkNoErr()
}

func TestHandlerSendAllRemapPerAm(t *testing.T) {
	var (
		errc      = make(chan error, 1)
		expected1 = make([]*Alert, 0, maxBatchSize)
		expected2 = make([]*Alert, 0, maxBatchSize)
		expected3 = make([]*Alert, 0)

		status1, status2, status3 atomic.Int32
	)
	status1.Store(int32(http.StatusOK))
	status2.Store(int32(http.StatusOK))
	status3.Store(int32(http.StatusOK))

	server1 := newTestHTTPServerBuilder(&expected1, errc, "", "", &status1)
	server2 := newTestHTTPServerBuilder(&expected2, errc, "", "", &status2)
	server3 := newTestHTTPServerBuilder(&expected3, errc, "", "", &status3)

	defer server1.Close()
	defer server2.Close()
	defer server3.Close()

	h := NewManager(&Options{}, nil)

	am1Cfg := config.DefaultAlertmanagerConfig
	am1Cfg.Timeout = model.Duration(time.Second)

	am2Cfg := config.DefaultAlertmanagerConfig
	am2Cfg.Timeout = model.Duration(time.Second)
	am2Cfg.AlertRelabelConfigs = []*relabel.Config{
		{
			SourceLabels: model.LabelNames{"alertnamedrop"},
			Action:       "drop",
			Regex:        relabel.MustNewRegexp(".+"),
		},
	}

	am3Cfg := config.DefaultAlertmanagerConfig
	am3Cfg.Timeout = model.Duration(time.Second)
	am3Cfg.AlertRelabelConfigs = []*relabel.Config{
		{
			SourceLabels: model.LabelNames{"alertname"},
			Action:       "drop",
			Regex:        relabel.MustNewRegexp(".+"),
		},
	}

	opts := &Options{Do: do, QueueCapacity: 10_000}
	logger := log.NewNopLogger()

	h.alertmanagers = map[string]*alertmanagerSet{
		// Drop no alerts.
		"1": {
			ams: []alertmanager{
				alertmanagerMock{
					urlf: func() string { return server1.URL },
				},
			},
			cfg: &am1Cfg,
			sendLoops: map[string]*sendLoop{
				server1.URL: newSendLoop(server1.URL, nil, &am1Cfg, opts, logger, h.metrics),
			},
			opts:    opts,
			loops:   &h.loops,
			metrics: h.metrics,
			logger:  logger,
		},
		// Drop only alerts with the "alertnamedrop" label.
		"2": {
			ams: []alertmanager{
				alertmanagerMock{
					urlf: func() string { return server2.URL },
				},
			},
			cfg: &am2Cfg,
			sendLoops: map[string]*sendLoop{
				server2.URL: newSendLoop(server2.URL, nil, &am2Cfg, opts, logger, h.metrics),
			},
			opts:    opts,
			loops:   &h.loops,
			metrics: h.metrics,
			logger:  logger,
		},
		// Drop all alerts.
		"3": {
			ams: []alertmanager{
				alertmanagerMock{
					urlf: func() string { return server3.URL },
				},
			},
			cfg: &am3Cfg,
			sendLoops: map[string]*sendLoop{
				server3.URL: newSendLoop(server3.URL, nil, &am3Cfg, opts, logger, h.metrics),
			},
			opts:    opts,
			loops:   &h.loops,
			metrics: h.metrics,
			logger:  logger,
		},
		// Empty list of Alertmanager endpoints.
		"4": {
			ams:       []alertmanager{},
			cfg:       &config.DefaultAlertmanagerConfig,
			sendLoops: make(map[string]*sendLoop),
			opts:      opts,
			loops:     &h.loops,
			metrics:   h.metrics,
			logger:    logger,
		},
	}

	var alerts []*Alert
	for i := range make([]struct{}, maxBatchSize/2) {
		alerts = append(alerts,
			&Alert{
				Labels: labels.FromStrings("alertname", strconv.Itoa(i)),
			},
			&Alert{
				Labels: labels.FromStrings("alertname", "test", "alertnamedrop", strconv.Itoa(i)),
			},
		)

		expected1 = append(expected1,
			&Alert{
				Labels: labels.FromStrings("alertname", strconv.Itoa(i)),
			}, &Alert{
				Labels: labels.FromStrings("alertname", "test", "alertnamedrop", strconv.Itoa(i)),
			},
		)

		expected2 = append(expected2, &Alert{
			Labels: labels.FromStrings("alertname", strconv.Itoa(i)),
		})
	}

	checkNoErr := func() {
		t.Helper()
		select {
		case err := <-errc:
			require.NoError(t, err)
		default:
		}
	}

	for _, ams := range h.alertmanagers {
		ams.startSendLoops(ams.ams)
	}
	defer func() {
		for _, ams := range h.alertmanagers {
			ams.cleanSendLoops(ams.ams...)
		}
	}()

	h.Send(alerts...)
	require.Eventually(t, func() bool {
		return prom_testutil.ToFloat64(h.metrics.sent.WithLabelValues(server1.URL)) == maxBatchSize &&
			prom_testutil.ToFloat64(h.metrics.sent.WithLabelValues(server2.URL)) == maxBatchSize/2
	}, 10*time.Second, 10*time.Millisecond)
	checkNoErr()

	// Set 3 drops all the alerts, so its Alertmanager is never contacted.
	require.Zero(t, prom_testutil.ToFloat64(h.metrics.sent.WithLabelValues(server3.URL)))
}

func TestExternalLabels(t *testing.T) {
	h := NewManager(&Options{
		QueueCapacity:  3 * maxBatchSize,
		ExternalLabels: labels.FromStrings("a", "b"),
		RelabelConfigs: []*relabel.Config{
			{
				SourceLabels: model.LabelNames{"alertname"},
				TargetLabel:  "a",
				Action:       "replace",
				Regex:        relabel.MustNewRegexp("externalrelabelthis"),
				Replacement:  "c",
			},
		},
	}, nil)

	cfg := config.DefaultAlertmanagerConfig
	h.alertmanagers = map[string]*alertmanagerSet{
		"test": {
			cfg: &cfg,
			sendLoops: map[string]*sendLoop{
				"test": newSendLoop("test", nil, &cfg, h.opts, log.NewNopLogger(), h.metrics),
			},
		},
	}

	// This alert should get the external label attached.
	h.Send(&Alert{
		Labels: labels.FromStrings("alertname", "test"),
	})

	// This alert should get the external label attached, but then set to "c"
	// through relabelling.
	h.Send(&Alert{
		Labels: labels.FromStrings("alertname", "externalrelabelthis"),
	})

	expected := []*Alert{
		{Labels: labels.FromStrings("alertname", "test", "a", "b")},
		{Labels: labels.FromStrings("alertname", "externalrelabelthis", "a", "c")},
	}

	require.NoError(t, alertsEqual(expected, h.alertmanagers["test"].sendLoops["test"].queue))
}

func TestHandlerRelabel(t *testing.T) {
	h := NewManager(&Options{
		QueueCapacity: 3 * maxBatchSize,
		RelabelConfigs: []*relabel.Config{
			{
				SourceLabels: model.LabelNames{"alertname"},
				Action:       "drop",
				Regex:        relabel.MustNewRegexp("drop"),
			},
			{
				SourceLabels: model.LabelNames{"alertname"},
				TargetLabel:  "alertname",
				Action:       "replace",
				Regex:        relabel.MustNewRegexp("rename"),
				Replacement:  "renamed",
			},
		},
	}, nil)

	cfg := config.DefaultAlertmanagerConfig
	h.alertmanagers = map[string]*alertmanagerSet{
		"test": {
			cfg: &cfg,
			sendLoops: map[string]*sendLoop{
				"test": newSendLoop("test", nil, &cfg, h.opts, log.NewNopLogger(), h.metrics),
			},
		},
	}

	// This alert should be dropped due to the configuration
	h.Send(&Alert{
		Labels: labels.FromStrings("alertname", "drop"),
	})

	// This alert should be replaced due to the configuration
	h.Send(&Alert{
		Labels: labels.FromStrings("alertname", "rename"),
	})

	expected := []*Alert{
		{Labels: labels.FromStrings("alertname", "renamed")},
	}

	require.NoError(t, alertsEqual(expected, h.alertmanagers["test"].sendLoops["test"].queue))
}

func TestHandlerQueuing(t *testing.T) {
	var (
		expectedc = make(chan []*Alert)
		called    = make(chan struct{})
		done      = make(chan struct{})
		errc      = make(chan error, 1)
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Notify the test function that we have received something.
		select {
		case called <- struct{}{}:
		case <-done:
			return
		}

		// Wait for the test function to unblock us.
		select {
		case expected := <-expectedc:
			var alerts []*Alert

			b, err := io.ReadAll(r.Body)
			if err != nil {
				panic(err)
			}

			err = json.Unmarshal(b, &alerts)
			if err == nil {
				err = alertsEqual(expected, alerts)
			}
			select {
			case errc <- err:
			default:
			}
		case <-done:
		}
	}))
	defer func() {
		close(done)
		server.Close()
	}()

	h := NewManager(
		&Options{
			QueueCapacity: 3 * maxBatchSize,
		},
		nil,
	)

	h.alertmanagers = make(map[string]*alertmanagerSet)

	am1Cfg := config.DefaultAlertmanagerConfig
	am1Cfg.Timeout = model.Duration(time.Second)

	h.alertmanagers["1"] = &alertmanagerSet{
		ams: []alertmanager{
			alertmanagerMock{
				urlf: func() string { return server.URL },
			},
		},
		cfg: &am1Cfg,
		sendLoops: map[string]*sendLoop{
			server.URL: newSendLoop(server.URL, nil, &am1Cfg, h.opts, log.NewNopLogger(), h.metrics),
		},
		opts:    h.opts,
		loops:   &h.loops,
		metrics: h.metrics,
		logger:  log.NewNopLogger(),
	}
	go h.Run(nil)
	defer h.Stop()

	for _, ams := range h.alertmanagers {
		ams.startSendLoops(ams.ams)
	}

	var alerts []*Alert
	for i := range make([]struct{}, 20*maxBatchSize) {
		alerts = append(alerts, &Alert{
			Labels: labels.FromStrings("alertname", strconv.Itoa(i)),
		})
	}

	assertAlerts := func(expected []*Alert) {
		t.Helper()
		for {
			select {
			case <-called:
				expectedc <- expected
			case err := <-errc:
				require.NoError(t, err)
				return
			case <-time.After(5 * time.Second):
				require.FailNow(t, "Alerts were not pushed.")
			}
		}
	}

	// If the batch is larger than the queue capacity, it should be truncated
	// from the front.
	h.Send(alerts[:4*maxBatchSize]...)
	for i := 1; i < 4; i++ {
		assertAlerts(alerts[i*maxBatchSize : (i+1)*maxBatchSize])
	}

	// Send one batch, wait for it to arrive and block the server so the queue fills up.
	h.Send(alerts[:maxBatchSize]...)
	<-called

	// Send several batches while the server is still blocked so the queue
	// fills up to its maximum capacity (3*maxBatchSize). Then check that the
	// queue is truncated in the front.
	h.Send(alerts[1*maxBatchSize : 2*maxBatchSize]...) // this batch should be dropped.
	h.Send(alerts[2*maxBatchSize : 3*maxBatchSize]...)
	h.Send(alerts[3*maxBatchSize : 4*maxBatchSize]...)

	// Send the batch that drops the first one.
	h.Send(alerts[4*maxBatchSize : 5*maxBatchSize]...)

	// Unblock the server.
	expectedc <- alerts[:maxBatchSize]
	select {
	case err := <-errc:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		require.FailNow(t, "Alerts were not pushed.")
	}

	// Verify that we receive the last 3 batches.
	for i := 2; i < 5; i++ {
		assertAlerts(alerts[i*maxBatchSize : (i+1)*maxBatchSize])
	}
}

type alertmanagerMock struct {
	urlf func() string
}

func (a alertmanagerMock) url() *url.URL {
	u, err := url.Parse(a.urlf())
	if err != nil {
		panic(err)
	}
	return u
}

func TestLabelSetNotReused(t *testing.T) {
	tg := makeInputTargetGroup()
	_, _, err := AlertmanagerFromGroup(tg, &config.AlertmanagerConfig{})

	require.NoError(t, err)

	// Target modified during alertmanager extraction
	require.Equal(t, tg, makeInputTargetGroup())
}

func TestReload(t *testing.T) {
	tests := []struct {
		in  *targetgroup.Group
		out string
	}{
		{
			in: &targetgroup.Group{
				Targets: []model.LabelSet{
					{
						"__address__": "alertmanager:9093",
					},
				},
			},
			out: "http://alertmanager:9093/api/v2/alerts",
		},
	}

	n := NewManager(&Options{}, nil)

	cfg := &config.Config{}
	s := `
alerting:
  alertmanagers:
  - static_configs:
`
	err := yaml.UnmarshalStrict([]byte(s), cfg)
	require.NoError(t, err, "Unable to load YAML config.")
	require.Len(t, cfg.AlertingConfig.AlertmanagerConfigs, 1)

	err = n.ApplyConfig(cfg)
	require.NoError(t, err, "Error applying the config.")

	tgs := make(map[string][]*targetgroup.Group)
	for _, tt := range tests {
		for k := range cfg.AlertingConfig.AlertmanagerConfigs.ToMap() {
			tgs[k] = []*targetgroup.Group{
				tt.in,
			}
			break
		}
		n.reload(tgs)
		res := n.Alertmanagers()[0].String()

		require.Equal(t, tt.out, res)
	}
}

func TestDroppedAlertmanagers(t *testing.T) {
	tests := []struct {
		in  *targetgroup.Group
		out string
	}{
		{
			in: &targetgroup.Group{
				Targets: []model.LabelSet{
					{
						"__address__": "alertmanager:9093",
					},
				},
			},
			out: "http://alertmanager:9093/api/v2/alerts",
		},
	}

	n := NewManager(&Options{}, nil)

	cfg := &config.Config{}
	s := `
alerting:
  alertmanagers:
  - static_configs:
    relabel_configs:
      - source_labels: ['__address__']
        regex: 'alertmanager:9093'
        action: drop
`
	err := yaml.UnmarshalStrict([]byte(s), cfg)
	require.NoError(t, err, "Unable to load YAML config.")
	require.Len(t, cfg.AlertingConfig.AlertmanagerConfigs, 1)

	err = n.ApplyConfig(cfg)
	require.NoError(t, err, "Error applying the config.")

	tgs := make(map[string][]*targetgroup.Group)
	for _, tt := range tests {
		for k := range cfg.AlertingConfig.AlertmanagerConfigs.ToMap() {
			tgs[k] = []*targetgroup.Group{
				tt.in,
			}
			break
		}

		n.reload(tgs)
		res := n.DroppedAlertmanagers()[0].String()

		require.Equal(t, res, tt.out)
	}
}

func makeInputTargetGroup() *targetgroup.Group {
	return &targetgroup.Group{
		Targets: []model.LabelSet{
			{
				model.AddressLabel:            model.LabelValue("1.1.1.1:9090"),
				model.LabelName("notcommon1"): model.LabelValue("label"),
			},
		},
		Labels: model.LabelSet{
			model.LabelName("common"): model.LabelValue("label"),
		},
		Source: "testsource",
	}
}

func TestLabelsToOpenAPILabelSet(t *testing.T) {
	require.Equal(t, models.LabelSet{"aaa": "111", "bbb": "222"}, labelsToOpenAPILabelSet(labels.FromStrings("aaa", "111", "bbb", "222")))
}

// TestHangingNotifier ensures that the notifier takes into account SD changes even when there are
// queued alerts. This test reproduces the issue described in https://github.com/prometheus/prometheus/issues/13676.
// and https://github.com/prometheus/prometheus/issues/8768.
func TestHangingNotifier(t *testing.T) {
	const (
		batches     = 100
		alertsCount = maxBatchSize * batches
	)

	var (
		sendTimeout = 100 * time.Millisecond
		sdUpdatert  = sendTimeout / 2

		done = make(chan struct{})
	)

	defer func() {
		close(done)
	}()

	// Set up a faulty Alertmanager.
	var faultyCalled atomic.Bool
	faultyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		faultyCalled.Store(true)
		select {
		case <-done:
		case <-time.After(time.Hour):
		}
	}))
	faultyURL, err := url.Parse(faultyServer.URL)
	require.NoError(t, err)
	faultyURL.Path = "/api/v2/alerts"

	// Set up a functional Alertmanager.
	var functionalCalled atomic.Bool
	functionalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		functionalCalled.Store(true)
	}))
	functionalURL, err := url.Parse(functionalServer.URL)
	require.NoError(t, err)
	functionalURL.Path = "/api/v2/alerts"

	// Initialize the discovery manager
	// This is relevant as the updates aren't sent continually in real life, but only each updatert.
	// The old implementation of TestHangingNotifier didn't take that into acount.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reg := prometheus.NewRegistry()
	sdMetrics, err := discovery.RegisterSDMetrics(reg, discovery.NewRefreshMetrics(reg))
	require.NoError(t, err)
	sdManager := discovery.NewManager(
		ctx,
		log.NewNopLogger(),
		reg,
		sdMetrics,
		discovery.Name("sd-manager"),
		discovery.Updatert(sdUpdatert),
	)
	go sdManager.Run()

	// Set up the notifier with both faulty and functional Alertmanagers.
	notifier := NewManager(
		&Options{
			QueueCapacity: alertsCount,
		},
		nil,
	)
	notifier.alertmanagers = make(map[string]*alertmanagerSet)
	amCfg := config.DefaultAlertmanagerConfig
	amCfg.Timeout = model.Duration(sendTimeout)
	notifier.alertmanagers["config-0"] = &alertmanagerSet{
		ams: []alertmanager{
			alertmanagerMock{
				urlf: func() string { return faultyURL.String() },
			},
			alertmanagerMock{
				urlf: func() string { return functionalURL.String() },
			},
		},
		cfg:     &amCfg,
		loops:   &notifier.loops,
		metrics: notifier.metrics,
		sendLoops: map[string]*sendLoop{
			faultyURL.String():     newSendLoop(faultyURL.String(), nil, &amCfg, notifier.opts, log.NewNopLogger(), notifier.metrics),
			functionalURL.String(): newSendLoop(functionalURL.String(), nil, &amCfg, notifier.opts, log.NewNopLogger(), notifier.metrics),
		},
		opts:   notifier.opts,
		logger: log.NewNopLogger(),
	}

	for _, ams := range notifier.alertmanagers {
		ams.startSendLoops(ams.ams)
	}

	go notifier.Run(sdManager.SyncCh())
	defer notifier.Stop()

	require.Len(t, notifier.Alertmanagers(), 2)

	// Enqueue the alerts.
	var alerts []*Alert
	for i := range make([]struct{}, alertsCount) {
		alerts = append(alerts, &Alert{
			Labels: labels.FromStrings("alertname", strconv.Itoa(i)),
		})
	}
	notifier.Send(alerts...)

	// Wait for the Alertmanagers to start receiving alerts.
	// 10*sdUpdatert is used as an arbitrary timeout here.
	timeout := time.After(10 * sdUpdatert)
loop1:
	for {
		select {
		case <-timeout:
			t.Fatalf("Timeout waiting for the alertmanagers to be reached for the first time.")
		default:
			if faultyCalled.Load() && functionalCalled.Load() {
				break loop1
			}
		}
	}

	// Request to remove the faulty Alertmanager.
	c := map[string]discovery.Configs{
		"config-0": {
			discovery.StaticConfig{
				&targetgroup.Group{
					Targets: []model.LabelSet{
						{
							model.AddressLabel: model.LabelValue(functionalURL.Host),
						},
					},
				},
			},
		},
	}
	require.NoError(t, sdManager.ApplyConfig(c))

	// The faulty Alertmanager never answers, so its queue keeps the alerts that the
	// notifier must not wait for before it applies the discovery changes.
	require.NotEmpty(t, notifier.alertmanagers["config-0"].sendLoops[faultyURL.String()].queue)

	// The notifier should not wait until the alerts queue is empty to apply the discovery changes
	// A faulty Alertmanager could cause each alert sending cycle to take up to AlertmanagerConfig.Timeout
	// The queue may never be emptied, as the arrival rate could be larger than the departure rate
	// It could even overflow and alerts could be dropped.
	timeout = time.After(batches * sendTimeout)
loop2:
	for {
		select {
		case <-timeout:
			t.Fatalf("Timeout, the faulty alertmanager not removed on time.")
		default:
			// The faulty alertmanager was dropped.
			if len(notifier.Alertmanagers()) == 1 {
				break loop2
			}
		}
	}

	// The send loop of the dropped Alertmanager is gone, the remaining one still runs.
	notifier.alertmanagers["config-0"].mtx.RLock()
	defer notifier.alertmanagers["config-0"].mtx.RUnlock()
	require.NotContains(t, notifier.alertmanagers["config-0"].sendLoops, faultyURL.String())
	require.Contains(t, notifier.alertmanagers["config-0"].sendLoops, functionalURL.String())
}

func TestStop_DrainingDisabled(t *testing.T) {
	releaseReceiver := make(chan struct{})
	receiverReceivedRequest := make(chan struct{}, 2)
	alertsReceived := atomic.NewInt64(0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Let the test know we've received a request.
		receiverReceivedRequest <- struct{}{}

		var alerts []*Alert

		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		err = json.Unmarshal(b, &alerts)
		require.NoError(t, err)

		alertsReceived.Add(int64(len(alerts)))

		// Wait for the test to release us.
		<-releaseReceiver

		w.WriteHeader(http.StatusOK)
	}))
	defer func() {
		server.Close()
	}()

	m := NewManager(
		&Options{
			QueueCapacity:   10,
			DrainOnShutdown: false,
		},
		nil,
	)

	m.alertmanagers = make(map[string]*alertmanagerSet)

	am1Cfg := config.DefaultAlertmanagerConfig
	am1Cfg.Timeout = model.Duration(time.Second)

	m.alertmanagers["1"] = &alertmanagerSet{
		ams: []alertmanager{
			alertmanagerMock{
				urlf: func() string { return server.URL },
			},
		},
		cfg: &am1Cfg,
		sendLoops: map[string]*sendLoop{
			server.URL: newSendLoop(server.URL, nil, &am1Cfg, m.opts, log.NewNopLogger(), m.metrics),
		},
		opts:    m.opts,
		loops:   &m.loops,
		metrics: m.metrics,
		logger:  log.NewNopLogger(),
	}

	for _, ams := range m.alertmanagers {
		ams.startSendLoops(ams.ams)
	}

	notificationManagerStopped := make(chan struct{})

	go func() {
		defer close(notificationManagerStopped)
		m.Run(nil)
	}()

	// Queue two alerts. The first should be immediately sent to the receiver, which should block until we release it later.
	m.Send(&Alert{Labels: labels.FromStrings(labels.AlertName, "alert-1")})

	select {
	case <-receiverReceivedRequest:
		// Nothing more to do.
	case <-time.After(10 * time.Second):
		require.FailNow(t, "gave up waiting for receiver to receive notification of first alert")
	}

	m.Send(&Alert{Labels: labels.FromStrings(labels.AlertName, "alert-2")})

	// Stop the notification manager, pause to allow the shutdown to be observed, and then allow the receiver to proceed.
	m.Stop()
	time.Sleep(time.Second)
	close(releaseReceiver)

	// Wait for the notification manager to stop and confirm only the first notification was sent.
	// The second notification should be dropped.
	select {
	case <-notificationManagerStopped:
		// Nothing more to do.
	case <-time.After(10 * time.Second):
		require.FailNow(t, "gave up waiting for notification manager to stop")
	}

	require.Equal(t, int64(1), alertsReceived.Load())
}

func TestStop_DrainingEnabled(t *testing.T) {
	releaseReceiver := make(chan struct{})
	receiverReceivedRequest := make(chan struct{}, 2)
	alertsReceived := atomic.NewInt64(0)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Let the test know we've received a request.
		receiverReceivedRequest <- struct{}{}

		var alerts []*Alert

		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)

		err = json.Unmarshal(b, &alerts)
		require.NoError(t, err)

		alertsReceived.Add(int64(len(alerts)))

		// Wait for the test to release us.
		<-releaseReceiver

		w.WriteHeader(http.StatusOK)
	}))
	defer func() {
		server.Close()
	}()

	m := NewManager(
		&Options{
			QueueCapacity:   10,
			DrainOnShutdown: true,
		},
		nil,
	)

	m.alertmanagers = make(map[string]*alertmanagerSet)

	am1Cfg := config.DefaultAlertmanagerConfig
	am1Cfg.Timeout = model.Duration(time.Second)

	m.alertmanagers["1"] = &alertmanagerSet{
		ams: []alertmanager{
			alertmanagerMock{
				urlf: func() string { return server.URL },
			},
		},
		cfg: &am1Cfg,
		sendLoops: map[string]*sendLoop{
			server.URL: newSendLoop(server.URL, nil, &am1Cfg, m.opts, log.NewNopLogger(), m.metrics),
		},
		opts:    m.opts,
		loops:   &m.loops,
		metrics: m.metrics,
		logger:  log.NewNopLogger(),
	}

	for _, ams := range m.alertmanagers {
		ams.startSendLoops(ams.ams)
	}

	notificationManagerStopped := make(chan struct{})

	go func() {
		defer close(notificationManagerStopped)
		m.Run(nil)
	}()

	// Queue two alerts. The first should be immediately sent to the receiver, which should block until we release it later.
	m.Send(&Alert{Labels: labels.FromStrings(labels.AlertName, "alert-1")})

	select {
	case <-receiverReceivedRequest:
		// Nothing more to do.
	case <-time.After(10 * time.Second):
		require.FailNow(t, "gave up waiting for receiver to receive notification of first alert")
	}

	m.Send(&Alert{Labels: labels.FromStrings(labels.AlertName, "alert-2")})

	// Stop the notification manager and allow the receiver to proceed.
	m.Stop()
	close(releaseReceiver)

	// Wait for the notification manager to stop and confirm both notifications were sent.
	select {
	case <-notificationManagerStopped:
		// Nothing more to do.
	case <-time.After(10 * time.Second):
		require.FailNow(t, "gave up waiting for notification manager to stop")
	}

	require.Equal(t, int64(2), alertsReceived.Load())
}

// TestQueuesDrainingOnApplyConfig ensures that when an alertmanagerSet disappears after an ApplyConfig(), its
// sendLoops queues are drained only when DrainOnShutdown is set.
func TestQueuesDrainingOnApplyConfig(t *testing.T) {
	for _, drainOnShutdown := range []bool{false, true} {
		t.Run(strconv.FormatBool(drainOnShutdown), func(t *testing.T) {
			requestReceived := make(chan struct{}, 2)
			releaseRequest := make(chan struct{})
			alertsReceived := atomic.NewInt64(0)

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var alerts []*Alert

				b, err := io.ReadAll(r.Body)
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(b, &alerts))

				alertsReceived.Add(int64(len(alerts)))
				requestReceived <- struct{}{}

				// Wait for the test to release us.
				<-releaseRequest

				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			amCfg := config.DefaultAlertmanagerConfig
			amCfg.Timeout = model.Duration(time.Second)

			opts := &Options{Do: do, QueueCapacity: 10, DrainOnShutdown: drainOnShutdown}
			logger := log.NewNopLogger()

			h := NewManager(opts, nil)
			h.alertmanagers = map[string]*alertmanagerSet{
				"1": {
					ams: []alertmanager{
						alertmanagerMock{
							urlf: func() string { return server.URL },
						},
					},
					cfg: &amCfg,
					sendLoops: map[string]*sendLoop{
						server.URL: newSendLoop(server.URL, nil, &amCfg, opts, logger, h.metrics),
					},
					opts:    opts,
					loops:   &h.loops,
					metrics: h.metrics,
					logger:  logger,
				},
			}
			for _, ams := range h.alertmanagers {
				ams.startSendLoops(ams.ams)
			}

			// The first alert hangs in the request to the alertmanager, the
			// second one stays in the queue.
			h.Send(&Alert{Labels: labels.FromStrings("alertname", "first")})
			select {
			case <-requestReceived:
			case <-time.After(10 * time.Second):
				require.FailNow(t, "gave up waiting for the first alert to be sent")
			}
			h.Send(&Alert{Labels: labels.FromStrings("alertname", "second")})

			// Remove the alertmanagerSet and let the pending request finish.
			require.NoError(t, h.ApplyConfig(&config.Config{}))
			close(releaseRequest)

			if drainOnShutdown {
				select {
				case <-requestReceived:
				case <-time.After(10 * time.Second):
					require.FailNow(t, "the queue wasn't drained")
				}
			} else {
				select {
				case <-requestReceived:
					require.FailNow(t, "the queue should not be drained")
				case <-time.After(500 * time.Millisecond):
				}
			}

			// The send loop stops once it is done with its queue.
			h.loops.Wait()

			if drainOnShutdown {
				require.Equal(t, int64(2), alertsReceived.Load())
			} else {
				require.Equal(t, int64(1), alertsReceived.Load())
			}
		})
	}
}

func TestApplyConfig(t *testing.T) {
	targetURL := "alertmanager:9093"
	targetGroup := &targetgroup.Group{
		Targets: []model.LabelSet{
			{
				"__address__": model.LabelValue(targetURL),
			},
		},
	}
	alertmanagerURL := fmt.Sprintf("http://%s/api/v2/alerts", targetURL)

	n := NewManager(&Options{}, nil)
	cfg := &config.Config{}
	s := `
alerting:
  alertmanagers:
  - file_sd_configs:
    - files:
      - foo.json
`
	// 1. Ensure known alertmanagers are not dropped during ApplyConfig.
	require.NoError(t, yaml.UnmarshalStrict([]byte(s), cfg))
	require.Len(t, cfg.AlertingConfig.AlertmanagerConfigs, 1)

	// First, apply the config and reload.
	require.NoError(t, n.ApplyConfig(cfg))
	tgs := map[string][]*targetgroup.Group{"config-0": {targetGroup}}
	n.reload(tgs)
	require.Len(t, n.Alertmanagers(), 1)
	require.Equal(t, alertmanagerURL, n.Alertmanagers()[0].String())

	// The send loop of the discovered alertmanager is running.
	require.Contains(t, n.alertmanagers["config-0"].sendLoops, alertmanagerURL)
	sendLoopBeforeReload := n.alertmanagers["config-0"].sendLoops[alertmanagerURL]

	// Reapply the config.
	require.NoError(t, n.ApplyConfig(cfg))
	// Ensure the known alertmanagers are not dropped.
	require.Len(t, n.Alertmanagers(), 1)
	require.Equal(t, alertmanagerURL, n.Alertmanagers()[0].String())
	// Ensure the send loop and its queue survived the reload.
	require.Same(t, sendLoopBeforeReload, n.alertmanagers["config-0"].sendLoops[alertmanagerURL])

	// 2. Ensure known alertmanagers are not dropped during ApplyConfig even when
	// the config order changes.
	s = `
alerting:
  alertmanagers:
  - static_configs:
  - file_sd_configs:
    - files:
      - foo.json
`
	require.NoError(t, yaml.UnmarshalStrict([]byte(s), cfg))
	require.Len(t, cfg.AlertingConfig.AlertmanagerConfigs, 2)

	require.NoError(t, n.ApplyConfig(cfg))
	require.Len(t, n.Alertmanagers(), 1)
	// Ensure no unnecessary alertmanagers are injected.
	require.Empty(t, n.alertmanagers["config-0"].ams)
	// Ensure the config order is taken into account.
	ams := n.alertmanagers["config-1"].ams
	require.Len(t, ams, 1)
	require.Equal(t, alertmanagerURL, ams[0].url().String())

	// 3. Ensure known alertmanagers are reused for new config with identical AlertmanagerConfig.
	s = `
alerting:
  alertmanagers:
  - file_sd_configs:
    - files:
      - foo.json
  - file_sd_configs:
    - files:
      - foo.json
`
	require.NoError(t, yaml.UnmarshalStrict([]byte(s), cfg))
	require.Len(t, cfg.AlertingConfig.AlertmanagerConfigs, 2)

	require.NoError(t, n.ApplyConfig(cfg))
	require.Len(t, n.Alertmanagers(), 2)
	for cfgIdx := 0; cfgIdx < 2; cfgIdx++ {
		ams := n.alertmanagers[fmt.Sprintf("config-%d", cfgIdx)].ams
		require.Len(t, ams, 1)
		require.Equal(t, alertmanagerURL, ams[0].url().String())
	}

	// 4. Ensure known alertmanagers are reused only for identical AlertmanagerConfig.
	s = `
alerting:
  alertmanagers:
  - file_sd_configs:
    - files:
      - foo.json
    path_prefix: /bar
  - file_sd_configs:
    - files:
      - foo.json
    relabel_configs:
    - source_labels: ['__address__']
      regex: 'doesntmatter:1234'
      action: drop
`
	require.NoError(t, yaml.UnmarshalStrict([]byte(s), cfg))
	require.Len(t, cfg.AlertingConfig.AlertmanagerConfigs, 2)

	require.NoError(t, n.ApplyConfig(cfg))
	require.Empty(t, n.Alertmanagers())
}

// TestAlertsRelabelingIsIsolated ensures that alerts relabeling in one
// alertmanagerSet doesn't affect the others.
// See https://github.com/prometheus/prometheus/pull/17063.
func TestAlertsRelabelingIsIsolated(t *testing.T) {
	var (
		errc      = make(chan error, 1)
		expected1 = make([]*Alert, 0)
		expected2 = make([]*Alert, 0)

		status1, status2 atomic.Int32
	)
	status1.Store(int32(http.StatusOK))
	status2.Store(int32(http.StatusOK))

	server1 := newTestHTTPServerBuilder(&expected1, errc, "", "", &status1)
	server2 := newTestHTTPServerBuilder(&expected2, errc, "", "", &status2)

	defer server1.Close()
	defer server2.Close()

	h := NewManager(&Options{}, nil)

	am1Cfg := config.DefaultAlertmanagerConfig
	am1Cfg.Timeout = model.Duration(time.Second)
	am1Cfg.AlertRelabelConfigs = []*relabel.Config{
		{
			SourceLabels: model.LabelNames{"alertname"},
			Regex:        relabel.MustNewRegexp("(.*)"),
			TargetLabel:  "parasite",
			Action:       relabel.Replace,
			Replacement:  "yes",
		},
	}

	am2Cfg := config.DefaultAlertmanagerConfig
	am2Cfg.Timeout = model.Duration(time.Second)

	opts := &Options{Do: do, QueueCapacity: 10}
	logger := log.NewNopLogger()

	h.alertmanagers = map[string]*alertmanagerSet{
		"am1": {
			ams: []alertmanager{
				alertmanagerMock{
					urlf: func() string { return server1.URL },
				},
			},
			cfg: &am1Cfg,
			sendLoops: map[string]*sendLoop{
				server1.URL: newSendLoop(server1.URL, nil, &am1Cfg, opts, logger, h.metrics),
			},
			opts:    opts,
			loops:   &h.loops,
			metrics: h.metrics,
			logger:  logger,
		},
		"am2": {
			ams: []alertmanager{
				alertmanagerMock{
					urlf: func() string { return server2.URL },
				},
			},
			cfg: &am2Cfg,
			sendLoops: map[string]*sendLoop{
				server2.URL: newSendLoop(server2.URL, nil, &am2Cfg, opts, logger, h.metrics),
			},
			opts:    opts,
			loops:   &h.loops,
			metrics: h.metrics,
			logger:  logger,
		},
	}

	for _, ams := range h.alertmanagers {
		ams.startSendLoops(ams.ams)
	}
	defer func() {
		for _, ams := range h.alertmanagers {
			ams.cleanSendLoops(ams.ams...)
		}
	}()

	// am1 adds the parasite label, am2 must not see it.
	expected1 = append(expected1, &Alert{
		Labels: labels.FromStrings("alertname", "test", "parasite", "yes"),
	})
	expected2 = append(expected2, &Alert{
		Labels: labels.FromStrings("alertname", "test"),
	})

	h.Send(&Alert{
		Labels: labels.FromStrings("alertname", "test"),
	})

	require.Eventually(t, func() bool {
		return prom_testutil.ToFloat64(h.metrics.sent.WithLabelValues(server1.URL)) == 1 &&
			prom_testutil.ToFloat64(h.metrics.sent.WithLabelValues(server2.URL)) == 1
	}, 10*time.Second, 10*time.Millisecond)

	select {
	case err := <-errc:
		require.NoError(t, err)
	default:
	}
}

// TestNotifierQueueIndependentOfFailedAlertmanager is a regression test for
// https://github.com/prometheus/prometheus/issues/7676.
// The test creates a black hole alertmanager that never responds to any requests,
// with an alertmanager_config.timeout set to a year. The alerts must still reach
// the healthy alertmanager, and Send must not block.
func TestNotifierQueueIndependentOfFailedAlertmanager(t *testing.T) {
	stopBlackHole := make(chan struct{})
	blackHoleAM := newBlackHoleAlertmanager(stopBlackHole)
	defer func() {
		close(stopBlackHole)
		blackHoleAM.Close()
	}()

	doneAlertReceive := make(chan struct{})
	immediateAM := newImmediateAlertmanager(doneAlertReceive)
	defer immediateAM.Close()

	h := NewManager(&Options{
		Do:            do,
		QueueCapacity: 10,
	}, nil)

	h.alertmanagers = make(map[string]*alertmanagerSet)

	amCfg := config.DefaultAlertmanagerConfig
	amCfg.Timeout = model.Duration(365 * 24 * time.Hour)

	h.alertmanagers["1"] = &alertmanagerSet{
		ams: []alertmanager{
			alertmanagerMock{
				urlf: func() string { return blackHoleAM.URL },
			},
		},
		cfg:  &amCfg,
		opts: h.opts,
		sendLoops: map[string]*sendLoop{
			blackHoleAM.URL: newSendLoop(blackHoleAM.URL, http.DefaultClient, &amCfg, h.opts, log.NewNopLogger(), h.metrics),
		},
		loops:   &h.loops,
		metrics: h.metrics,
		logger:  log.NewNopLogger(),
	}

	h.alertmanagers["2"] = &alertmanagerSet{
		ams: []alertmanager{
			alertmanagerMock{
				urlf: func() string { return immediateAM.URL },
			},
		},
		cfg:  &amCfg,
		opts: h.opts,
		sendLoops: map[string]*sendLoop{
			immediateAM.URL: newSendLoop(immediateAM.URL, http.DefaultClient, &amCfg, h.opts, log.NewNopLogger(), h.metrics),
		},
		loops:   &h.loops,
		metrics: h.metrics,
		logger:  log.NewNopLogger(),
	}

	for _, ams := range h.alertmanagers {
		ams.startSendLoops(ams.ams)
	}

	doneSend := make(chan struct{})
	go func() {
		h.Send(&Alert{
			Labels: labels.FromStrings("alertname", "test"),
		})
		close(doneSend)
	}()

	select {
	case <-doneAlertReceive:
		// The happy case, the alert was received by the healthy alertmanager.
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for the alert to be received by the healthy alertmanager")
	}

	select {
	case <-doneSend:
		// The happy case, Send returned.
	case <-time.After(10 * time.Second):
		t.Fatal("Timeout waiting for Send to return")
	}
}

func newBlackHoleAlertmanager(stop <-chan struct{}) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		// Do nothing, wait to be canceled.
		<-stop
		w.WriteHeader(http.StatusOK)
	}))
}

func newImmediateAlertmanager(done chan<- struct{}) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		close(done)
	}))
}

// sendLoopStopped reports whether the send loop has been asked to stop.
func sendLoopStopped(s *sendLoop) bool {
	select {
	case <-s.stopped:
		return true
	default:
		return false
	}
}

// alertmanagerConfigs unmarshals an alerting config for ApplyConfig.
func alertmanagerConfigs(t *testing.T, s string) *config.Config {
	t.Helper()

	cfg := &config.Config{}
	require.NoError(t, yaml.UnmarshalStrict([]byte(s), cfg))

	return cfg
}

// TestApplyConfigStopsSetsThatWereNotReused ensures a reload hands over every
// known alertmanager set to at most one set of the new config and stops the
// ones left over, whatever position they held in the previous config.
func TestApplyConfigStopsSetsThatWereNotReused(t *testing.T) {
	const targetURL = "alertmanager:9093"

	targetGroup := &targetgroup.Group{
		Targets: []model.LabelSet{{"__address__": model.LabelValue(targetURL)}},
	}

	twoSets := `
alerting:
  alertmanagers:
  - path_prefix: /first
    file_sd_configs:
    - files:
      - foo.json
  - path_prefix: /second
    file_sd_configs:
    - files:
      - foo.json
`
	secondSetOnly := `
alerting:
  alertmanagers:
  - path_prefix: /second
    file_sd_configs:
    - files:
      - foo.json
`

	t.Run("a removed set does not take the send loops of the set that stays", func(t *testing.T) {
		n := NewManager(&Options{QueueCapacity: 10}, nil)

		require.NoError(t, n.ApplyConfig(alertmanagerConfigs(t, twoSets)))
		n.reload(map[string][]*targetgroup.Group{
			"config-0": {targetGroup},
			"config-1": {targetGroup},
		})

		firstURL := fmt.Sprintf("http://%s/first/api/v2/alerts", targetURL)
		secondURL := fmt.Sprintf("http://%s/second/api/v2/alerts", targetURL)
		removed := n.alertmanagers["config-0"].sendLoops[firstURL]
		kept := n.alertmanagers["config-1"].sendLoops[secondURL]
		require.NotNil(t, removed)
		require.NotNil(t, kept)

		// The set that stays moves from config-1 to config-0, so a cleanup by
		// position would stop the send loop it just inherited.
		require.NoError(t, n.ApplyConfig(alertmanagerConfigs(t, secondSetOnly)))

		require.Same(t, kept, n.alertmanagers["config-0"].sendLoops[secondURL])
		require.False(t, sendLoopStopped(kept))
		require.True(t, sendLoopStopped(removed))
	})

	t.Run("a changed set stops the send loops of the config it replaces", func(t *testing.T) {
		n := NewManager(&Options{QueueCapacity: 10}, nil)

		require.NoError(t, n.ApplyConfig(alertmanagerConfigs(t, secondSetOnly)))
		n.reload(map[string][]*targetgroup.Group{"config-0": {targetGroup}})

		secondURL := fmt.Sprintf("http://%s/second/api/v2/alerts", targetURL)
		replaced := n.alertmanagers["config-0"].sendLoops[secondURL]
		require.NotNil(t, replaced)

		// Only the timeout changes, so the set keeps its position but not its
		// config hash: its send loop must not be left running without an owner.
		changed := `
alerting:
  alertmanagers:
  - path_prefix: /second
    timeout: 23s
    file_sd_configs:
    - files:
      - foo.json
`
		require.NoError(t, n.ApplyConfig(alertmanagerConfigs(t, changed)))

		require.Empty(t, n.alertmanagers["config-0"].sendLoops)
		require.True(t, sendLoopStopped(replaced))
	})

	t.Run("duplicated configs do not share send loops", func(t *testing.T) {
		n := NewManager(&Options{QueueCapacity: 10}, nil)

		require.NoError(t, n.ApplyConfig(alertmanagerConfigs(t, secondSetOnly)))
		n.reload(map[string][]*targetgroup.Group{"config-0": {targetGroup}})

		secondURL := fmt.Sprintf("http://%s/second/api/v2/alerts", targetURL)
		loop := n.alertmanagers["config-0"].sendLoops[secondURL]
		require.NotNil(t, loop)

		// The config now holds the same alertmanager config twice. Only one of
		// the two sets inherits the known send loop, the other one waits for
		// its own service discovery update.
		duplicated := secondSetOnly + `  - path_prefix: /second
    file_sd_configs:
    - files:
      - foo.json
`
		require.NoError(t, n.ApplyConfig(alertmanagerConfigs(t, duplicated)))

		require.Len(t, n.alertmanagers, 2)
		first := n.alertmanagers["config-0"].sendLoops[secondURL]
		second := n.alertmanagers["config-1"].sendLoops[secondURL]
		require.NotNil(t, first)
		require.NotNil(t, second)
		// Each set has its own send loop and its own queue: one of them took
		// over the known queue, the other one started a new one.
		require.NotSame(t, first, second)
		require.False(t, sendLoopStopped(loop))

		// Dropping the duplicate keeps a running send loop for the set that stays.
		require.NoError(t, n.ApplyConfig(alertmanagerConfigs(t, secondSetOnly)))

		require.Len(t, n.alertmanagers, 1)
		kept := n.alertmanagers["config-0"].sendLoops[secondURL]
		require.NotNil(t, kept)
		require.True(t, kept == first || kept == second)
		require.False(t, sendLoopStopped(kept))
		require.NotEqual(t, sendLoopStopped(first), sendLoopStopped(second))
	})
}

// TestApplyConfigKeepsTheRunningConfigOnError ensures a rejected config leaves
// the running one alone: the sets of the running config keep their send loops
// and their queues, and no send loop of a discarded set is left running.
func TestApplyConfigKeepsTheRunningConfigOnError(t *testing.T) {
	received := make(chan string, 10)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		received <- r.URL.Path
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	targetGroup := &targetgroup.Group{
		Targets: []model.LabelSet{{"__address__": model.LabelValue(serverURL.Host)}},
	}

	running := `
alerting:
  alertmanagers:
  - path_prefix: /first
    file_sd_configs:
    - files:
      - foo.json
  - path_prefix: /second
    file_sd_configs:
    - files:
      - foo.json
`
	// The third config fails to build, as its CA file does not exist. Which
	// config ApplyConfig reaches first depends on the map order, so the state
	// of every set of the running config is checked after every attempt.
	rejected := running + `  - path_prefix: /third
    tls_config:
      ca_file: /nonexistent/ca.pem
    file_sd_configs:
    - files:
      - foo.json
`

	n := NewManager(&Options{Do: do, QueueCapacity: 10}, nil)
	require.NoError(t, n.ApplyConfig(alertmanagerConfigs(t, running)))
	n.reload(map[string][]*targetgroup.Group{
		"config-0": {targetGroup},
		"config-1": {targetGroup},
	})

	runReturned := make(chan struct{})
	go func() {
		n.Run(nil)
		close(runReturned)
	}()

	for i := range 20 {
		require.Error(t, n.ApplyConfig(alertmanagerConfigs(t, rejected)), "attempt %d", i)

		n.mtx.RLock()
		require.Len(t, n.alertmanagers, 2, "attempt %d", i)
		for key, ams := range n.alertmanagers {
			ams.mtx.RLock()
			loops := len(ams.sendLoops)
			ams.mtx.RUnlock()
			require.Equal(t, 1, loops, "set %s lost its send loop on attempt %d", key, i)
		}
		n.mtx.RUnlock()
	}

	// Both alertmanagers of the running config still receive notifications.
	n.Send(&Alert{Labels: labels.FromStrings("alertname", "after-a-rejected-config")})
	paths := make([]string, 0, 2)
	for range 2 {
		select {
		case path := <-received:
			paths = append(paths, path)
		case <-time.After(10 * time.Second):
			require.FailNow(t, "an alertmanager of the running config received no notification")
		}
	}
	require.ElementsMatch(t, []string{"/first/api/v2/alerts", "/second/api/v2/alerts"}, paths)

	// A send loop of a discarded set would keep Run from returning.
	n.Stop()
	select {
	case <-runReturned:
	case <-time.After(10 * time.Second):
		require.FailNow(t, "Run did not return after Stop")
	}
}

// TestApplyConfigUpdatesSendLoopClient ensures an inherited send loop sends with
// the client of the new config. The config hash cannot tell two configs that
// differ only in their credentials apart, as a secret marshals as a placeholder.
func TestApplyConfigUpdatesSendLoopClient(t *testing.T) {
	var authorization atomic.String
	received := make(chan struct{}, 1)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization.Store(r.Header.Get("Authorization"))
		w.WriteHeader(http.StatusOK)
		select {
		case received <- struct{}{}:
		default:
		}
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)

	configWithPassword := func(password string) *config.Config {
		return alertmanagerConfigs(t, fmt.Sprintf(`
alerting:
  alertmanagers:
  - timeout: 5s
    basic_auth:
      username: user
      password: %s
    file_sd_configs:
    - files:
      - foo.json
`, password))
	}
	expectedAuthorization := func(password string) string {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte("user:"+password))
	}

	n := NewManager(&Options{QueueCapacity: 10}, nil)

	require.NoError(t, n.ApplyConfig(configWithPassword("first")))
	n.reload(map[string][]*targetgroup.Group{
		"config-0": {{Targets: []model.LabelSet{{"__address__": model.LabelValue(serverURL.Host)}}}},
	})

	alertmanagerURL := fmt.Sprintf("http://%s/api/v2/alerts", serverURL.Host)
	loop := n.alertmanagers["config-0"].sendLoops[alertmanagerURL]
	require.NotNil(t, loop)

	n.Send(&Alert{Labels: labels.FromStrings("alertname", "first")})
	select {
	case <-received:
	case <-time.After(10 * time.Second):
		require.FailNow(t, "gave up waiting for the first alert")
	}
	require.Equal(t, expectedAuthorization("first"), authorization.Load())

	// Rotate the password. The send loop keeps its queue and picks up the new
	// client of its set.
	require.NoError(t, n.ApplyConfig(configWithPassword("second")))
	require.Same(t, loop, n.alertmanagers["config-0"].sendLoops[alertmanagerURL])

	n.Send(&Alert{Labels: labels.FromStrings("alertname", "second")})
	require.Eventually(t, func() bool {
		return authorization.Load() == expectedAuthorization("second")
	}, 10*time.Second, 10*time.Millisecond, "the send loop kept sending with the old password")
}

// TestApplyConfigDoesNotWaitForDrainingQueues ensures a removed Alertmanager
// drains its queue on its own goroutine: a reload holds the manager lock, so
// draining under it would block Send for as long as the removed Alertmanager
// takes to answer.
func TestApplyConfigDoesNotWaitForDrainingQueues(t *testing.T) {
	stopBlackHole := make(chan struct{})
	blackHoleAM := newBlackHoleAlertmanager(stopBlackHole)
	defer func() {
		close(stopBlackHole)
		blackHoleAM.Close()
	}()

	amCfg := config.DefaultAlertmanagerConfig
	amCfg.Timeout = model.Duration(365 * 24 * time.Hour)

	h := NewManager(&Options{Do: do, QueueCapacity: 10, DrainOnShutdown: true}, nil)
	h.alertmanagers = map[string]*alertmanagerSet{
		"1": {
			ams: []alertmanager{
				alertmanagerMock{urlf: func() string { return blackHoleAM.URL }},
			},
			cfg:  &amCfg,
			opts: h.opts,
			sendLoops: map[string]*sendLoop{
				blackHoleAM.URL: newSendLoop(blackHoleAM.URL, http.DefaultClient, &amCfg, h.opts, log.NewNopLogger(), h.metrics),
			},
			loops:   &h.loops,
			metrics: h.metrics,
			logger:  log.NewNopLogger(),
		},
	}
	for _, ams := range h.alertmanagers {
		ams.startSendLoops(ams.ams)
	}

	// The first alert hangs in the request to the black hole Alertmanager, the
	// second one stays in the queue to be drained on shutdown.
	h.Send(&Alert{Labels: labels.FromStrings("alertname", "in-flight")})
	require.Eventually(t, func() bool {
		return prom_testutil.ToFloat64(h.metrics.queueLength.WithLabelValues(blackHoleAM.URL)) == 0
	}, 10*time.Second, 10*time.Millisecond, "gave up waiting for the alert to be picked up")
	h.Send(&Alert{Labels: labels.FromStrings("alertname", "queued")})

	applied := make(chan error, 1)
	go func() {
		applied <- h.ApplyConfig(&config.Config{})
	}()

	select {
	case err := <-applied:
		require.NoError(t, err)
	case <-time.After(10 * time.Second):
		require.FailNow(t, "ApplyConfig blocked on the queue of the removed Alertmanager")
	}

	sent := make(chan struct{})
	go func() {
		h.Send(&Alert{Labels: labels.FromStrings("alertname", "after-reload")})
		close(sent)
	}()

	select {
	case <-sent:
	case <-time.After(10 * time.Second):
		require.FailNow(t, "Send blocked on the queue of the removed Alertmanager")
	}
}

// TestRunWaitsForInFlightNotifications ensures Run only returns once the send
// loops have finished the request they were in, as its contract promises that
// nothing is sent after it returned.
func TestRunWaitsForInFlightNotifications(t *testing.T) {
	requestReceived := make(chan struct{})
	releaseRequest := make(chan struct{})

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		close(requestReceived)
		<-releaseRequest
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	amCfg := config.DefaultAlertmanagerConfig
	amCfg.Timeout = model.Duration(time.Minute)

	h := NewManager(&Options{Do: do, QueueCapacity: 10}, nil)
	h.alertmanagers = map[string]*alertmanagerSet{
		"1": {
			ams: []alertmanager{
				alertmanagerMock{urlf: func() string { return server.URL }},
			},
			cfg:  &amCfg,
			opts: h.opts,
			sendLoops: map[string]*sendLoop{
				server.URL: newSendLoop(server.URL, http.DefaultClient, &amCfg, h.opts, log.NewNopLogger(), h.metrics),
			},
			loops:   &h.loops,
			metrics: h.metrics,
			logger:  log.NewNopLogger(),
		},
	}
	for _, ams := range h.alertmanagers {
		ams.startSendLoops(ams.ams)
	}

	runReturned := make(chan struct{})
	go func() {
		h.Run(nil)
		close(runReturned)
	}()

	h.Send(&Alert{Labels: labels.FromStrings("alertname", "test")})

	select {
	case <-requestReceived:
	case <-time.After(10 * time.Second):
		require.FailNow(t, "gave up waiting for the notification to be sent")
	}

	h.Stop()

	select {
	case <-runReturned:
		require.FailNow(t, "Run returned while a notification was still in flight")
	case <-time.After(100 * time.Millisecond):
	}

	close(releaseRequest)

	select {
	case <-runReturned:
	case <-time.After(10 * time.Second):
		require.FailNow(t, "gave up waiting for Run to return")
	}
}
