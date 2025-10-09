package processor

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/pp/go/util"
)

var (
	// AlwaysCommit commit flags.
	AlwaysCommit = true

	// ErrUnknownRelablerID error when relabler ID not found.
	ErrUnknownRelablerID = errors.New("unknown relabler id")
)

// RemoteWriteProcessor RemoteWrite processor.
type RemoteWriteProcessor struct {
	adapter Adapter
	states  StatesStorage

	responseStatusCodeCount *prometheus.CounterVec
}

// NewRemoteWriteProcessor init new [RemoteWriteProcessor].
func NewRemoteWriteProcessor(
	adapter Adapter,
	states StatesStorage,
	registerer prometheus.Registerer,
) *RemoteWriteProcessor {
	factory := util.NewUnconflictRegisterer(registerer)

	return &RemoteWriteProcessor{
		adapter: adapter,
		states:  states,
		responseStatusCodeCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_opprotocol_processor_response_status_code",
			Help: "Number of 200/400 status codes responded with.",
		}, []string{"processor_type", "status_code"}),
	}
}

// Process read remote write data and append to adapter.
func (p *RemoteWriteProcessor) Process(ctx context.Context, remoteWrite RemoteWrite) error {
	status := model.RemoteWriteProcessingStatus{Code: http.StatusOK}
	defer func() {
		p.responseStatusCodeCount.With(
			prometheus.Labels{"processor_type": "remote_write", "status_code": strconv.Itoa(status.Code)},
		).Inc()
		_ = remoteWrite.Write(ctx, status)
	}()

	state, ok := p.states.GetStateByID(remoteWrite.Metadata().RelabelerID)
	if !ok {
		status.Code = http.StatusPreconditionFailed
		status.Message = ErrUnknownRelablerID.Error()
		return ErrUnknownRelablerID
	}

	rwb, err := remoteWrite.Read(ctx)
	if err != nil {
		status.Code = http.StatusBadRequest
		status.Message = err.Error()
		return err
	}

	if err := p.adapter.AppendSnappyProtobuf(ctx, rwb, state, AlwaysCommit); err != nil {
		status.Code = http.StatusBadRequest
		status.Message = err.Error()
		return err
	}

	return nil
}
