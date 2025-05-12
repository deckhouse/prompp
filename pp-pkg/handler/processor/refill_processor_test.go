package processor_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp-pkg/handler/decoder/ppcore"
	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/pp-pkg/handler/processor"
	"github.com/prometheus/prometheus/pp-pkg/handler/storage/block"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/util/pool"
)

type RefillProcessorSuite struct {
	suite.Suite
}

func TestRefillProcessorSuite(t *testing.T) {
	suite.Run(t, new(RefillProcessorSuite))
}

func (s *RefillProcessorSuite) TestProcess() {
	tmpDir, err := os.MkdirTemp("", "refill_processor-")
	s.Require().NoError(err)

	defer func() { _ = os.RemoveAll(tmpDir) }()
	buffers := pool.New(8, 100e3, 2, func(sz int) any { return make([]byte, 0, sz) })

	blockStorage := block.NewStorage(tmpDir, buffers)
	mr := &metricReceiver{appendFn: func(ctx context.Context, hashdex cppbridge.ShardedData, relabelerID string) error {
		return nil
	}}

	blockID := uuid.New()
	shardID := uint16(0)
	shardLog := uint8(0)
	segmentEncodingVersion := cppbridge.EncodersVersion()

	var expectedStatus model.RefillProcessingStatus

	gen := &segmentGenerator{segmentSize: 10}

	refill := &testRefill{
		metadata: model.Metadata{
			TenantID:               "",
			BlockID:                blockID,
			ShardID:                shardID,
			ShardsLog:              shardLog,
			SegmentEncodingVersion: segmentEncodingVersion,
		},
		readFn: func(ctx context.Context) (*model.Segment, error) {
			if len(gen.segments) == 5 {
				return nil, io.EOF
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
		writeFn: func(ctx context.Context, status model.RefillProcessingStatus) error {
			expectedStatus = status
			return nil
		},
	}

	decoderBuilder := ppcore.NewBuilder(blockStorage)
	refillProcessor := processor.NewRefillProcessor(decoderBuilder, mr, log.NewNopLogger(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	err = refillProcessor.Process(ctx, refill)
	s.Require().NoError(err)

	s.Equal(expectedStatus.Code, http.StatusOK)
}

func (s *RefillProcessorSuite) TestProcessWithError() {
	tmpDir, err := os.MkdirTemp("", "refill_processor-")
	s.Require().NoError(err)

	defer func() { _ = os.RemoveAll(tmpDir) }()
	buffers := pool.New(8, 100e3, 2, func(sz int) any { return make([]byte, 0, sz) })

	blockStorage := block.NewStorage(tmpDir, buffers)
	mr := &metricReceiver{appendFn: func(ctx context.Context, hashdex cppbridge.ShardedData, relabelerID string) error {
		return nil
	}}

	blockID := uuid.New()
	shardID := uint16(0)
	shardLog := uint8(0)
	segmentEncodingVersion := cppbridge.EncodersVersion()

	gen := &segmentGenerator{segmentSize: 10}

	fakeErr := errors.New("read error")

	refill := &testRefill{
		metadata: model.Metadata{
			TenantID:               "",
			BlockID:                blockID,
			ShardID:                shardID,
			ShardsLog:              shardLog,
			SegmentEncodingVersion: segmentEncodingVersion,
		},
		readFn: func(ctx context.Context) (*model.Segment, error) {
			if len(gen.segments) == 3 {
				return &model.Segment{}, fakeErr
			}

			segment, readErr := gen.generate()
			return &segment.encoded, readErr
		},
		writeFn: func(ctx context.Context, status model.RefillProcessingStatus) error {
			return nil
		},
	}

	decoderBuilder := ppcore.NewBuilder(blockStorage)
	refillProcessor := processor.NewRefillProcessor(decoderBuilder, mr, log.NewNopLogger(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	err = refillProcessor.Process(ctx, refill)
	s.Require().ErrorIs(err, fakeErr)
}

//
// testRefill
//

type testRefill struct {
	metadata model.Metadata
	readFn   func(ctx context.Context) (*model.Segment, error)
	writeFn  func(ctx context.Context, status model.RefillProcessingStatus) error
}

func (s *testRefill) Metadata() model.Metadata {
	return s.metadata
}

func (s *testRefill) Read(ctx context.Context) (*model.Segment, error) {
	return s.readFn(ctx)
}

func (s *testRefill) Write(ctx context.Context, status model.RefillProcessingStatus) error {
	return s.writeFn(ctx, status)
}
