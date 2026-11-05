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

	n.opts.ExternalLabels = conf.GlobalConfig.ExternalLabels
	n.opts.RelabelConfigs = conf.AlertingConfig.AlertRelabelConfigs

	amSets := make(map[string]*alertmanagerSet)
	// configToAlertmanagers maps alertmanager sets for each unique AlertmanagerConfig,
	// helping to avoid dropping known alertmanagers and re-use them without waiting for SD updates when applying the config.
	configToAlertmanagers := make(map[string]*alertmanagerSet, len(n.alertmanagers))
	for _, oldAmSet := range n.alertmanagers {
		hash, err := oldAmSet.configHash()
		if err != nil {
			return err
		}
		configToAlertmanagers[hash] = oldAmSet
	}

	for k, cfg := range conf.AlertingConfig.AlertmanagerConfigs.ToMap() {
		ams, err := newAlertmanagerSet(cfg, n.opts, n.logger, n.metrics)
		if err != nil {
			return err
		}

		hash, err := ams.configHash()
		if err != nil {
			return err
		}

		if oldAmSet, ok := configToAlertmanagers[hash]; ok {
			ams.ams = oldAmSet.ams
			ams.droppedAms = oldAmSet.droppedAms
			ams.sendLoops = oldAmSet.sendLoops
		}

		amSets[k] = ams
	}

	// Clean up the send loops of sets that don't exist in the new config.
	for k, oldAmSet := range n.alertmanagers {
		if _, exists := amSets[k]; !exists {
			oldAmSet.mtx.Lock()
			oldAmSet.cleanSendLoops(oldAmSet.ams...)
			oldAmSet.mtx.Unlock()
		}
	}

	n.alertmanagers = amSets

	return nil
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

	mtx        sync.RWMutex
	ams        []alertmanager
	droppedAms []alertmanager
	sendLoops  map[string]*sendLoop

	logger log.Logger
}

func newAlertmanagerSet(cfg *config.AlertmanagerConfig, opts *Options, logger log.Logger, metrics *alertMetrics) (*alertmanagerSet, error) {
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
	s.mtx.Lock()
	defer s.mtx.Unlock()

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
		go sendLoop.loop()
		s.sendLoops[us] = sendLoop
	}
}

// cleanSendLoops stops and cleans the send loops for each removed alertmanager.
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
			go l.loop()
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
