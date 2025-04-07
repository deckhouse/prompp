package querier

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/util"
)

type Metrics struct {
	LabelNamesDuration  *prometheus.HistogramVec
	LabelValuesDuration *prometheus.HistogramVec
	SelectDuration      *prometheus.HistogramVec
}

func NewMetrics(registerer prometheus.Registerer) *Metrics {
	factory := util.NewUnconflictRegisterer(registerer)
	return &Metrics{
		LabelNamesDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "prompp_head_query_label_names_duration",
				Help:    "Label names query from head duration in milliseconds",
				Buckets: []float64{1, 5, 10, 20, 50, 100},
			},
			[]string{"generation"},
		),
		LabelValuesDuration: factory.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    "prompp_head_query_label_values_duration",
				Help:    "Label values query from head duration in milliseconds",
				Buckets: []float64{1, 5, 10, 20, 50, 100},
			},
			[]string{"generation"},
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
			},
			[]string{"generation"},
		),
	}
}
