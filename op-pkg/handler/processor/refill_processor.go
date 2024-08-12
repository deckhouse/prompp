package processor

import (
	"context"

	"github.com/odarix/odarix-core-go/util"
	"github.com/prometheus/client_golang/prometheus"
)

type RefillProcessor struct {
	decoderBuilder DecoderBuilder
	receiver       Receiver

	criticalErrorCount *prometheus.CounterVec
	decodedSampleCount *prometheus.CounterVec
	decodedSeriesCount *prometheus.CounterVec
	writtenSeriesCount *prometheus.CounterVec
	writtenSampleCount *prometheus.CounterVec
}

func NewRefillProcessor(
	decoderBuilder DecoderBuilder,
	receiver Receiver,
	registerer prometheus.Registerer,
) *RefillProcessor {
	factory := util.NewUnconflictRegisterer(registerer)
	return &RefillProcessor{
		decoderBuilder: decoderBuilder,
		receiver:       receiver,
		criticalErrorCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_receiver_critical_error_count",
			Help: "Total number of critical errors occurred during serving metric stream.",
		}, []string{"error", "receiver_type"}),
		decodedSeriesCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_receiver_decoded_series_count",
			Help: "Number of series decoded.",
		}, []string{"receiver_type"}),
		decodedSampleCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_receiver_decoded_samples_count",
			Help: "Number of samples decoded.",
		}, []string{"receiver_type"}),
		writtenSeriesCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_receiver_written_series_count",
			Help: "Number of series decoded and written to prometheus",
		}, []string{"receiver_type"}),
		writtenSampleCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_receiver_written_samples_count",
			Help: "Number of samples decoded and written to prometheus",
		}, []string{"receiver_type"}),
	}
}

func (p *RefillProcessor) Process(_ context.Context, _ Refill) error {
	return nil
}
