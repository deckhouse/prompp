package remotewriter

import (
	"testing"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/storage/ready"
)

func TestRemoteWriter_Run(_ *testing.T) {
	rw := New("", nil, clockwork.NewFakeClock(), ready.NoOpNotifier{}, prometheus.DefaultRegisterer)
	_ = rw
}
