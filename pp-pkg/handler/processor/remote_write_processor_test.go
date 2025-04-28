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
	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

type RemoteWriteProcessorSuite struct {
	suite.Suite
}

func TestRemoteWriteProcessorSuite(t *testing.T) {
	suite.Run(t, new(RemoteWriteProcessorSuite))
}

func (s *RemoteWriteProcessorSuite) TestProcess() {
	mr := &metricReceiver{appendFn: func(ctx context.Context, hashdex cppbridge.ShardedData, relabelerID string) error {
		return nil
	}}

	blockID := uuid.New()
	shardID := uint16(0)
	shardLog := uint8(0)
	segmentEncodingVersion := cppbridge.EncodersVersion()

	var expectedStatus model.RemoteWriteProcessingStatus

	rw := &testRemoteWrite{
		metadata: model.Metadata{
			TenantID:               "",
			BlockID:                blockID,
			ShardID:                shardID,
			ShardsLog:              shardLog,
			SegmentEncodingVersion: segmentEncodingVersion,
		},
		readFn: func(ctx context.Context) (*model.RemoteWriteBuffer, error) {
			return &model.RemoteWriteBuffer{}, nil
		},
		writeFn: func(ctx context.Context, status model.RemoteWriteProcessingStatus) error {
			expectedStatus = status
			return nil
		},
	}

	rwProcessor := processor.NewRemoteWriteProcessor(mr, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	err := rwProcessor.Process(ctx, rw)
	s.Require().NoError(err)

	s.Equal(http.StatusOK, expectedStatus.Code)
}

func (s *RemoteWriteProcessorSuite) TestProcessWithError() {
	mr := &metricReceiver{appendFn: func(ctx context.Context, hashdex cppbridge.ShardedData, relabelerID string) error {
		return nil
	}}

	blockID := uuid.New()
	shardID := uint16(0)
	shardLog := uint8(0)
	segmentEncodingVersion := cppbridge.EncodersVersion()

	fakeErr := errors.New("read error")

	var expectedStatus model.RemoteWriteProcessingStatus

	rw := &testRemoteWrite{
		metadata: model.Metadata{
			TenantID:               "",
			BlockID:                blockID,
			ShardID:                shardID,
			ShardsLog:              shardLog,
			SegmentEncodingVersion: segmentEncodingVersion,
		},
		readFn: func(ctx context.Context) (*model.RemoteWriteBuffer, error) {
			return nil, fakeErr
		},
		writeFn: func(ctx context.Context, status model.RemoteWriteProcessingStatus) error {
			expectedStatus = status
			return nil
		},
	}

	rwProcessor := processor.NewRemoteWriteProcessor(mr, nil)

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute*5)
	defer cancel()

	err := rwProcessor.Process(ctx, rw)
	s.Require().ErrorIs(err, fakeErr)

	s.Equal(expectedStatus.Code, http.StatusBadRequest)
	s.Equal(expectedStatus.Message, fakeErr.Error())
}

//
// testRemoteWrite
//

type testRemoteWrite struct {
	metadata model.Metadata
	readFn   func(ctx context.Context) (*model.RemoteWriteBuffer, error)
	writeFn  func(ctx context.Context, status model.RemoteWriteProcessingStatus) error
}

func (s *testRemoteWrite) Metadata() model.Metadata {
	return s.metadata
}

func (s *testRemoteWrite) Read(ctx context.Context) (*model.RemoteWriteBuffer, error) {
	return s.readFn(ctx)
}

func (s *testRemoteWrite) Write(ctx context.Context, status model.RemoteWriteProcessingStatus) error {
	return s.writeFn(ctx, status)
}
