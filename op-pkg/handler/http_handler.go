// Copyright OpCore

package handler

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/odarix/odarix-core-go/relabeler"
	"github.com/odarix/odarix-core-go/util"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/route"
	"go.uber.org/atomic"
)

var successfulMessage = []byte("ok")

// RemoteWriteHandler service for remote write Prometheus.
type RemoteWriteHandler struct {
	receiver Receiver
	logger   log.Logger
	stop     *atomic.Bool
	// stats
	requests *prometheus.CounterVec
}

// NewRemoteWriteHandler init new RemoteWriteHandler.
func NewRemoteWriteHandler(
	receiver Receiver,
	logger log.Logger,
	registerer prometheus.Registerer,
) *RemoteWriteHandler {
	factory := util.NewUnconflictRegisterer(registerer)
	rwh := &RemoteWriteHandler{
		receiver: receiver,
		stop:     new(atomic.Bool),
		logger:   log.With(logger, "component", "remote_write_handler"),
		// stats
		requests: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "remote_write_request",
				Help: "Total requests completed.",
			},
			[]string{"reason", "code"},
		),
	}

	level.Info(rwh.logger).Log("msg", "created")

	return rwh
}

// ServeHTTP http handler.
func (rwh *RemoteWriteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if rwh.stop.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, "Service Unavailable")
		return
	}

	ctx := r.Context()
	relabelerID := route.Param(ctx, "relabeler_id")
	if ok := rwh.receiver.RelabelerIDIsExist(relabelerID); !ok {
		level.Error(rwh.logger).Log("msg", "relabeler id not found", "relabeler_id", relabelerID)
		http.NotFound(w, r)
		return
	}

	delivered, err := rwh.decodeAndSend(r, relabelerID)
	if err != nil {
		if !errors.Is(err, relabeler.ErrShutdown) {
			level.Error(rwh.logger).Log("msg", "failed decode request", "err", err)
		}
		rwh.requests.With(prometheus.Labels{"reason": "error", "code": "400"}).Inc()
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !delivered {
		rwh.requests.With(prometheus.Labels{"reason": "rejected", "code": "400"}).Inc()
		http.Error(w, "rejected", http.StatusBadRequest)
		return
	}

	rwh.writeSuccessful(w)
}

// Shutdown set the stop flag and reject all incoming requests.
func (rwh *RemoteWriteHandler) Shutdown() {
	rwh.stop.Store(true)
	level.Info(rwh.logger).Log("msg", "stopped")
}

// decodeAndSend decode snappy request and send to server.
func (rwh *RemoteWriteHandler) decodeAndSend(r *http.Request, relabelerID string) (bool, error) {
	bs := acquireBufferSnappy()

	if err := bs.decodeFrom(r.Body); err != nil {
		bs.Destroy()
		return false, err
	}

	if err := rwh.receiver.AppendProtobuf(r.Context(), bs, relabelerID); err != nil {
		return false, err
	}

	return true, nil
}

// writeSuccessful send Successful message.
func (rwh *RemoteWriteHandler) writeSuccessful(w http.ResponseWriter) {
	rwh.requests.With(prometheus.Labels{"reason": "successful", "code": "200"}).Inc()
	_, err := w.Write(successfulMessage)
	if err != nil {
		level.Error(rwh.logger).Log("msg", "failed write respose", "err", err)
		return
	}
}
