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
	"crypto/md5" // #nosec G501 // cryptographic strength is not required
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"sync"
	"time"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/go-openapi/strfmt"
	"github.com/prometheus/alertmanager/api/v2/models"
	"github.com/prometheus/client_golang/prometheus"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/prometheus/common/sigv4"
	"github.com/prometheus/common/version"
	"gopkg.in/yaml.v2"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/discovery/targetgroup"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/relabel"
)

const (
	contentTypeJSON = "application/json"
)

// String constants for instrumentation.
const (
	namespace         = "prometheus"
	subsystem         = "notifications"
	alertmanagerLabel = "alertmanager"
)

var userAgent = fmt.Sprintf("Prometheus/%s", version.Version)

// Alert is a generic representation of an alert in the Prometheus eco-system.
type Alert struct {
	// Label value pairs for purpose of aggregation, matching, and disposition
	// dispatching. This must minimally include an "alertname" label.
	Labels labels.Labels `json:"labels"`

	// Extra key/value information which does not define alert identity.
	Annotations labels.Labels `json:"annotations"`

	// The known time range for this alert. Both ends are optional.
	StartsAt     time.Time `json:"startsAt,omitempty"`
	EndsAt       time.Time `json:"endsAt,omitempty"`
	GeneratorURL string    `json:"generatorURL,omitempty"`
}

// Name returns the name of the alert. It is equivalent to the "alertname" label.
func (a *Alert) Name() string {
	return a.Labels.Get(labels.AlertName)
}

// Hash returns a hash over the alert. It is equivalent to the alert labels hash.
func (a *Alert) Hash() uint64 {
	return a.Labels.Hash()
}

func (a *Alert) String() string {
	s := fmt.Sprintf("%s[%s]", a.Name(), fmt.Sprintf("%016x", a.Hash())[:7])
	if a.Resolved() {
		return s + "[resolved]"
	}
	return s + "[active]"
}

// Resolved returns true iff the activity interval ended in the past.
func (a *Alert) Resolved() bool {
	return a.ResolvedAt(time.Now())
}

// ResolvedAt returns true iff the activity interval ended before
// the given timestamp.
func (a *Alert) ResolvedAt(ts time.Time) bool {
	if a.EndsAt.IsZero() {
		return false
	}
	return !a.EndsAt.After(ts)
}

// Manager is responsible for dispatching alert notifications to an
// alert manager service.
type Manager struct {
	opts *Options

	metrics *alertMetrics

	mtx sync.RWMutex

	stopOnce      *sync.Once
	stopRequested chan struct{}

	// loops counts the running send loops, the ones that are shutting down
	// after a reload included, so that Run can wait for all of them.
	loops sync.WaitGroup

	alertmanagers map[string]*alertmanagerSet
	logger        log.Logger
}

// Options are the configurable parameters of a Handler.
type Options struct {
	QueueCapacity   int
	DrainOnShutdown bool
	ExternalLabels  labels.Labels
	RelabelConfigs  []*relabel.Config
	// Used for sending HTTP requests to the Alertmanager.
	Do func(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error)

	Registerer prometheus.Registerer
}

type alertMetrics struct {
	// mtx guards loops and the aggregation of queueLength over them.
	mtx   sync.Mutex
	loops map[string][]*loopMetrics

	latency                 *prometheus.SummaryVec
	errors                  *prometheus.CounterVec
	sent                    *prometheus.CounterVec
	successfullySent        *prometheus.CounterVec // PP_CHANGES.md: add successfully send metric
	dropped                 *prometheus.CounterVec
	queueLength             *prometheus.GaugeVec
	queueCapacity           prometheus.Gauge
	alertmanagersDiscovered prometheus.GaugeFunc
}

