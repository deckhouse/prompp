// Copyright OpCore

package web

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/jonboulle/clockwork"
	"github.com/odarix/odarix-core-go/relabeler"
	"github.com/odarix/odarix-core-go/util"
	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/atomic"
)

var successfulMessage = []byte("ok")

type Receiver interface {
	Append(ctx context.Context, data relabeler.ProtoData, relabelerID string) error
}

// RemoteWriteHandler service for remote write Prometheus.
type RemoteWriteHandler struct {
	receiver Receiver
	logger   log.Logger
	stop     *atomic.Bool
	endpoint string
	// stats
	requests *prometheus.CounterVec
}

// NewRemoteWriteHandler init new RemoteWriteHandler.
func NewRemoteWriteHandler(
	receiver Receiver,
	clock clockwork.Clock,
	logger log.Logger,
	registerer prometheus.Registerer,
	name string,
) *RemoteWriteHandler {
	factory := util.NewUnconflictRegisterer(registerer)
	rwh := &RemoteWriteHandler{
		receiver: receiver,
		stop:     new(atomic.Bool),
		logger:   log.WithPrefix(logger, "remote_write_receiver", name),
		endpoint: name,

		requests: factory.NewCounterVec(
			prometheus.CounterOpts{
				Name: "remote_request",
				Help: "Total requests completed.",
			},
			[]string{"reason", "code"},
		),
	}

	level.Info(rwh.logger).Log("msg", "init")

	return rwh
}

// Endpoint return current endpoint.
func (rwh *RemoteWriteHandler) Endpoint() string {
	return rwh.endpoint
}

// ServeHTTP http handler.
func (rwh *RemoteWriteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if rwh.stop.Load() {
		http.Error(w, "service is stopped", http.StatusBadRequest)
		return
	}

	delivered, err := rwh.decodeAndSend(r)
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
func (rwh *RemoteWriteHandler) decodeAndSend(r *http.Request) (bool, error) {
	bs := acquireBufferSnappy()

	if err := bs.decodeFrom(r.Body); err != nil {
		return false, err
	}

	if err := rwh.receiver.Append(r.Context(), bs, rwh.endpoint); err != nil {
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
