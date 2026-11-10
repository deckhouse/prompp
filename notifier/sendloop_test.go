// Copyright 2025 The Prometheus Authors
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
	"bytes"
	"context"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	prom_testutil "github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
)

func testAlertMetrics() *alertMetrics {
	return newAlertMetrics(prometheus.NewRegistry(), 0, func() float64 { return 0 })
}

func TestCustomDo(t *testing.T) {
	const testURL = "http://testurl.com/"
	const testBody = "testbody"

	var received bool
	h := sendLoop{
		logger: log.NewNopLogger(),
		opts: &Options{
			Do: func(_ context.Context, _ *http.Client, req *http.Request) (*http.Response, error) {
				received = true
				body, err := io.ReadAll(req.Body)

				require.NoError(t, err)

				require.Equal(t, testBody, string(body))

				require.Equal(t, testURL, req.URL.String())

				return &http.Response{
					Body: io.NopCloser(bytes.NewBuffer(nil)),
				}, nil
			},
		},
	}

	h.sendOne(context.Background(), nil, testURL, []byte(testBody))

	require.True(t, received, "Expected to receive an alert, but didn't")
}

func TestHandlerNextBatch(t *testing.T) {
	s := newSendLoop("http://mock", nil, &config.DefaultAlertmanagerConfig, &Options{QueueCapacity: 3 * maxBatchSize}, log.NewNopLogger(), testAlertMetrics())

	for i := range make([]struct{}, 2*maxBatchSize+1) {
		s.queue = append(s.queue, &Alert{
			Labels: labels.FromStrings("alertname", strconv.Itoa(i)),
		})
	}

	expected := append([]*Alert{}, s.queue...)

	require.NoError(t, alertsEqual(expected[0:maxBatchSize], s.nextBatch()))
	require.NoError(t, alertsEqual(expected[maxBatchSize:2*maxBatchSize], s.nextBatch()))
	require.NoError(t, alertsEqual(expected[2*maxBatchSize:], s.nextBatch()))
	require.Empty(t, s.queue, "Expected queue to be empty but got %d alerts", len(s.queue))
}

func TestAddAlertsToQueue(t *testing.T) {
	alert1 := &Alert{Labels: labels.FromStrings("alertname", "existing1")}
	alert2 := &Alert{Labels: labels.FromStrings("alertname", "existing2")}

	s := newSendLoop("http://foo.bar/", nil, nil, &Options{QueueCapacity: 5}, log.NewNopLogger(), testAlertMetrics())
	s.add(alert1, alert2)
	require.Equal(t, []*Alert{alert1, alert2}, s.queue)

	alert3 := &Alert{Labels: labels.FromStrings("alertname", "new1")}
	alert4 := &Alert{Labels: labels.FromStrings("alertname", "new2")}

	// Add new alerts to the queue, expect 0 dropped.
	s.add(alert3, alert4)
	require.Zero(t, prom_testutil.ToFloat64(s.metrics.all.dropped.WithLabelValues(s.alertmanagerURL)))

	// Verify all new alerts were added to the queue.
	require.Equal(t, []*Alert{alert1, alert2, alert3, alert4}, s.queue)
	require.Equal(t, 4.0, prom_testutil.ToFloat64(s.metrics.all.queueLength.WithLabelValues(s.alertmanagerURL)))
}

func TestAddAlertsToQueueExceedingCapacity(t *testing.T) {
	alert1 := &Alert{Labels: labels.FromStrings("alertname", "alert1")}
	alert2 := &Alert{Labels: labels.FromStrings("alertname", "alert2")}

	s := newSendLoop("http://foo.bar/", nil, nil, &Options{QueueCapacity: 3}, log.NewNopLogger(), testAlertMetrics())
	s.add(alert1, alert2)

	alert3 := &Alert{Labels: labels.FromStrings("alertname", "alert3")}
	alert4 := &Alert{Labels: labels.FromStrings("alertname", "alert4")}

	// Add new alerts to the queue, expect 1 dropped.
	s.add(alert3, alert4)
	require.Equal(t, 1.0, prom_testutil.ToFloat64(s.metrics.all.dropped.WithLabelValues(s.alertmanagerURL)))

	// The oldest alert was dropped in favor of the newer ones.
	require.Equal(t, []*Alert{alert2, alert3, alert4}, s.queue)
}

func TestAddAlertsToQueueExceedingTotalCapacity(t *testing.T) {
	alert1 := &Alert{Labels: labels.FromStrings("alertname", "alert1")}
	alert2 := &Alert{Labels: labels.FromStrings("alertname", "alert2")}

	s := newSendLoop("http://foo.bar/", nil, nil, &Options{QueueCapacity: 3}, log.NewNopLogger(), testAlertMetrics())
	s.add(alert1, alert2)

	alert3 := &Alert{Labels: labels.FromStrings("alertname", "alert3")}
	alert4 := &Alert{Labels: labels.FromStrings("alertname", "alert4")}
	alert5 := &Alert{Labels: labels.FromStrings("alertname", "alert5")}
	alert6 := &Alert{Labels: labels.FromStrings("alertname", "alert6")}

	// Add new alerts to the queue, expect 3 dropped: 1 from the new batch + 2 from the existing queued items.
	s.add(alert3, alert4, alert5, alert6)
	require.Equal(t, 3.0, prom_testutil.ToFloat64(s.metrics.all.dropped.WithLabelValues(s.alertmanagerURL)))

	require.Equal(t, []*Alert{alert4, alert5, alert6}, s.queue)
}