func newAlertMetrics(r prometheus.Registerer, queueCap int, alertmanagersDiscovered func() float64) *alertMetrics {
	m := &alertMetrics{
		latency: prometheus.NewSummaryVec(prometheus.SummaryOpts{
			Namespace:  namespace,
			Subsystem:  subsystem,
			Name:       "latency_seconds",
			Help:       "Latency quantiles for sending alert notifications.",
			Objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
		},
			[]string{alertmanagerLabel},
		),
		errors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "errors_total",
			Help:      "Total number of errors sending alert notifications.",
		},
			[]string{alertmanagerLabel},
		),
		sent: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "sent_total",
			Help:      "Total number of alerts sent.",
		},
			[]string{alertmanagerLabel},
		),
		// PP_CHANGES.md: add successfully send metric
		successfullySent: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "successfully_sent_total",
			Help:      "Total number of successfully sent alerts.",
		},
			[]string{alertmanagerLabel},
		),
		dropped: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "dropped_total",
			Help:      "Total number of alerts dropped due to errors when sending to Alertmanager.",
		},
			[]string{alertmanagerLabel},
		),
		queueLength: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_length",
			Help:      "The number of alert notifications in the queue.",
		},
			[]string{alertmanagerLabel},
		),
		queueCapacity: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: namespace,
			Subsystem: subsystem,
			Name:      "queue_capacity",
			Help:      "The capacity of the alert notifications queue.",
		}),
		alertmanagersDiscovered: prometheus.NewGaugeFunc(prometheus.GaugeOpts{
			Name: "prometheus_notifications_alertmanagers_discovered",
			Help: "The number of alertmanagers discovered and active.",
		}, alertmanagersDiscovered),
	}

	m.loops = make(map[string][]*loopMetrics)
	m.queueCapacity.Set(float64(queueCap))

	if r != nil {
		r.MustRegister(
			m.latency,
			m.errors,
			m.sent,
			m.successfullySent, // PP_CHANGES.md: add successfully send metric
			m.dropped,
			m.queueLength,
			m.queueCapacity,
			m.alertmanagersDiscovered,
		)
	}

	return m
}

// loopMetrics reports the metrics of a single send loop under the URL of its
// Alertmanager. Several send loops can report under one URL, for instance when
// two Alertmanager configurations differ only in their timeout and point at the
// same endpoint. Their series are therefore shared: queue_length holds the
// total length of their queues, and the series of a URL are only removed once
// the last of those loops has stopped.
type loopMetrics struct {
	all *alertMetrics
	url string

	// queueLength of this loop alone, guarded by all.mtx.
	queueLength int
}

// acquire starts reporting for one more send loop of the given Alertmanager,
// initializing its series to 0.
func (m *alertMetrics) acquire(url string) *loopMetrics {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	// This will initialize the Counters for the AM to 0.
	m.errors.WithLabelValues(url)
	m.sent.WithLabelValues(url)
	// PP_CHANGES.md: add successfully send metric
	m.successfullySent.WithLabelValues(url)
	m.dropped.WithLabelValues(url)
	m.queueLength.WithLabelValues(url)

	lm := &loopMetrics{all: m, url: url}
	m.loops[url] = append(m.loops[url], lm)

	return lm
}

// release stops reporting for this send loop, removing the series of its
// Alertmanager once no other send loop reports under the same URL.
func (lm *loopMetrics) release() {
	m := lm.all

	m.mtx.Lock()
	defer m.mtx.Unlock()

	loops := m.loops[lm.url]
	for i, l := range loops {
		if l == lm {
			loops = append(loops[:i], loops[i+1:]...)
			break
		}
	}

	if len(loops) > 0 {
		m.loops[lm.url] = loops
		m.setQueueLength(lm.url)

		return
	}

	delete(m.loops, lm.url)
	m.latency.DeleteLabelValues(lm.url)
	m.sent.DeleteLabelValues(lm.url)
	// PP_CHANGES.md: add successfully send metric
	m.successfullySent.DeleteLabelValues(lm.url)
	m.dropped.DeleteLabelValues(lm.url)
	m.errors.DeleteLabelValues(lm.url)
	m.queueLength.DeleteLabelValues(lm.url)
}

// setQueueLength records the length of the queue of this send loop.
func (lm *loopMetrics) setQueueLength(n int) {
	m := lm.all

	m.mtx.Lock()
	defer m.mtx.Unlock()

	lm.queueLength = n
	m.setQueueLength(lm.url)
}

// setQueueLength reports the total queue length of the send loops of one URL.
// It expects the caller to hold mtx.
func (m *alertMetrics) setQueueLength(url string) {
	var total int
	for _, l := range m.loops[url] {
		total += l.queueLength
	}
	m.queueLength.WithLabelValues(url).Set(float64(total))
}

