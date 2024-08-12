package processor

import (
	"context"
	"errors"
	"fmt"

	"github.com/odarix/odarix-core-go/cppbridge"
	"github.com/odarix/odarix-core-go/util"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/op-pkg/handler/model"
)

type StreamProcessor struct {
	decoderBuilder DecoderBuilder
	receiver       Receiver

	criticalErrorCount   *prometheus.CounterVec
	rejectedSegmentCount *prometheus.CounterVec
	decodedSampleCount   *prometheus.CounterVec
	decodedSeriesCount   *prometheus.CounterVec
	writtenSeriesCount   *prometheus.CounterVec
	writtenSampleCount   *prometheus.CounterVec
}

func NewStreamProcessor(
	decoderBuilder DecoderBuilder,
	receiver Receiver,
	registerer prometheus.Registerer,
) *StreamProcessor {
	factory := util.NewUnconflictRegisterer(registerer)

	return &StreamProcessor{
		decoderBuilder: decoderBuilder,
		receiver:       receiver,
		criticalErrorCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_processor_critical_error_count",
			Help: "Total number of critical errors occurred during serving metric stream.",
		}, []string{"error", "processor_type"}),
		rejectedSegmentCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_processor_rejected_segment_count",
			Help: "Number of rejected segments",
		}, []string{"processor_type"}),
		decodedSeriesCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_processor_decoded_series_count",
			Help: "Number of series decoded.",
		}, []string{"processor_type"}),
		decodedSampleCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_processor_decoded_samples_count",
			Help: "Number of samples decoded.",
		}, []string{"processor_type"}),
		writtenSeriesCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_processor_written_series_count",
			Help: "Number of series decoded and written to prometheus",
		}, []string{"processor_type"}),
		writtenSampleCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_processor_written_samples_count",
			Help: "Number of samples decoded and written to prometheus",
		}, []string{"processor_type"}),
	}
}

func (p *StreamProcessor) Process(ctx context.Context, stream MetricStream) error {
	decoder := p.decoderBuilder.Build(stream.Metadata())
	defer func() { _ = decoder.Close() }()

	var err error
	var encodedSegment model.Segment
	var decodedSegment cppbridge.HashdexContent

	for {
		encodedSegment, err = stream.Read(ctx)
		if err != nil {
			p.criticalErrorCount.With(prometheus.Labels{"error": err.Error(), "processor_type": "stream"}).Inc()
			return fmt.Errorf("failed to read from stream: %w", err)
		}

		if !encodedSegment.IsValid() {
			err = errors.New("corrupted segment")
			p.criticalErrorCount.With(prometheus.Labels{"error": err.Error(), "processor_type": "stream"}).Inc()
			return err
		}

		if len(encodedSegment.Body) == 0 {
			return decoder.Discard()
		}

		decodedSegment, err = decoder.DecodeToHashdex(ctx, encodedSegment)
		if err != nil {
			p.criticalErrorCount.With(prometheus.Labels{"error": err.Error(), "processor_type": "stream"}).Inc()
			return fmt.Errorf("failed to decoded segment: %w", err)
		}

		p.decodedSeriesCount.With(prometheus.Labels{"processor_type": "stream"}).Add(float64(decodedSegment.Series()))
		p.decodedSampleCount.With(prometheus.Labels{"processor_type": "stream"}).Add(float64(decodedSegment.Samples()))

		processingStatus := model.SegmentProcessingStatus{
			SegmentID: decodedSegment.SegmentID(),
			Code:      model.ProcessingStatusOk,
			Message:   "ok",
			Timestamp: decodedSegment.CreatedAt(),
		}

		if err = p.receiver.AppendHashdex(
			ctx,
			decodedSegment.ShardedData(),
			// TODO make config for incoming data
			config.TransparentRelabeler,
		); err != nil {
			processingStatus.Code = model.ProcessingStatusRejected
			processingStatus.Message = err.Error()
			p.rejectedSegmentCount.With(prometheus.Labels{"processor_type": "stream"}).Inc()
		} else {
			p.writtenSeriesCount.With(
				prometheus.Labels{"processor_type": "stream"},
			).Add(float64(decodedSegment.Series()))
			p.writtenSampleCount.With(
				prometheus.Labels{"processor_type": "stream"},
			).Add(float64(decodedSegment.Samples()))
		}

		if writeErr := stream.Write(ctx, processingStatus); err != nil {
			return writeErr
		}
	}
}
