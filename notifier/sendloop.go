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

	opts    *Options
	metrics *loopMetrics

	mtx     sync.RWMutex
	cfg     *config.AlertmanagerConfig
	client  *http.Client
	queue   []*Alert
	hasWork chan struct{}

	stopped  chan struct{}
	stopOnce sync.Once
	done     chan struct{}

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
	return &sendLoop{
		alertmanagerURL: alertmanagerURL,
		client:          client,
		cfg:             cfg,
		opts:            opts,
		logger:          logger,
		metrics:         metrics.acquire(alertmanagerURL),
		queue:           make([]*Alert, 0, opts.QueueCapacity),
		hasWork:         make(chan struct{}, 1),
		stopped:         make(chan struct{}),
		done:            make(chan struct{}),
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

	s.metrics.setQueueLength(len(s.queue))
	if dropped > 0 {
		s.metrics.addDropped(dropped)
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

// stop asks the send loop to shut down and returns immediately, so that a
// caller holding a lock is never blocked on the network. The loop finishes the
// batch it is sending, drains its queue if draining is enabled, then removes
// its metrics and exits. Use wait to block until it has exited.
// It is safe to call multiple times.
func (s *sendLoop) stop() {
	s.stopOnce.Do(func() {
		level.Debug(s.logger).Log("msg", "Stopping send loop")
		close(s.stopped)
	})
}

// wait blocks until the send loop has stopped sending. It only returns for a
// loop that is running, so it must follow a call to stop.
func (s *sendLoop) wait() {
	<-s.done
}

// shutdown runs on the goroutine of the loop once it has been asked to stop and
// is no longer sending a batch.
func (s *sendLoop) shutdown() {
	if s.opts.DrainOnShutdown {
		level.Info(s.logger).Log("msg", "Draining any remaining notifications...")
		s.drainQueue()
		level.Info(s.logger).Log("msg", "Remaining notifications drained")
	} else if ql := s.queueLen(); ql > 0 {
		level.Warn(s.logger).Log("msg", "Alert notification queue not drained on shutdown, dropping alerts", "count", ql)
		s.metrics.addDropped(ql)
	}

	// The series of this Alertmanager are only removed once no other send loop
	// reports under the same URL.
	s.metrics.release()
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
	s.metrics.setQueueLength(len(s.queue))

	return alerts
}

func (s *sendLoop) sendOneBatch() {
	alerts := s.nextBatch()

	if !s.sendAll(alerts) {
		s.metrics.addDropped(len(alerts))
	}
}

// reconfigure points the send loop at the client and the configuration of the
// Alertmanager set that took it over on a reload, keeping the queued alerts.
func (s *sendLoop) reconfigure(client *http.Client, cfg *config.AlertmanagerConfig) {
	s.mtx.Lock()
	defer s.mtx.Unlock()

	s.client = client
	s.cfg = cfg
}

// target returns the client and the configuration to send the next batch with.
func (s *sendLoop) target() (*http.Client, *config.AlertmanagerConfig) {
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	return s.client, s.cfg
}

// loop continuously consumes the notifications queue and sends alerts to
// the Alertmanager. It returns once the loop has been stopped and, if draining
// is enabled, its queue has been drained.
func (s *sendLoop) loop() {
	defer close(s.done)

	level.Debug(s.logger).Log("msg", "Starting send loop")
	for {
		// If we've been asked to stop, that takes priority over sending any further notifications.
		select {
		case <-s.stopped:
			s.shutdown()
			return
		default:
			select {
			case <-s.stopped:
				s.shutdown()
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

	client, cfg := s.target()

	begin := time.Now()

	var payload []byte
	var err error
	switch cfg.APIVersion {
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
			"msg", fmt.Sprintf("Invalid Alertmanager API version '%v', expected one of '%v'", cfg.APIVersion, config.SupportedAlertmanagerAPIVersions),
		)
		return false
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Timeout))
	defer cancel()

	err = s.sendOne(ctx, client, s.alertmanagerURL, payload)

	// An attempt counts towards the latency and sent_total whatever its
	// outcome, so that errors_total stays a share of the attempts.
	// PP_CHANGES.md: add successfully send metric
	s.metrics.observeLatency(time.Since(begin))
	s.metrics.addSent(len(alerts))

	if err != nil {
		level.Error(s.logger).Log("msg", "Error sending alerts", "count", len(alerts), "err", err)
		s.metrics.addErrors(len(alerts))
		return false
	}
	s.metrics.addSuccessfullySent(len(alerts))

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