func (lm *loopMetrics) observeLatency(d time.Duration) {
	lm.all.latency.WithLabelValues(lm.url).Observe(d.Seconds())
}

func (lm *loopMetrics) addSent(n int) {
	lm.all.sent.WithLabelValues(lm.url).Add(float64(n))
}

// addSuccessfullySent counts the alerts the Alertmanager took.
// PP_CHANGES.md: add successfully send metric.
func (lm *loopMetrics) addSuccessfullySent(n int) {
	lm.all.successfullySent.WithLabelValues(lm.url).Add(float64(n))
}

func (lm *loopMetrics) addErrors(n int) {
	lm.all.errors.WithLabelValues(lm.url).Add(float64(n))
}

func (lm *loopMetrics) addDropped(n int) {
	lm.all.dropped.WithLabelValues(lm.url).Add(float64(n))
}

func do(ctx context.Context, client *http.Client, req *http.Request) (*http.Response, error) {
	if client == nil {
		client = http.DefaultClient
	}
	return client.Do(req.WithContext(ctx))
}

// NewManager is the manager constructor.
func NewManager(o *Options, logger log.Logger) *Manager {
	if o.Do == nil {
		o.Do = do
	}
	if logger == nil {
		logger = log.NewNopLogger()
	}

	n := &Manager{
		stopRequested: make(chan struct{}),
		stopOnce:      &sync.Once{},
		opts:          o,
		logger:        logger,
	}

	alertmanagersDiscoveredFunc := func() float64 { return float64(len(n.Alertmanagers())) }

	n.metrics = newAlertMetrics(
		o.Registerer,
		o.QueueCapacity,
		alertmanagersDiscoveredFunc,
	)

	return n
}

// ApplyConfig updates the status state as the new config requires.
func (n *Manager) ApplyConfig(conf *config.Config) error {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	// Run stops the send loops of the current sets once a stop is requested, so
	// a reload after that point would start loops that nothing stops, and its
	// loops.Add would race with the loops.Wait of Run.
	select {
	case <-n.stopRequested:
		return nil
	default:
	}

	n.opts.ExternalLabels = conf.GlobalConfig.ExternalLabels
	n.opts.RelabelConfigs = conf.AlertingConfig.AlertRelabelConfigs

	// Index the known alertmanager sets by the hash of their configuration, so
	// that a reload keeps the known alertmanagers and their queues instead of
	// waiting for the next service discovery update. knownAmSets serves the
	// alertmanagers of a config, which any number of sets of the new config can
	// copy, while reusable hands over the send loops of a config, and with them
	// the queued notifications, to at most one set of the new config.
	knownAmSets := make(map[string]*alertmanagerSet, len(n.alertmanagers))
	reusable := make(map[string][]*alertmanagerSet, len(n.alertmanagers))
	for _, oldAmSet := range n.alertmanagers {
		hash, err := oldAmSet.configHash()
		if err != nil {
			return err
		}
		knownAmSets[hash] = oldAmSet
		reusable[hash] = append(reusable[hash], oldAmSet)
	}

	// Phase one creates every set of the new config and may fail. It leaves the
	// sets of the previous config untouched and starts no send loop, so that a
	// rejected config leaves the notifications of the running config alone.
	type pendingAmSet struct {
		key  string
		hash string
		set  *alertmanagerSet
	}
	amCfgs := conf.AlertingConfig.AlertmanagerConfigs.ToMap()
	pending := make([]pendingAmSet, 0, len(amCfgs))
	for k, cfg := range amCfgs {
		ams, err := newAlertmanagerSet(cfg, n.opts, &n.loops, n.logger, n.metrics)
		if err != nil {
			return err
		}

		hash, err := ams.configHash()
		if err != nil {
			return err
		}

		pending = append(pending, pendingAmSet{key: k, hash: hash, set: ams})
	}

	// Phase two cannot fail: it hands the alertmanagers and the queues of the
	// previous config over to the new sets and starts the missing send loops.
	amSets := make(map[string]*alertmanagerSet, len(pending))
	for _, p := range pending {
		if oldAmSet, ok := knownAmSets[p.hash]; ok {
			p.set.inheritAlertmanagers(oldAmSet)
		}
		if oldAmSet := takeReusable(reusable, p.hash); oldAmSet != nil {
			p.set.takeSendLoops(oldAmSet)
		}
		// A set that took over no send loop, a second set of a duplicated
		// config for instance, needs its own send loops for the alertmanagers
		// it inherited.
		p.set.addSendLoops(p.set.ams)

		amSets[p.key] = p.set
	}

	// Stop the send loops of the known sets that no new set took over,
	// whatever their position in the previous config was.
	for _, oldAmSets := range reusable {
		for _, oldAmSet := range oldAmSets {
			oldAmSet.mtx.Lock()
			oldAmSet.cleanSendLoops(oldAmSet.ams...)
			oldAmSet.mtx.Unlock()
		}
	}

	n.alertmanagers = amSets

	return nil
}

