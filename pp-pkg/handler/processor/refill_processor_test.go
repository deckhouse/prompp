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
	ar := &AdapterMock{
		AppendHashdexFunc: func(context.Context, cppbridge.ShardedData, *cppbridge.StateV2, bool) error {
			return nil
		},
		MergeOutOfOrderChunksFunc: func() {},
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

	var expectedStatus model.RefillProcessingStatus

	gen := &segmentGenerator{segmentSize: 10}

	refill := &RefillMock{
		MetadataFunc: func() model.Metadata {
			return metadata
		},
		ReadFunc: func(context.Context) (*model.Segment, error) {
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
		WriteFunc: func(_ context.Context, status model.RefillProcessingStatus) error {
			expectedStatus = status
			return nil
		},
	}

	decoderBuilder := ppcore.NewBuilder(blockStorage)
	refillProcessor := processor.NewRefillProcessor(decoderBuilder, ar, states, log.NewNopLogger(), nil)

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
	ar := &AdapterMock{
		AppendHashdexFunc: func(context.Context, cppbridge.ShardedData, *cppbridge.StateV2, bool) error {
			return nil
		},
		MergeOutOfOrderChunksFunc: func() {},
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

	gen := &segmentGenerator{segmentSize: 10}

	fakeErr := errors.New("read error")

	refill := &RefillMock{
		MetadataFunc: func() model.Metadata {
			return metadata
		},
		ReadFunc: func(context.Context) (*model.Segment, error) {
			if len(gen.segments) == 3 {
				return &model.Segment{}, fakeErr
			}

			segment, readErr := gen.generate()
			return &segment.encoded, readErr
		},
		WriteFunc: func(context.Context, model.RefillProcessingStatus) error {
			return nil
		},
	}

	decoderBuilder := ppcore.NewBuilder(blockStorage)
	refillProcessor := processor.NewRefillProcessor(decoderBuilder, ar, states, log.NewNopLogger(), nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	err = refillProcessor.Process(ctx, refill)
	s.Require().ErrorIs(err, fakeErr)
}
