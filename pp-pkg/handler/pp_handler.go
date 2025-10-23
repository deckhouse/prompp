package handler

import (
	"net/http"
	"path/filepath"
	"strconv"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"
	"golang.org/x/net/websocket"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pp-pkg/handler/adapter"
	"github.com/prometheus/prometheus/pp-pkg/handler/decoder/ppcore"
	"github.com/prometheus/prometheus/pp-pkg/handler/middleware"
	"github.com/prometheus/prometheus/pp-pkg/handler/processor"
	"github.com/prometheus/prometheus/pp-pkg/handler/storage/block"
	"github.com/prometheus/prometheus/pp/go/util"
	"github.com/prometheus/prometheus/util/pool"
)

// ppLocalStoragePath path to local wal storage.
const ppLocalStoragePath = "ppdata/"

// PPHandler service for remote write via pp-protocol.
type PPHandler struct {
	adapter     Adapter
	states      *StatesStorage
	logger      log.Logger
	stream      StreamProcessor
	refill      RefillProcessor
	remoteWrite RemoteWriteProcessor
	buffers     *pool.Pool
	stop        *atomic.Bool
	// stats
	activeConnections *prometheus.GaugeVec
}

// NewPPHandler init new PPHandler.
func NewPPHandler(
	workDir string,
	ar Adapter,
	logger log.Logger,
	registerer prometheus.Registerer,
) *PPHandler {
	buffers := pool.New(8, 1e6, 2, func(sz int) any { return make([]byte, 0, sz) })
	ppBlockStorage := block.NewStorage(filepath.Join(workDir, ppLocalStoragePath), buffers)
	states := NewStatesStorage()
	factory := util.NewUnconflictRegisterer(registerer)
	h := &PPHandler{
		adapter: ar,
		states:  states,
		logger:  log.With(logger, "component", "pp_handler"),
		stream:  processor.NewStreamProcessor(ppcore.NewBuilder(ppBlockStorage), ar, states, registerer),
		refill: processor.NewRefillProcessor(
			ppcore.NewReplayDecoderBuilder(ppBlockStorage),
			ar,
			states,
			logger,
			registerer,
		),
		remoteWrite: processor.NewRemoteWriteProcessor(ar, states, registerer),
		buffers:     buffers,
		stop:        new(atomic.Bool),
		// stats
		activeConnections: factory.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: "remote_write_pp_protocol_active_connections_count",
				Help: "Number of pp_protocol active connections.",
			},
			[]string{"type"},
		),
	}

	level.Info(h.logger).Log("msg", "created")

	return h
}

// ApplyConfig updates the configs for [StatesStorage].
func (h *PPHandler) ApplyConfig(conf *config.Config) error {
	return h.states.ApplyConfig(conf)
}

// Websocket handler for websocket stream.
func (h *PPHandler) Websocket(middlewares ...middleware.Middleware) http.HandlerFunc {
	hf := h.metadataValidator(websocket.Handler(h.websocketHandler).ServeHTTP)
	for _, mw := range middlewares {
		hf = mw(hf)
	}
	return h.measure(hf, "stream")
}

// Refill handler for refill.
func (h *PPHandler) Refill(middlewares ...middleware.Middleware) http.HandlerFunc {
	hf := h.metadataValidator(h.refillHandler())
	for _, mw := range middlewares {
		hf = mw(hf)
	}
	return h.measure(hf, "refill")
}

// RemoteWrite handler for RemoteWrite.
func (h *PPHandler) RemoteWrite(middlewares ...middleware.Middleware) http.HandlerFunc {
	hf := h.metadataValidator(h.remoteWriteHandler())
	for _, mw := range middlewares {
		hf = mw(hf)
	}
	return h.measure(hf, "remote_write")
}

// measure middleware for metrics.
func (h *PPHandler) measure(next http.Handler, typeHandler string) http.HandlerFunc {
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
func (h *PPHandler) metadataValidator(next http.HandlerFunc) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		metadata := middleware.MetadataFromContext(r.Context())
		if _, ok := h.states.GetStateByID(metadata.RelabelerID); !ok {
			level.Error(h.logger).Log("msg", "relabeler id not found", "relabeler_id", metadata.RelabelerID)
			rw.WriteHeader(http.StatusPreconditionFailed)
			return
		}

		next.ServeHTTP(rw, r)
	}
}

// websocketHandler handler for websocket.
func (h *PPHandler) websocketHandler(wconn *websocket.Conn) {
	defer func() { _ = wconn.Close() }()
	wconn.PayloadType = websocket.BinaryFrame
	ctx := wconn.Request().Context()
	metadata := middleware.MetadataFromContext(ctx)
	if err := h.stream.Process(ctx, adapter.NewStream(wconn, h.buffers, &metadata)); err != nil {
		level.Error(h.logger).Log("msg", "failed processing stream", "err", err)
		return
	}
}

// refillHandler handler for refill.
func (h *PPHandler) refillHandler() http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		ctx := request.Context()
		metadata := middleware.MetadataFromContext(ctx)
		if err := h.refill.Process(ctx, adapter.NewRefill(request.Body, writer, h.buffers, &metadata)); err != nil {
			level.Error(h.logger).Log("msg", "failed processing refill", "err", err)
			return
		}
	}
}

// remoteWriteHandler handler for RemoteWrite.
func (h *PPHandler) remoteWriteHandler() http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		ctx := request.Context()
		metadata := middleware.MetadataFromContext(ctx)
		contentLength, _ := strconv.Atoi(request.Header.Get("content-length"))
		if err := h.remoteWrite.Process(
			ctx,
			adapter.NewRemoteWrite(
				request.Body,
				writer,
				&metadata,
				h.buffers,
				contentLength,
			),
		); err != nil {
			level.Error(h.logger).Log("msg", "failed processing remote_write", "err", err)
			return
		}
	}
}

// Shutdown set the stop flag and reject all incoming requests.
func (h *PPHandler) Shutdown() {
	h.stop.Store(true)
	level.Info(h.logger).Log("msg", "stopped")
}