func TestNextBatchAlertsFromQueue(t *testing.T) {
	s := newSendLoop("http://foo.bar/", nil, nil, &Options{QueueCapacity: 2 * maxBatchSize}, log.NewNopLogger(), testAlertMetrics())

	var alerts []*Alert
	for i := range make([]struct{}, maxBatchSize) {
		alerts = append(alerts, &Alert{Labels: labels.FromStrings("alertname", strconv.Itoa(i))})
	}

	// A single batch-sized queue is returned in one batch.
	s.add(alerts...)
	require.Equal(t, alerts, s.nextBatch())
	require.Empty(t, s.nextBatch())

	// A queue larger than the batch size is split into batches.
	extra := []*Alert{
		{Labels: labels.FromStrings("alertname", "extra1")},
		{Labels: labels.FromStrings("alertname", "extra2")},
	}
	s.add(append(append([]*Alert{}, alerts...), extra...)...)
	require.Equal(t, alerts, s.nextBatch())
	require.Equal(t, extra, s.nextBatch())
	require.Empty(t, s.nextBatch())
}

// TestMetricsAreRegistered ensures every alert metric reaches the registry.
// PP_CHANGES.md: add successfully send metric.
func TestMetricsAreRegistered(t *testing.T) {
	reg := prometheus.NewRegistry()
	metrics := newAlertMetrics(reg, 10, func() float64 { return 0 })

	// A send loop initializes the per-Alertmanager metrics, which a vector only
	// reports once it has children.
	newSendLoop("http://foo.bar/", nil, &config.DefaultAlertmanagerConfig, &Options{QueueCapacity: 10}, log.NewNopLogger(), metrics)

	families, err := reg.Gather()
	require.NoError(t, err)

	names := make([]string, 0, len(families))
	for _, f := range families {
		names = append(names, f.GetName())
	}

	require.Subset(t, names, []string{
		"prometheus_notifications_alertmanagers_discovered",
		"prometheus_notifications_dropped_total",
		"prometheus_notifications_errors_total",
		"prometheus_notifications_queue_capacity",
		"prometheus_notifications_queue_length",
		"prometheus_notifications_sent_total",
		"prometheus_notifications_successfully_sent_total",
	})
}

// okDo answers every request with 200 OK, so that a send loop can run without
// an Alertmanager.
func okDo(_ context.Context, _ *http.Client, _ *http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     "200 OK",
		Body:       io.NopCloser(bytes.NewReader(nil)),
	}, nil
}

// runSendLoop starts the given send loop and returns a function that stops it
// and waits for its shutdown, as the manager does.
func runSendLoop(s *sendLoop) func() {
	go s.loop()

	return func() {
		s.stop()
		s.wait()
	}
}

// seriesOf counts the series of the given Alertmanager in the registry.
func seriesOf(t *testing.T, reg *prometheus.Registry, alertmanagerURL string) int {
	t.Helper()

	families, err := reg.Gather()
	require.NoError(t, err)

	var series int
	for _, f := range families {
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == alertmanagerLabel && l.GetValue() == alertmanagerURL {
					series++
				}
			}
		}
	}

	return series
}

// latencySampleCount returns how many requests the latency summary of the given
// Alertmanager holds.
func latencySampleCount(t *testing.T, reg *prometheus.Registry, alertmanagerURL string) uint64 {
	t.Helper()

	families, err := reg.Gather()
	require.NoError(t, err)

	for _, f := range families {
		if f.GetName() != "prometheus_notifications_latency_seconds" {
			continue
		}
		for _, m := range f.GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetName() == alertmanagerLabel && l.GetValue() == alertmanagerURL {
					return m.GetSummary().GetSampleCount()
				}
			}
		}
	}

	return 0
}

// TestSendLoopMetricsCleanedOnStop ensures a restarted send loop for the same
// Alertmanager does not conflict with the metrics of the stopped one.
func TestSendLoopMetricsCleanedOnStop(t *testing.T) {
	const alertmanagerURL = "http://alertmanager:9093"

	// Use a single registry throughout the test - this is critical to catch registry conflicts.
	reg := prometheus.NewRegistry()
	metrics := newAlertMetrics(reg, 10, func() float64 { return 0 })

	opts := &Options{QueueCapacity: 10, Do: okDo}

	// Create the first sendLoop - this initializes the metrics with the alertmanager URL label.
	sendLoop1 := newSendLoop(alertmanagerURL, nil, &config.DefaultAlertmanagerConfig, opts, log.NewNopLogger(), metrics)
	stop1 := runSendLoop(sendLoop1)

	require.Equal(t, 0.0, prom_testutil.ToFloat64(metrics.dropped.WithLabelValues(alertmanagerURL)))
	require.Equal(t, 0.0, prom_testutil.ToFloat64(metrics.sent.WithLabelValues(alertmanagerURL)))

	// Stop the sendLoop - this should clean up all the metrics.
	stop1()
	require.Zero(t, seriesOf(t, reg, alertmanagerURL))

	// Create a second sendLoop with the same URL - this should not panic or conflict.
	sendLoop2 := newSendLoop(alertmanagerURL, nil, &config.DefaultAlertmanagerConfig, opts, log.NewNopLogger(), metrics)
	defer runSendLoop(sendLoop2)()

	require.Equal(t, 0.0, prom_testutil.ToFloat64(metrics.dropped.WithLabelValues(alertmanagerURL)))
	require.Equal(t, 0.0, prom_testutil.ToFloat64(metrics.sent.WithLabelValues(alertmanagerURL)))
}