// takeReusable removes and returns the set of the previous config that fits the
// given config hash best: the one carrying the most send loops, so that a
// removed duplicate of a config never takes the queues of the duplicate that
// stays. It returns nil once every set of that hash has been handed over.
func takeReusable(reusable map[string][]*alertmanagerSet, hash string) *alertmanagerSet {
	candidates := reusable[hash]
	if len(candidates) == 0 {
		return nil
	}

	best := 0
	for i, candidate := range candidates {
		if len(candidate.sendLoops) > len(candidates[best].sendLoops) {
			best = i
		}
	}

	oldAmSet := candidates[best]
	reusable[hash] = append(candidates[:best], candidates[best+1:]...)

	return oldAmSet
}

// Run processes target updates continuously, returning once Stop has been called and
// every send loop has stopped. Each send loop drains its queue first if draining is enabled.
//
// Notifications are dispatched by the send loop of each Alertmanager, so a slow or
// unreachable Alertmanager neither starves the other ones nor the target updates.
// Refer to https://github.com/prometheus/prometheus/issues/13676 and
// https://github.com/prometheus/prometheus/issues/7676 for more details.
func (n *Manager) Run(tsets <-chan map[string][]*targetgroup.Group) {
	n.targetUpdateLoop(tsets)

	n.mtx.Lock()
	for _, ams := range n.alertmanagers {
		ams.mtx.Lock()
		ams.cleanSendLoops(ams.ams...)
		ams.mtx.Unlock()
	}
	n.mtx.Unlock()

	// Stopping a send loop only signals it, so wait here for the loops to
	// finish the batch they are sending and to drain their queues. Waiting
	// outside the lock keeps a shutting down Alertmanager from blocking Send.
	n.loops.Wait()

	level.Info(n.logger).Log("msg", "Notification manager stopped")
}

// targetUpdateLoop receives updates of target groups and triggers a reload.
func (n *Manager) targetUpdateLoop(tsets <-chan map[string][]*targetgroup.Group) {
	for {
		// If we've been asked to stop, that takes priority over processing any further target group updates.
		select {
		case <-n.stopRequested:
			return
		default:
			select {
			case <-n.stopRequested:
				return
			case ts := <-tsets:
				n.reload(ts)
			}
		}
	}
}

func (n *Manager) reload(tgs map[string][]*targetgroup.Group) {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	for id, tgroup := range tgs {
		am, ok := n.alertmanagers[id]
		if !ok {
			level.Error(n.logger).Log("msg", "couldn't sync alert manager set", "err", fmt.Sprintf("invalid id:%v", id))
			continue
		}
		am.sync(tgroup)
	}
}

// Send queues the given notification requests for processing.
// Panics if called on a handler that is not running.
func (n *Manager) Send(alerts ...*Alert) {
	// If we've been asked to stop, that takes priority over accepting new alerts.
	select {
	case <-n.stopRequested:
		return
	default:
	}

	n.mtx.RLock()
	defer n.mtx.RUnlock()

	alerts = relabelAlerts(n.opts.RelabelConfigs, n.opts.ExternalLabels, alerts)
	if len(alerts) == 0 {
		return
	}

	for _, ams := range n.alertmanagers {
		ams.send(alerts...)
	}
}

