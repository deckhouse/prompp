package processor_test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/pp-pkg/handler/processor"
	pp_pkg_model "github.com/prometheus/prometheus/pp-pkg/model"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

type RemoteWriteProcessorSuite struct {
	suite.Suite
}

func TestRemoteWriteProcessorSuite(t *testing.T) {
	suite.Run(t, new(RemoteWriteProcessorSuite))
}

func (s *RemoteWriteProcessorSuite) TestProcess() {
	ar := &AdapterMock{
		AppendSnappyProtobufFunc: func(context.Context, pp_pkg_model.ProtobufData, *cppbridge.StateV2, bool) error {
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

	var actualStatus model.RemoteWriteProcessingStatus

	rw := &RemoteWriteMock{
		MetadataFunc: func() model.Metadata {
			return metadata
		},

		ReadFunc: func(context.Context) (*model.RemoteWriteBuffer, error) {
			return &model.RemoteWriteBuffer{}, nil
		},

		WriteFunc: func(_ context.Context, status model.RemoteWriteProcessingStatus) error {
			actualStatus = status
			return nil
		},
	}

	rwProcessor := processor.NewRemoteWriteProcessor(ar, states, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	err := rwProcessor.Process(ctx, rw)
	s.Require().NoError(err)

	s.Equal(http.StatusOK, actualStatus.Code)
}

func (s *RemoteWriteProcessorSuite) TestProcessWithErrorRead() {
	ar := &AdapterMock{
		AppendSnappyProtobufFunc: func(context.Context, pp_pkg_model.ProtobufData, *cppbridge.StateV2, bool) error {
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

	fakeErr := errors.New("read error")
	var actualStatus model.RemoteWriteProcessingStatus

	rw := &RemoteWriteMock{
		MetadataFunc: func() model.Metadata {
			return metadata
		},

		ReadFunc: func(context.Context) (*model.RemoteWriteBuffer, error) {
			return nil, fakeErr
		},

		WriteFunc: func(_ context.Context, status model.RemoteWriteProcessingStatus) error {
			actualStatus = status
			return nil
		},
	}

	rwProcessor := processor.NewRemoteWriteProcessor(ar, states, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	err := rwProcessor.Process(ctx, rw)
	s.Require().ErrorIs(err, fakeErr)

	s.Equal(http.StatusBadRequest, actualStatus.Code)
	s.Equal(fakeErr.Error(), actualStatus.Message)
}

func (s *RemoteWriteProcessorSuite) TestProcessWithErrorGetStateByID() {
	ar := &AdapterMock{
		AppendSnappyProtobufFunc: func(context.Context, pp_pkg_model.ProtobufData, *cppbridge.StateV2, bool) error {
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
		GetStateByIDFunc: func(string) (*cppbridge.StateV2, bool) {
			return nil, false
		},
	}

	var actualStatus model.RemoteWriteProcessingStatus

	rw := &RemoteWriteMock{
		MetadataFunc: func() model.Metadata {
			return metadata
		},

		ReadFunc: func(context.Context) (*model.RemoteWriteBuffer, error) {
			return &model.RemoteWriteBuffer{}, nil
		},

		WriteFunc: func(_ context.Context, status model.RemoteWriteProcessingStatus) error {
			actualStatus = status
			return nil
		},
	}

	rwProcessor := processor.NewRemoteWriteProcessor(ar, states, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	err := rwProcessor.Process(ctx, rw)
	s.Require().ErrorIs(err, processor.ErrUnknownRelablerID)

	s.Equal(http.StatusPreconditionFailed, actualStatus.Code)
	s.Equal(processor.ErrUnknownRelablerID.Error(), actualStatus.Message)
}
