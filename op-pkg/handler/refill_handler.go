package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/odarix/odarix-core-go/util"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/op-pkg/handler/adapter"
	"github.com/prometheus/prometheus/op-pkg/handler/middleware"
	"github.com/prometheus/prometheus/op-pkg/handler/processor"
)

type Refiller interface {
	Process(ctx context.Context, refill processor.Refill) error
}

func Refill(refiller Refiller, registerer prometheus.Registerer) http.HandlerFunc {
	return middleware.Measure(registerer, "refill")(
		middleware.ResolveMetadata(
			refill(refiller, registerer),
		),
	)
}

func refill(refiller Refiller, registerer prometheus.Registerer) http.HandlerFunc {
	refillResponseStatusCodeCount := util.NewUnconflictRegisterer(registerer).NewCounterVec(
		prometheus.CounterOpts{
			Name: "remote_write_receiver_refill_response_status_code",
			Help: "Number of 200/400 status codes responded with.",
		},
		[]string{"status_code"},
	)
	return func(writer http.ResponseWriter, request *http.Request) {
		ctx := request.Context()
		metadata := middleware.MetadataFromContext(ctx)

		statusCode := -1
		defer func() {
			refillResponseStatusCodeCount.With(prometheus.Labels{"status_code": strconv.Itoa(statusCode)}).Inc()
		}()

		err := refiller.Process(ctx, adapter.NewRefill(request.Body, metadata))
		if err != nil {
			statusCode = http.StatusBadRequest
			writer.WriteHeader(statusCode)
			_, _ = writer.Write([]byte(err.Error()))
			return
		}

		statusCode = http.StatusOK
		writer.WriteHeader(http.StatusOK)
	}
}