func relabelAlerts(relabelConfigs []*relabel.Config, externalLabels labels.Labels, alerts []*Alert) []*Alert {
	lb := labels.NewBuilder(labels.EmptyLabels())
	var relabeledAlerts []*Alert

	for _, a := range alerts {
		lb.Reset(a.Labels)
		externalLabels.Range(func(l labels.Label) {
			if a.Labels.Get(l.Name) == "" {
				lb.Set(l.Name, l.Value)
			}
		})

		keep := relabel.ProcessBuilder(lb, relabelConfigs...)
		if !keep {
			continue
		}

		// If relabeling has altered the labels, create a new Alert to preserve immutability.
		if !labels.Equal(a.Labels, lb.Labels()) {
			a = &Alert{
				Labels:       lb.Labels(),
				Annotations:  a.Annotations,
				StartsAt:     a.StartsAt,
				EndsAt:       a.EndsAt,
				GeneratorURL: a.GeneratorURL,
			}
		}

		relabeledAlerts = append(relabeledAlerts, a)
	}
	return relabeledAlerts
}

// Alertmanagers returns a slice of Alertmanager URLs.
func (n *Manager) Alertmanagers() []*url.URL {
	n.mtx.RLock()
	amSets := n.alertmanagers
	n.mtx.RUnlock()

	var res []*url.URL

	for _, ams := range amSets {
		ams.mtx.RLock()
		for _, am := range ams.ams {
			res = append(res, am.url())
		}
		ams.mtx.RUnlock()
	}

	return res
}

// DroppedAlertmanagers returns a slice of Alertmanager URLs.
func (n *Manager) DroppedAlertmanagers() []*url.URL {
	n.mtx.RLock()
	amSets := n.alertmanagers
	n.mtx.RUnlock()

	var res []*url.URL

	for _, ams := range amSets {
		ams.mtx.RLock()
		for _, dam := range ams.droppedAms {
			res = append(res, dam.url())
		}
		ams.mtx.RUnlock()
	}

	return res
}

func alertsToOpenAPIAlerts(alerts []*Alert) models.PostableAlerts {
	openAPIAlerts := models.PostableAlerts{}
	for _, a := range alerts {
		start := strfmt.DateTime(a.StartsAt)
		end := strfmt.DateTime(a.EndsAt)
		openAPIAlerts = append(openAPIAlerts, &models.PostableAlert{
			Annotations: labelsToOpenAPILabelSet(a.Annotations),
			EndsAt:      end,
			StartsAt:    start,
			Alert: models.Alert{
				GeneratorURL: strfmt.URI(a.GeneratorURL),
				Labels:       labelsToOpenAPILabelSet(a.Labels),
			},
		})
	}

	return openAPIAlerts
}

func labelsToOpenAPILabelSet(modelLabelSet labels.Labels) models.LabelSet {
	apiLabelSet := models.LabelSet{}
	modelLabelSet.Range(func(label labels.Label) {
		apiLabelSet[label.Name] = label.Value
	})

	return apiLabelSet
}

// Stop signals the notification manager to shut down and immediately returns.
//
// Run will return once the notification manager has successfully shut down.
//
// The manager will optionally drain any queued notifications before shutting down.
//
// Stop is safe to call multiple times.
func (n *Manager) Stop() {
	level.Info(n.logger).Log("msg", "Stopping notification manager...")

	n.stopOnce.Do(func() {
		close(n.stopRequested)
	})
}

// Alertmanager holds Alertmanager endpoint information.
type alertmanager interface {
	url() *url.URL
}

type alertmanagerLabels struct{ labels.Labels }

const pathLabel = "__alerts_path__"

func (a alertmanagerLabels) url() *url.URL {
	return &url.URL{
		Scheme: a.Get(model.SchemeLabel),
		Host:   a.Get(model.AddressLabel),
		Path:   a.Get(pathLabel),
	}
}

// alertmanagerSet contains a set of Alertmanagers discovered via a group of service
// discovery definitions that have a common configuration on how alerts should be sent.
type alertmanagerSet struct {
	cfg    *config.AlertmanagerConfig
	client *http.Client
	opts   *Options

	metrics *alertMetrics
	// loops is the wait group of the manager that owns this set. It is nil for
	// a set that no manager owns, as in the tests of this package.
	loops *sync.WaitGroup

	mtx        sync.RWMutex
	ams        []alertmanager
	droppedAms []alertmanager
	sendLoops  map[string]*sendLoop

	logger log.Logger
}

