package handler

import (
	"context"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"golang.org/x/net/websocket"

	"github.com/prometheus/prometheus/op-pkg/handler/adapter"
	"github.com/prometheus/prometheus/op-pkg/handler/middleware"
	"github.com/prometheus/prometheus/op-pkg/handler/processor"
)

type StreamProcessor interface {
	Process(ctx context.Context, stream processor.MetricStream) error
}

func Websocket(streamProcessor StreamProcessor, registerer prometheus.Registerer) http.HandlerFunc {
	return middleware.Measure(registerer, "stream")(
		middleware.ResolveMetadata(
			websocketHandler(streamProcessor).ServeHTTP,
		),
	)
}

func websocketHandler(streamProcessor StreamProcessor) websocket.Handler {
	return func(conn *websocket.Conn) {
		defer func() { _ = conn.Close() }()
		conn.PayloadType = websocket.BinaryFrame
		ctx := conn.Request().Context()
		metadata := middleware.MetadataFromContext(ctx)
		if err := streamProcessor.Process(ctx, adapter.NewStream(conn, metadata)); err != nil {
			// todo write error
			return
		}
	}
}
