package processor_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp-pkg/handler/decoder/ppcore"
	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/pp-pkg/handler/processor"
	"github.com/prometheus/prometheus/pp-pkg/handler/storage/block"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	coremodel "github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/util/pool"
)

// TODO //go:generate -command moq go run github.com/matryer/moq -out processor_moq_test.go -pkg processor_test -rm . Adapter StatesStorage RemoteWrite MetricStream Refill

type segmentContainer struct {
	timeSeries []coremodel.TimeSeries
	encoded    model.Segment
}

type segmentGenerator struct {
	segmentSize int
	encoder     *cppbridge.WALEncoder
	segments    []segmentContainer
}

func (g *segmentGenerator) generate() (segmentContainer, error) {
	batch := make([]coremodel.TimeSeries, 0, g.segmentSize)
	ts := time.Now().UnixMilli()
	for i := 0; i < g.segmentSize; i++ {
		batch = append(batch, coremodel.TimeSeries{
			LabelSet: coremodel.NewLabelSetBuilder().
				Set("__name__", fmt.Sprintf("metric_%d", i)).
				Build(),
			Timestamp: uint64(ts),
			Value:     float64(i),
		})
	}

	hdx, err := cppbridge.NewWALGoModelHashdex(cppbridge.DefaultWALHashdexLimits(), batch)
	if err != nil {
		return segmentContainer{}, err
	}

	if g.encoder == nil {
		g.encoder = cppbridge.NewWALEncoder(0, 0)
	}

	segmentKey, encodedSegmentData, err := g.encoder.Encode(context.Background(), hdx)
	if err != nil {
		return segmentContainer{}, err
	}

	buf := bytes.NewBuffer(nil)
	bytesWritten, err := encodedSegmentData.WriteTo(buf)
	if err != nil {
		return segmentContainer{}, err
	}

	segment := model.Segment{
		ID:   segmentKey.Segment,
		Size: uint32(bytesWritten),
		CRC:  crc32.ChecksumIEEE(buf.Bytes()),
		Body: buf.Bytes(),
	}

	container := segmentContainer{timeSeries: batch, encoded: segment}
	g.segments = append(g.segments, container)

	return container, nil
}

func TestStreamProcessor_Process(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "stream_processor-")
	require.NoError(t, err)
	defer func() { _ = os.RemoveAll(tmpDir) }()
	buffers := pool.New(8, 100e3, 2, func(sz int) interface{} { return make([]byte, 0, sz) })

	blockStorage := block.NewStorage(tmpDir, buffers)

	ar := &AdapterMock{
		AppendHashdexFunc: func(context.Context, cppbridge.ShardedData, *cppbridge.StateV2, bool) error {
			return nil
		},
	}

	metadata := model.Metadata{
		TenantID:               "",
		BlockID:                uuid.New(),
		ShardID:                0,
		ShardsLog:              0,
		SegmentEncodingVersion: 3,
		RelabelerID:            uuid.New().String(),
	}

	states := &StatesStorageMock{
		GetStateByIDFunc: func(stateID string) (*cppbridge.StateV2, bool) {
			if metadata.RelabelerID != stateID {
				return nil, false
			}

			return nil, true
		},
	}

	decoderBuilder := ppcore.NewBuilder(blockStorage)

	streamProcessor := processor.NewStreamProcessor(decoderBuilder, ar, states, nil)
	resolvec := make(chan struct{}, 1)

	gen := &segmentGenerator{segmentSize: 10}

	iteration := 0
	fakeErr := errors.New("read error")

	stream := &MetricStreamMock{
		MetadataFunc: func() model.Metadata {
			return metadata
		},
		ReadFunc: func(context.Context) (*model.Segment, error) {
			resolvec <- struct{}{}

			if len(gen.segments) == 3 && iteration == 0 {
				iteration++
				<-resolvec
				return &model.Segment{}, fakeErr
			}

			segment, readErr := gen.generate()
			return &segment.encoded, readErr
		},
		WriteFunc: func(context.Context, model.StreamSegmentProcessingStatus) error {
			<-resolvec
			return nil
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	err = streamProcessor.Process(ctx, stream)
	require.ErrorIs(t, err, fakeErr)

	stream = &MetricStreamMock{
		MetadataFunc: func() model.Metadata {
			return metadata
		},
		ReadFunc: func(context.Context) (*model.Segment, error) {
			resolvec <- struct{}{}

			if len(gen.segments) == 5 && iteration == 1 {
				iteration++
				<-resolvec
				return &model.Segment{}, fakeErr
			}

			segment, readErr := gen.generate()
			return &segment.encoded, readErr
		},
		WriteFunc: func(context.Context, model.StreamSegmentProcessingStatus) error {
			<-resolvec
			return nil
		},
	}

	err = streamProcessor.Process(ctx, stream)
	require.ErrorIs(t, err, fakeErr)

	stream = &MetricStreamMock{
		MetadataFunc: func() model.Metadata {
			return metadata
		},
		ReadFunc: func(context.Context) (*model.Segment, error) {
			resolvec <- struct{}{}

			if len(gen.segments) == 5 && iteration == 2 {
				iteration++
				return &gen.segments[len(gen.segments)-1].encoded, nil
			}

			if len(gen.segments) == 10 {
				return &model.Segment{
					ID:   9,
					Size: 0,
					CRC:  0,
					Body: nil,
				}, nil
			}

			segment, readErr := gen.generate()
			return &segment.encoded, readErr
		},
		WriteFunc: func(context.Context, model.StreamSegmentProcessingStatus) error {
			<-resolvec
			return nil
		},
	}

	err = streamProcessor.Process(ctx, stream)
	require.NoError(t, err)
}
