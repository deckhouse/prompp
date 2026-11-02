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
	require.Zero(t, prom_testutil.ToFloat64(s.metrics.dropped.WithLabelValues(s.alertmanagerURL)))

	// Verify all new alerts were added to the queue.
	require.Equal(t, []*Alert{alert1, alert2, alert3, alert4}, s.queue)
	require.Equal(t, 4.0, prom_testutil.ToFloat64(s.metrics.queueLength.WithLabelValues(s.alertmanagerURL)))
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
	require.Equal(t, 1.0, prom_testutil.ToFloat64(s.metrics.dropped.WithLabelValues(s.alertmanagerURL)))

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
	require.Equal(t, 3.0, prom_testutil.ToFloat64(s.metrics.dropped.WithLabelValues(s.alertmanagerURL)))

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
// PP_CHANGES.md: add successfully send metric
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

// TestSendLoopMetricsCleanedOnStop ensures a restarted send loop for the same
// Alertmanager does not conflict with the metrics of the stopped one.
func TestSendLoopMetricsCleanedOnStop(t *testing.T) {
	const alertmanagerURL = "http://alertmanager:9093"

	// Use a single registry throughout the test - this is critical to catch registry conflicts.
	metrics := newAlertMetrics(prometheus.NewRegistry(), 10, func() float64 { return 0 })

	opts := &Options{QueueCapacity: 10}

	// Create the first sendLoop - this initializes the metrics with the alertmanager URL label.
	sendLoop1 := newSendLoop(alertmanagerURL, nil, &config.DefaultAlertmanagerConfig, opts, log.NewNopLogger(), metrics)

	require.Equal(t, 0.0, prom_testutil.ToFloat64(metrics.dropped.WithLabelValues(alertmanagerURL)))
	require.Equal(t, 0.0, prom_testutil.ToFloat64(metrics.sent.WithLabelValues(alertmanagerURL)))

	// Stop the sendLoop - this should clean up all the metrics.
	sendLoop1.stop()

	// Create a second sendLoop with the same URL - this should not panic or conflict.
	sendLoop2 := newSendLoop(alertmanagerURL, nil, &config.DefaultAlertmanagerConfig, opts, log.NewNopLogger(), metrics)
	defer sendLoop2.stop()

	require.Equal(t, 0.0, prom_testutil.ToFloat64(metrics.dropped.WithLabelValues(alertmanagerURL)))
	require.Equal(t, 0.0, prom_testutil.ToFloat64(metrics.sent.WithLabelValues(alertmanagerURL)))
}
