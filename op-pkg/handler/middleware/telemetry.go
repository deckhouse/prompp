package middleware

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/odarix/odarix-core-go/util"
)

func Measure(registerer prometheus.Registerer, receiver string) func(next http.HandlerFunc) http.HandlerFunc {
	activeConnectionsCount := util.NewUnconflictRegisterer(registerer).NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "remote_write_receiver_active_connections_count",
			Help: "Number of active connections.",
		},
		[]string{"receiver"},
	)

	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(writer http.ResponseWriter, request *http.Request) {
			gauge := activeConnectionsCount.With(prometheus.Labels{"receiver": receiver})
			gauge.Inc()
			defer gauge.Dec()
			next(writer, request)
		}
	}
}
