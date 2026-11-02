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
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/prometheus/prometheus/config"
)

// maxBatchSize is the maximum number of alerts a send loop puts in one request.
const maxBatchSize = 64

// sendLoop holds the notification queue of a single Alertmanager and sends its
// batches independently of the other Alertmanagers.
type sendLoop struct {
	alertmanagerURL string

	cfg    *config.AlertmanagerConfig
	client *http.Client
	opts   *Options

	metrics *alertMetrics

	mtx      sync.RWMutex
	queue    []*Alert
	hasWork  chan struct{}
	stopped  chan struct{}
	stopOnce sync.Once

	logger log.Logger
}

func newSendLoop(
	alertmanagerURL string,
	client *http.Client,
	cfg *config.AlertmanagerConfig,
	opts *Options,
	logger log.Logger,
	metrics *alertMetrics,
) *sendLoop {
	// This will initialize the Counters for the AM to 0.
	metrics.dropped.WithLabelValues(alertmanagerURL)
	metrics.errors.WithLabelValues(alertmanagerURL)
	metrics.sent.WithLabelValues(alertmanagerURL)
	// PP_CHANGES.md: add successfully send metric
	metrics.successfullySent.WithLabelValues(alertmanagerURL)
	metrics.queueLength.WithLabelValues(alertmanagerURL)

	return &sendLoop{
		alertmanagerURL: alertmanagerURL,
		client:          client,
		cfg:             cfg,
		opts:            opts,
		logger:          logger,
		metrics:         metrics,
		queue:           make([]*Alert, 0, opts.QueueCapacity),
		hasWork:         make(chan struct{}, 1),
		stopped:         make(chan struct{}),
	}
}

// add queues the given alerts, dropping the oldest ones if the queue is full.
func (s *sendLoop) add(alerts ...*Alert) {
	select {
	case <-s.stopped:
		return
	default:
	}

	s.mtx.Lock()
	defer s.mtx.Unlock()

	var dropped int
	// Queue capacity should be significantly larger than a single alert
	// batch could be.
	if d := len(alerts) - s.opts.QueueCapacity; d > 0 {
		level.Warn(s.logger).Log("msg", "Alert batch larger than queue capacity, dropping alerts", "num_dropped", d)
		dropped += d
		alerts = alerts[d:]
	}

	// If the queue is full, remove the oldest alerts in favor
	// of newer ones.
	if d := (len(s.queue) + len(alerts)) - s.opts.QueueCapacity; d > 0 {
		level.Warn(s.logger).Log("msg", "Alert notification queue full, dropping alerts", "num_dropped", d)
		dropped += d
		s.queue = s.queue[d:]
	}

	s.queue = append(s.queue, alerts...)

	// Notify the sending goroutine that there are alerts to be processed.
	s.notifyWork()

	s.metrics.queueLength.WithLabelValues(s.alertmanagerURL).Set(float64(len(s.queue)))
	if dropped > 0 {
		s.metrics.dropped.WithLabelValues(s.alertmanagerURL).Add(float64(dropped))
	}
}

// notifyWork signals that the alert queue has items. If we cannot send on the
// channel, it means the signal already exists and has not been consumed yet.
func (s *sendLoop) notifyWork() {
	select {
	case <-s.stopped:
		return
	case s.hasWork <- struct{}{}:
	default:
	}
}

// stop drains the queue if draining is enabled, then removes the metrics of
// this Alertmanager. It is safe to call multiple times.
func (s *sendLoop) stop() {
	s.stopOnce.Do(func() {
		level.Debug(s.logger).Log("msg", "Stopping send loop")
		close(s.stopped)

		if s.opts.DrainOnShutdown {
			s.drainQueue()
		} else if ql := s.queueLen(); ql > 0 {
			level.Warn(s.logger).Log("msg", "Alert notification queue not drained on shutdown, dropping alerts", "count", ql)
			s.metrics.dropped.WithLabelValues(s.alertmanagerURL).Add(float64(ql))
		}

		s.metrics.latency.DeleteLabelValues(s.alertmanagerURL)
		s.metrics.sent.DeleteLabelValues(s.alertmanagerURL)
		// PP_CHANGES.md: add successfully send metric
		s.metrics.successfullySent.DeleteLabelValues(s.alertmanagerURL)
		s.metrics.dropped.DeleteLabelValues(s.alertmanagerURL)
		s.metrics.errors.DeleteLabelValues(s.alertmanagerURL)
		s.metrics.queueLength.DeleteLabelValues(s.alertmanagerURL)
	})
}