func newAlertmanagerSet(cfg *config.AlertmanagerConfig, opts *Options, loops *sync.WaitGroup, logger log.Logger, metrics *alertMetrics) (*alertmanagerSet, error) {
	client, err := config_util.NewClientFromConfig(cfg.HTTPClientConfig, "alertmanager")
	if err != nil {
		return nil, err
	}
	t := client.Transport

	if cfg.SigV4Config != nil {
		t, err = sigv4.NewSigV4RoundTripper(cfg.SigV4Config, client.Transport)
		if err != nil {
			return nil, err
		}
	}

	client.Transport = t

	s := &alertmanagerSet{
		client:    client,
		cfg:       cfg,
		opts:      opts,
		loops:     loops,
		sendLoops: make(map[string]*sendLoop),
		logger:    logger,
		metrics:   metrics,
	}
	return s, nil
}

// sync extracts a deduplicated set of Alertmanager endpoints from a list
// of target groups definitions.
func (s *alertmanagerSet) sync(tgs []*targetgroup.Group) {
	allAms := []alertmanager{}
	allDroppedAms := []alertmanager{}

	for _, tg := range tgs {
		ams, droppedAms, err := AlertmanagerFromGroup(tg, s.cfg)
		if err != nil {
			level.Error(s.logger).Log("msg", "Creating discovered Alertmanagers failed", "err", err)
			continue
		}
		allAms = append(allAms, ams...)
		allDroppedAms = append(allDroppedAms, droppedAms...)
	}

	s.mtx.Lock()
	defer s.mtx.Unlock()
	previousAms := s.ams
	s.ams = []alertmanager{}
	s.droppedAms = []alertmanager{}
	s.droppedAms = append(s.droppedAms, allDroppedAms...)

	// Deduplicate Alertmanagers and add sendloops for new Alertmanagers.
	seen := map[string]struct{}{}
	for _, am := range allAms {
		us := am.url().String()
		if _, ok := seen[us]; ok {
			continue
		}

		seen[us] = struct{}{}
		s.ams = append(s.ams, am)
	}
	s.addSendLoops(s.ams)

	// Populate a list of Alertmanagers to clean up,
	// avoid cleaning up what we just added.
	for _, am := range previousAms {
		us := am.url().String()
		if _, ok := seen[us]; ok {
			continue
		}
		seen[us] = struct{}{}
		s.cleanSendLoops(am)
	}
}

func (s *alertmanagerSet) configHash() (string, error) {
	b, err := yaml.Marshal(s.cfg)
	if err != nil {
		return "", err
	}
	hash := md5.Sum(b) // #nosec G401 // cryptographic strength is not required
	return hex.EncodeToString(hash[:]), nil
}

// send relabels the alerts for this set and queues them on every send loop.
func (s *alertmanagerSet) send(alerts ...*Alert) {
	// Only reads here: queueing on a send loop takes the lock of that loop, so
	// concurrent Send calls do not serialize on the set.
	s.mtx.RLock()
	defer s.mtx.RUnlock()

	if len(s.cfg.AlertRelabelConfigs) > 0 {
		alerts = relabelAlerts(s.cfg.AlertRelabelConfigs, labels.Labels{}, alerts)
		if len(alerts) == 0 {
			return
		}
	}

	for _, sendLoop := range s.sendLoops {
		sendLoop.add(alerts...)
	}
}

// inheritAlertmanagers copies the alertmanagers discovered by a set of the
// previous config that carries the same configuration, so that a reload does
// not drop them while waiting for the next service discovery update.
func (s *alertmanagerSet) inheritAlertmanagers(old *alertmanagerSet) {
	old.mtx.RLock()
	defer old.mtx.RUnlock()

	s.ams = old.ams
	s.droppedAms = old.droppedAms
}

// takeSendLoops takes over the send loops, and with them the queued
// notifications, of a set of the previous config. The old set is left without
// send loops, so that it cannot stop what it handed over.
func (s *alertmanagerSet) takeSendLoops(old *alertmanagerSet) {
	old.mtx.Lock()
	defer old.mtx.Unlock()

	s.sendLoops = old.sendLoops
	old.sendLoops = make(map[string]*sendLoop)

	// The config hash is taken over the marshalled config, where a secret is
	// replaced by a placeholder, so two configs that differ only in their
	// credentials share a hash. Point every send loop taken over at the client
	// and the config of this set, so that a rotated credential is used.
	for _, l := range s.sendLoops {
		l.reconfigure(s.client, s.cfg)
	}
}

