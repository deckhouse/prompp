package querier

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/util"
)

const (
	// QueryableAppenderSource metrics source for Appender.
	QueryableAppenderSource = "queryable_appender"
	// QueryableStorageSource metrics source for Storage.
	QueryableStorageSource = "queryable_storage"
)

type Metrics struct {
	LabelNamesDuration  prometheus.Histogram
	LabelValuesDuration prometheus.Histogram
	SelectDuration      *prometheus.HistogramVec
	AppendDuration      prometheus.Histogram

	WaitLockRotateDuration prometheus.Gauge
	RotationDuration       prometheus.Gauge
}

func NewMetrics(registerer prometheus.Registerer, source string) *Metrics {
	factory := util.NewUnconflictRegisterer(registerer)
	return &Metrics{
		LabelNamesDuration: factory.NewHistogram(
			prometheus.HistogramOpts{
				Name: "prompp_head_query_label_names_duration",
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
				Name: "prompp_head_query_label_values_duration",
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
		SelectDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name: "prompp_head_query_select_duration",
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
		),
		AppendDuration: factory.NewHistogram(
			prometheus.HistogramOpts{
				Name: "prompp_head_append_duration",
				Help: "Append to head duration in microseconds",
				Buckets: []float64{
					50, 100, 250, 500, 750,
					1000, 2500, 5000, 7500,
					10000, 25000, 50000, 75000,
					100000, 500000,
				},
			},
		),

		WaitLockRotateDuration: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "prompp_head_wait_lock_rotate_duration",
				Help: "The duration of the lock wait for rotation in nanoseconds",
			},
		),
		RotationDuration: factory.NewGauge(
			prometheus.GaugeOpts{
				Name: "prompp_head_rotate_duration",
				Help: "The duration of the rotate in nanoseconds",
			},
		),
	}
}
