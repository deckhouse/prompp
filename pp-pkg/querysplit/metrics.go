package querysplit

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/util"
)

// metrics are the split metrics. A nil *metrics records nothing.
type metrics struct {
	splitTotal    prometheus.Counter
	notSplitTotal prometheus.Counter
	subQueries    prometheus.Histogram
}

func newMetrics(r prometheus.Registerer) *metrics {
	if r == nil {
		return nil
	}

	factory := util.NewUnconflictRegisterer(r)
	total := factory.NewCounterVec(prometheus.CounterOpts{
		Name: "prompp_query_range_split_total",
		Help: "Number of range queries, by whether the server split them by block bounds.",
	}, []string{"result"})

	return &metrics{
		splitTotal:    total.WithLabelValues("split"),
		notSplitTotal: total.WithLabelValues("not_split"),
		subQueries: factory.NewHistogram(prometheus.HistogramOpts{
			Name:    "prompp_query_range_split_subqueries",
			Help:    "Number of sub-queries in a split range query.",
			Buckets: prometheus.ExponentialBuckets(2, 2, 8),
		}),
	}
}

func (m *metrics) split(subQueries int) {
	if m == nil {
		return
	}

	m.splitTotal.Inc()
	m.subQueries.Observe(float64(subQueries))
}

func (m *metrics) notSplit() {
	if m == nil {
		return
	}

	m.notSplitTotal.Inc()
}
