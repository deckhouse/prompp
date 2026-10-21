package querier

import (
	"math/rand/v2"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/prometheus/pp/go/util/stagestats"
)

const (
	// QueryableAppenderSource metrics source for Appender.
	QueryableAppenderSource = "queryable_appender"
	// QueryableStorageSource metrics source for Storage.
	QueryableStorageSource = "queryable_storage"
)

// QueryStageMetricsEnabled enables caller and shard stage metrics for newly created [Metrics].
// Disabled by default; enable via PROMPP_FEATURES=enable_query_stage_metrics.
var QueryStageMetricsEnabled = false

// Metrics records durations and optional stages for [Querier].
type Metrics struct {
	LabelNamesDuration  prometheus.Histogram
	LabelValuesDuration prometheus.Histogram

	queries [operationCount]queryRecorders
}

// NewMetrics init new [Metrics].
//
//revive:disable-next-line:function-length // metrics constructor.
func NewMetrics(registerer prometheus.Registerer, source string) *Metrics {
	factory := util.NewUnconflictRegisterer(registerer)
	selectDuration := factory.NewHistogramVec(
		prometheus.HistogramOpts{
			Name: "prompp_querier_query_select_duration",
			Help: "Select query from head duration in microseconds",
			Buckets: []float64{
				50, 100, 250, 500, 750,
				1000, 2500, 5000, 7500,
				10000, 25000, 50000, 75000,
				100000, 500000,
			},
			ConstLabels: prometheus.Labels{"source": source},
		},
		[]string{"query_type"},
	)

	m := &Metrics{
		LabelNamesDuration: factory.NewHistogram(
			prometheus.HistogramOpts{
				Name: "prompp_querier_query_label_names_duration",
				Help: "Label names query from head duration in microseconds",
				Buckets: []float64{
					50, 100, 250, 500, 750,
					1000, 2500, 5000, 7500,
					10000, 25000, 50000, 75000,
					100000, 500000,
				},
				ConstLabels: prometheus.Labels{"source": source},
			},
		),
		LabelValuesDuration: factory.NewHistogram(
			prometheus.HistogramOpts{
				Name: "prompp_querier_query_label_values_duration",
				Help: "Label values query from head duration in microseconds",
				Buckets: []float64{
					50, 100, 250, 500, 750,
					1000, 2500, 5000, 7500,
					10000, 25000, 50000, 75000,
					100000, 500000,
				},
				ConstLabels: prometheus.Labels{"source": source},
			},
		),
	}

	if QueryStageMetricsEnabled {
		for operation := range operationCount {
			m.queries[operation] = newQueryRecorders(registerer, source, operation)
		}
	}

	m.queries[operationSelectInstant].duration = selectDuration.WithLabelValues("instant")
	m.queries[operationSelectRange].duration = selectDuration.WithLabelValues("range")
	m.queries[operationLabelNames].duration = m.LabelNamesDuration
	m.queries[operationLabelValues].duration = m.LabelValuesDuration

	return m
}

// startQuery starts the caller's lap timer and selects a shard counter stripe for one operation invocation.
// Both recorders use the same random hint to select their independent stripes.
// It carries the legacy histogram selected during [NewMetrics] and starts its timer even when stages are disabled.
// Nil [Metrics] returns disabled stats without reading the clock.
func (m *Metrics) startQuery(operation queryOperation) (stats queryStats) {
	if m == nil {
		return stats
	}

	recorders := m.queries[operation]
	stats.duration = recorders.duration

	if recorders.stages == nil {
		if stats.duration != nil {
			stats.durationStart = stagestats.Now()
		}

		return stats
	}

	hint := rand.Uint32() // #nosec G404 // it's only a stripe selection

	stats.shards = recorders.shards.Stripe(hint)
	stats.lap = recorders.stages.Stripe(hint).Start()

	return stats
}