func (s *sendLoop) drainQueue() {
	for s.queueLen() > 0 {
		s.sendOneBatch()
	}
}

func (s *sendLoop) queueLen() int {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return len(s.queue)
}

func (s *sendLoop) nextBatch() []*Alert {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	var alerts []*Alert
	if len(s.queue) > maxBatchSize {
		alerts = append(make([]*Alert, 0, maxBatchSize), s.queue[:maxBatchSize]...)
		s.queue = s.queue[maxBatchSize:]
	} else {
		alerts = append(make([]*Alert, 0, len(s.queue)), s.queue...)
		s.queue = s.queue[:0]
	}
	s.metrics.queueLength.WithLabelValues(s.alertmanagerURL).Set(float64(len(s.queue)))

	return alerts
}

func (s *sendLoop) sendOneBatch() {
	alerts := s.nextBatch()

	if !s.sendAll(alerts) {
		s.metrics.dropped.WithLabelValues(s.alertmanagerURL).Add(float64(len(alerts)))
	}
}

// loop continuously consumes the notifications queue and sends alerts to
// the Alertmanager.
func (s *sendLoop) loop() {
	level.Debug(s.logger).Log("msg", "Starting send loop")
	for {
		// If we've been asked to stop, that takes priority over sending any further notifications.
		select {
		case <-s.stopped:
			return
		default:
			select {
			case <-s.stopped:
				return
			case <-s.hasWork:
				s.sendOneBatch()

				// If the queue still has items left, kick off the next iteration.
				if s.queueLen() > 0 {
					s.notifyWork()
				}
			}
		}
	}
}

// sendAll sends the alerts to the Alertmanager.
// It returns true if the alerts could be sent successfully.
func (s *sendLoop) sendAll(alerts []*Alert) bool {
	if len(alerts) == 0 {
		return true
	}

	begin := time.Now()

	var payload []byte
	var err error
	switch s.cfg.APIVersion {
	case config.AlertmanagerAPIVersionV1:
		payload, err = json.Marshal(alerts)
		if err != nil {
			level.Error(s.logger).Log("msg", "Encoding alerts for Alertmanager API v1 failed", "err", err)
			return false
		}

	case config.AlertmanagerAPIVersionV2:
		openAPIAlerts := alertsToOpenAPIAlerts(alerts)
		payload, err = json.Marshal(openAPIAlerts)
		if err != nil {
			level.Error(s.logger).Log("msg", "Encoding alerts for Alertmanager API v2 failed", "err", err)
			return false
		}

	default:
		level.Error(s.logger).Log(
			"msg", fmt.Sprintf("Invalid Alertmanager API version '%v', expected one of '%v'", s.cfg.APIVersion, config.SupportedAlertmanagerAPIVersions),
		)
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(s.cfg.Timeout))
	defer cancel()

	if err := s.sendOne(ctx, s.client, s.alertmanagerURL, payload); err != nil {
		level.Error(s.logger).Log("msg", "Error sending alerts", "count", len(alerts), "err", err)
		s.metrics.errors.WithLabelValues(s.alertmanagerURL).Add(float64(len(alerts)))
		return false
	}
	s.metrics.latency.WithLabelValues(s.alertmanagerURL).Observe(time.Since(begin).Seconds())
	s.metrics.sent.WithLabelValues(s.alertmanagerURL).Add(float64(len(alerts)))
	// PP_CHANGES.md: add successfully send metric
	s.metrics.successfullySent.WithLabelValues(s.alertmanagerURL).Add(float64(len(alerts)))

	return true
}

func (s *sendLoop) sendOne(ctx context.Context, c *http.Client, url string, b []byte) error {
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", contentTypeJSON)
	resp, err := s.opts.Do(ctx, c, req)
	if err != nil {
		return err
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()

	// Any HTTP status 2xx is OK.
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("bad response status %s", resp.Status)
	}

	return nil
}