// TestSendLoopMetricsSharedURL ensures the series of an Alertmanager survive as
// long as a send loop reports under its URL, and that queue_length holds the
// queues of all those loops. Two Alertmanager configurations that differ only in
// a field the URL does not carry, their timeout for instance, share the series
// of their common endpoint.
func TestSendLoopMetricsSharedURL(t *testing.T) {
	const alertmanagerURL = "http://alertmanager:9093"

	reg := prometheus.NewRegistry()
	metrics := newAlertMetrics(reg, 10, func() float64 { return 0 })

	opts := &Options{QueueCapacity: 10, Do: okDo}

	first := newSendLoop(alertmanagerURL, nil, &config.DefaultAlertmanagerConfig, opts, log.NewNopLogger(), metrics)
	second := newSendLoop(alertmanagerURL, nil, &config.DefaultAlertmanagerConfig, opts, log.NewNopLogger(), metrics)

	first.add(&Alert{Labels: labels.FromStrings("alertname", "first")})
	second.add(
		&Alert{Labels: labels.FromStrings("alertname", "second")},
		&Alert{Labels: labels.FromStrings("alertname", "third")},
	)

	// queue_length holds the queues of both send loops.
	require.Equal(t, 3.0, prom_testutil.ToFloat64(metrics.queueLength.WithLabelValues(alertmanagerURL)))

	second.nextBatch()
	require.Equal(t, 1.0, prom_testutil.ToFloat64(metrics.queueLength.WithLabelValues(alertmanagerURL)))

	second.metrics.addSent(2)

	// Stopping one send loop keeps the series of the other one.
	runSendLoop(second)()
	require.Equal(t, 2.0, prom_testutil.ToFloat64(metrics.sent.WithLabelValues(alertmanagerURL)))
	require.Equal(t, 1.0, prom_testutil.ToFloat64(metrics.queueLength.WithLabelValues(alertmanagerURL)))

	// The series are removed with the last send loop of the URL.
	first.metrics.release()
	require.Zero(t, seriesOf(t, reg, alertmanagerURL))
}

// TestSendLoopMetricsCountAttempts ensures a failed request counts towards
// sent_total and the latency, as the error share compares errors_total with
// them, while successfully_sent_total only counts what an Alertmanager took.
// PP_CHANGES.md: add successfully send metric.
func TestSendLoopMetricsCountAttempts(t *testing.T) {
	const alertmanagerURL = "http://alertmanager:9093"

	reg := prometheus.NewRegistry()
	metrics := newAlertMetrics(reg, 10, func() float64 { return 0 })

	status := http.StatusInternalServerError
	opts := &Options{
		QueueCapacity: 10,
		Do: func(_ context.Context, _ *http.Client, _ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: status,
				Status:     strconv.Itoa(status),
				Body:       io.NopCloser(bytes.NewReader(nil)),
			}, nil
		},
	}

	s := newSendLoop(alertmanagerURL, nil, &config.DefaultAlertmanagerConfig, opts, log.NewNopLogger(), metrics)
	alerts := []*Alert{{Labels: labels.FromStrings("alertname", "test")}}

	require.False(t, s.sendAll(alerts))
	require.Equal(t, 1.0, prom_testutil.ToFloat64(metrics.sent.WithLabelValues(alertmanagerURL)))
	require.Equal(t, 1.0, prom_testutil.ToFloat64(metrics.errors.WithLabelValues(alertmanagerURL)))
	require.Equal(t, 0.0, prom_testutil.ToFloat64(metrics.successfullySent.WithLabelValues(alertmanagerURL)))

	status = http.StatusOK
	require.True(t, s.sendAll(alerts))
	require.Equal(t, 2.0, prom_testutil.ToFloat64(metrics.sent.WithLabelValues(alertmanagerURL)))
	require.Equal(t, 1.0, prom_testutil.ToFloat64(metrics.errors.WithLabelValues(alertmanagerURL)))
	require.Equal(t, 1.0, prom_testutil.ToFloat64(metrics.successfullySent.WithLabelValues(alertmanagerURL)))

	// Both attempts are in the latency summary.
	require.Equal(t, uint64(2), latencySampleCount(t, reg, alertmanagerURL))
}