// start runs the given send loop and registers it with the manager, so that
// Run waits for the loop to drain its queue on shutdown.
func (s *alertmanagerSet) start(l *sendLoop) {
	if s.loops == nil {
		go l.loop()

		return
	}

	s.loops.Add(1)
	go func() {
		defer s.loops.Done()
		l.loop()
	}()
}

// addSendLoops creates and starts a send loop for newly discovered alertmanager.
// This function expects the caller to acquire needed locks.
func (s *alertmanagerSet) addSendLoops(ams []alertmanager) {
	for _, am := range ams {
		us := am.url().String()
		// Only add if sendloop doesn't already exist.
		if loop, exists := s.sendLoops[us]; exists {
			level.Debug(loop.logger).Log("msg", "Alertmanager already has send loop running, skipping")
			continue
		}
		sendLoop := newSendLoop(us, s.client, s.cfg, s.opts, log.With(s.logger, "alertmanager", us), s.metrics)
		s.start(sendLoop)
		s.sendLoops[us] = sendLoop
	}
}

// cleanSendLoops signals the send loops of the removed alertmanagers to stop
// and drops them from the set. It does not wait for them: a loop drains its
// queue on its own goroutine, so holding a lock here blocks neither Send nor a
// target update.
// This function expects the caller to acquire needed locks.
func (s *alertmanagerSet) cleanSendLoops(ams ...alertmanager) {
	for _, am := range ams {
		us := am.url().String()
		if sendLoop, ok := s.sendLoops[us]; ok {
			sendLoop.stop()
			delete(s.sendLoops, us)
		}
	}
}

// startSendLoops starts a send loop for newly discovered alertmanager.
// This function expects the caller to acquire needed locks.
// This is mainly needed for testing where the loops are added as part of the test setup.
func (s *alertmanagerSet) startSendLoops(ams []alertmanager) {
	for _, am := range ams {
		us := am.url().String()

		if l, ok := s.sendLoops[us]; ok {
			s.start(l)
			continue
		}
		panic(fmt.Sprintf("send loop not found for %s", us))
	}
}

func postPath(pre string, v config.AlertmanagerAPIVersion) string {
	alertPushEndpoint := fmt.Sprintf("/api/%v/alerts", string(v))
	return path.Join("/", pre, alertPushEndpoint)
}

// AlertmanagerFromGroup extracts a list of alertmanagers from a target group
// and an associated AlertmanagerConfig.
func AlertmanagerFromGroup(tg *targetgroup.Group, cfg *config.AlertmanagerConfig) ([]alertmanager, []alertmanager, error) {
	var res []alertmanager
	var droppedAlertManagers []alertmanager
	lb := labels.NewBuilder(labels.EmptyLabels())

	for _, tlset := range tg.Targets {
		lb.Reset(labels.EmptyLabels())

		for ln, lv := range tlset {
			lb.Set(string(ln), string(lv))
		}
		// Set configured scheme as the initial scheme label for overwrite.
		lb.Set(model.SchemeLabel, cfg.Scheme)
		lb.Set(pathLabel, postPath(cfg.PathPrefix, cfg.APIVersion))

		// Combine target labels with target group labels.
		for ln, lv := range tg.Labels {
			if _, ok := tlset[ln]; !ok {
				lb.Set(string(ln), string(lv))
			}
		}

		preRelabel := lb.Labels()
		keep := relabel.ProcessBuilder(lb, cfg.RelabelConfigs...)
		if !keep {
			droppedAlertManagers = append(droppedAlertManagers, alertmanagerLabels{preRelabel})
			continue
		}

		addr := lb.Get(model.AddressLabel)
		if err := config.CheckTargetAddress(model.LabelValue(addr)); err != nil {
			return nil, nil, err
		}

		res = append(res, alertmanagerLabels{lb.Labels()})
	}
	return res, droppedAlertManagers, nil
}
