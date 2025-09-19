package processor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/pp/go/util"
)

type RefillProcessor struct {
	decoderBuilder DecoderBuilder
	adapter        Adapter
	states         StatesStorage
	logger         log.Logger

	criticalErrorCount      *prometheus.CounterVec
	decodedSampleCount      *prometheus.CounterVec
	decodedSeriesCount      *prometheus.CounterVec
	writtenSeriesCount      *prometheus.CounterVec
	writtenSampleCount      *prometheus.CounterVec
	responseStatusCodeCount *prometheus.CounterVec
}

func NewRefillProcessor(
	decoderBuilder DecoderBuilder,
	adapter Adapter,
	states StatesStorage,
	logger log.Logger,
	registerer prometheus.Registerer,
) *RefillProcessor {
	factory := util.NewUnconflictRegisterer(registerer)
	return &RefillProcessor{
		decoderBuilder: decoderBuilder,
		adapter:        adapter,
		states:         states,
		logger:         log.With(logger, "component", "refill_processor"),
		criticalErrorCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_opprotocol_processor_critical_error_count",
			Help: "Total number of critical errors occurred during serving metric stream.",
		}, []string{"error", "processor_type"}),
		decodedSeriesCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_opprotocol_processor_decoded_series_count",
			Help: "Number of series decoded.",
		}, []string{"processor_type"}),
		decodedSampleCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_opprotocol_processor_decoded_samples_count",
			Help: "Number of samples decoded.",
		}, []string{"processor_type"}),
		writtenSeriesCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_opprotocol_processor_written_series_count",
			Help: "Number of series decoded and written to prometheus",
		}, []string{"processor_type"}),
		writtenSampleCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_opprotocol_processor_written_samples_count",
			Help: "Number of samples decoded and written to prometheus",
		}, []string{"processor_type"}),
		responseStatusCodeCount: factory.NewCounterVec(prometheus.CounterOpts{
			Name: "remote_write_opprotocol_processor_response_status_code",
			Help: "Number of 200/400 status codes responded with.",
		}, []string{"processor_type", "status_code"}),
	}
}

func (p *RefillProcessor) Process(ctx context.Context, refill Refill) error {
	meta := refill.Metadata()

	state, ok := p.states.GetStateByID(meta.RelabelerID)
	if !ok {
		p.criticalErrorCount.With(prometheus.Labels{
			"error":          ErrUnknownRelablerID.Error(),
			"processor_type": "stream",
		}).Inc()
		return ErrUnknownRelablerID
	}

	decoder := p.decoderBuilder.Build(meta)
	defer func() { _ = decoder.Close() }()

	var (
		decodedSamples uint32
		decodedSeries  uint32
	)

	for {
		decodedSegment, err := refill.Read(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				if disErr := decoder.Discard(); disErr != nil {
					p.criticalErrorCount.With(
						prometheus.Labels{"error": disErr.Error(), "processor_type": "refill"},
					).Inc()
				}

				p.writtenSeriesCount.With(prometheus.Labels{"processor_type": "refill"}).Add(float64(decodedSeries))
				p.writtenSampleCount.With(prometheus.Labels{"processor_type": "refill"}).Add(float64(decodedSamples))

				p.responseStatusCodeCount.With(
					prometheus.Labels{"processor_type": "refill", "status_code": "200"},
				).Inc()

				p.adapter.MergeOutOfOrderChunks(ctx)

				return refill.Write(ctx, model.RefillProcessingStatus{Code: http.StatusOK})
			}

			p.criticalErrorCount.With(prometheus.Labels{"error": err.Error(), "processor_type": "refill"}).Inc()
			return fmt.Errorf("failed to read segment: %w", err)
		}

		hashdexContent, err := decoder.DecodeToHashdex(ctx, decodedSegment)
		decodedSegment.Destroy()
		if err != nil {
			p.criticalErrorCount.With(prometheus.Labels{"error": err.Error(), "processor_type": "refill"}).Inc()
			return fmt.Errorf("failed to decode segment: %w", err)
		}

		decodedSeries += hashdexContent.Series()
		p.decodedSeriesCount.With(prometheus.Labels{"processor_type": "refill"}).Add(float64(hashdexContent.Series()))
		decodedSamples += hashdexContent.Samples()
		p.decodedSampleCount.With(prometheus.Labels{"processor_type": "refill"}).Add(float64(hashdexContent.Samples()))

		if err = p.adapter.AppendHashdex(ctx, hashdexContent.ShardedData(), state, true); err != nil {
			p.criticalErrorCount.With(prometheus.Labels{"error": err.Error(), "processor_type": "refill"}).Inc()
			return fmt.Errorf("failed to append decoded segment: %w", err)
		}
	}
}
