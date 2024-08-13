package handler

import (
	"net/http"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/odarix/odarix-core-go/util"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"
	"golang.org/x/net/websocket"

	"github.com/prometheus/prometheus/op-pkg/handler/adapter"
	"github.com/prometheus/prometheus/op-pkg/handler/decoder/opcore"
	"github.com/prometheus/prometheus/op-pkg/handler/middleware"
	"github.com/prometheus/prometheus/op-pkg/handler/model"
	"github.com/prometheus/prometheus/op-pkg/handler/processor"
	"github.com/prometheus/prometheus/op-pkg/handler/storage/block"
)

// OpHandler service for remote write via opprotocol.
type OpHandler struct {
	receiver Receiver
	logger   log.Logger
	stream   StreamProcessor
	refill   RefillProcessor
	stop     *atomic.Bool
	// stats
	activeConnections *prometheus.GaugeVec
}

// NewOpHandler init new OpHandler.
func NewOpHandler(
	receiver Receiver,
	logger log.Logger,
	registerer prometheus.Registerer,
) *OpHandler {
	opLocalStoragePath := "opdata/"
	opBlockStorage := block.NewStorage(opLocalStoragePath)
	factory := util.NewUnconflictRegisterer(registerer)
	h := &OpHandler{
		receiver: receiver,
		logger:   log.With(logger, "component", "op_handler"),
		stream:   processor.NewStreamProcessor(opcore.NewBuilder(opBlockStorage), receiver, registerer),
		refill:   processor.NewRefillProcessor(opcore.NewReplayDecoderBuilder(opBlockStorage), receiver, registerer),
		stop:     new(atomic.Bool),
		// stats
		activeConnections: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "remote_write_opprotocol_active_connections_count",
				Help: "Number of opprotocol active connections.",
			},
			[]string{"type"},
		),
	}

	level.Info(h.logger).Log("msg", "created")

	return h
}

// Websocket handler for websocket stream.
func (h *OpHandler) Websocket() http.HandlerFunc {
	return h.measure(
		middleware.ResolveMetadata(
			h.metadataValidator,
			websocket.Handler(h.websocketHandler).ServeHTTP,
		),
		"stream",
	)
}

// Refill handler for refill.
func (h *OpHandler) Refill() http.HandlerFunc {
	return h.measure(
		middleware.ResolveMetadata(
			h.metadataValidator,
			h.refillHandler(),
		),
		"refill",
	)
}

// measure middleware for metrics.
func (h *OpHandler) measure(next http.Handler, typeHandler string) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if h.stop.Load() {
			rw.WriteHeader(http.StatusServiceUnavailable)
			return
		}

		h.activeConnections.With(prometheus.Labels{"type": typeHandler}).Inc()
		defer h.activeConnections.With(prometheus.Labels{"type": typeHandler}).Dec()

		next.ServeHTTP(rw, r)
	}
}

// metadataValidator validate metadata.
func (h *OpHandler) metadataValidator(metadata *model.Metadata) bool {
	if ok := h.receiver.RelabelerIDIsExist(metadata.RelabelerID); !ok {
		level.Error(h.logger).Log("msg", "relabeler id not found", "relabeler_id", metadata.RelabelerID)
		return false
	}

	return true
}

// websocketHandler handler for websocket.
func (h *OpHandler) websocketHandler(wconn *websocket.Conn) {
	defer func() { _ = wconn.Close() }()
	wconn.PayloadType = websocket.BinaryFrame
	ctx := wconn.Request().Context()
	metadata := middleware.MetadataFromContext(ctx)
	if err := h.stream.Process(ctx, adapter.NewStream(wconn, metadata)); err != nil {
		level.Error(h.logger).Log("msg", "failed processing stream", "err", err)
		return
	}
}

// refillHandler handler for refill.
func (h *OpHandler) refillHandler() http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		ctx := request.Context()
		metadata := middleware.MetadataFromContext(ctx)
		if err := h.refill.Process(ctx, adapter.NewRefill(request.Body, metadata)); err != nil {
			level.Error(h.logger).Log("msg", "failed processing refill", "err", err)
			return
		}
	}
}

// Shutdown set the stop flag and reject all incoming requests.
func (h *OpHandler) Shutdown() {
	h.stop.Store(true)
	level.Info(h.logger).Log("msg", "stopped")
}
