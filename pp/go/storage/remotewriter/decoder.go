package remotewriter

import (
	"fmt"
	"io"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

//
// DecodedSegment
//

// DecodedSegment the segment decoded from the file [Wal].
type DecodedSegment struct {
	MaxTimestamp         int64
	ID                   uint32
	OutdatedSamplesCount uint32
	DroppedSamplesCount  uint32
	AddSeriesCount       uint32
	DroppedSeriesCount   uint32
	SampleCount          uint32
}

// Decoder decodes and relabeling series in segments from a file [Wal].
// Saves its state in the file for recovery upon restart.
type Decoder struct {
	relabeler     *cppbridge.StatelessRelabeler
	lss           *cppbridge.LabelSetStorage
	outputDecoder *cppbridge.WALOutputDecoder
}

// NewDecoder init new [Decoder].
func NewDecoder(
	externalLabels labels.Labels,
	relabelConfigs []*cppbridge.RelabelConfig,
	shardID uint16,
	encoderVersion uint8,
) (*Decoder, error) {
	relabeler, err := cppbridge.NewStatelessRelabeler(relabelConfigs)
	if err != nil {
		return nil, fmt.Errorf("failed to create stateless relabeler: %w", err)
	}

	lss := cppbridge.NewLssStorage()
	outputDecoder := cppbridge.NewWALOutputDecoder(
		LabelsToCppBridgeLabels(externalLabels),
		relabeler,
		lss,
		shardID,
		encoderVersion,
	)

	return &Decoder{
		relabeler:     relabeler,
		lss:           lss,
		outputDecoder: outputDecoder,
	}, nil
}

// Decode and relabeling series in segments from a file [Wal].
func (d *Decoder) Decode(
	segment []byte,
	minTimestamp int64,
	samplesStorage *cppbridge.CppSegmentSamplesStorage,
) (*DecodedSegment, error) {
	stats, err := d.outputDecoder.Decode(segment, minTimestamp, samplesStorage)
	if err != nil {
		return nil, err
	}
	return &DecodedSegment{
		MaxTimestamp:         stats.MaxTimestamp(),
		OutdatedSamplesCount: stats.OutdatedSampleCount(),
		DroppedSamplesCount:  stats.DroppedSampleCount(),
		AddSeriesCount:       stats.AddSeriesCount(),
		DroppedSeriesCount:   stats.DroppedSeriesCount(),
		SampleCount:          stats.SampleCount(),
	}, nil
}

// LoadFrom loads the state from a file.
func (d *Decoder) LoadFrom(reader io.Reader) error {
	state, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read from reader: %w", err)
	}

	return d.outputDecoder.LoadFrom(state)
}

// WriteTo writes output decoder state to io.Writer.
func (d *Decoder) WriteTo(writer io.Writer) (int64, error) {
	return d.outputDecoder.WriteTo(writer)
}

// LabelsToCppBridgeLabels converts [labels.Labels] to slice [cppbridge.Label].
func LabelsToCppBridgeLabels(lbls labels.Labels) []cppbridge.Label {
	result := make([]cppbridge.Label, 0, lbls.Len())
	lbls.Range(func(l labels.Label) {
		result = append(result, cppbridge.Label{
			Name:  l.Name,
			Value: l.Value,
		})
	})
	return result
}
