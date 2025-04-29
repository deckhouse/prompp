package adapter_test

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/go-faker/faker/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp-pkg/handler/adapter"
	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/util/pool"
)

type RemoteWriteSuite struct {
	suite.Suite

	ctx     context.Context
	meta    model.Metadata
	buffers *pool.Pool
}

func TestRemoteWriteSuite(t *testing.T) {
	suite.Run(t, new(RemoteWriteSuite))
}

func (s *RemoteWriteSuite) SetupSuite() {
	s.ctx = context.Background()
	s.meta = model.Metadata{
		TenantID:               "tenant_id",
		BlockID:                uuid.New(),
		ShardID:                0,
		ShardsLog:              1,
		SegmentEncodingVersion: 3,
		ProtocolVersion:        3,
		MediaType:              "media_type",
		ProductName:            "product_name",
		AgentHostname:          "agent_hostname",
		AgentUUID:              uuid.New(),
		RelabelerID:            "relabeler_id",
	}
	s.buffers = pool.New(8, 1e6, 2, func(sz int) any { return make([]byte, 0, sz) })
}

func (s *RemoteWriteSuite) TestRead() {
	body := faker.Paragraph()

	bb := &bufferCloser{}
	rw := &responseWriter{}

	_, err := bb.WriteString(body)
	s.Require().NoError(err)

	remoteWrite := adapter.NewRemoteWrite(bb, rw, &s.meta, s.buffers, 0)
	rwb, err := remoteWrite.Read(s.ctx)
	s.Require().NoError(err)
	defer rwb.Destroy()

	s.Equal(body, string(rwb.Bytes()))

	s.Equal(s.meta, remoteWrite.Metadata())
}

func (s *RemoteWriteSuite) TestWrite() {
	msg := faker.Paragraph()
	expectedStatus := model.RemoteWriteProcessingStatus{
		Code:    http.StatusOK,
		Message: msg,
	}

	bb := &bufferCloser{}
	rw := &responseWriter{}

	remoteWrite := adapter.NewRemoteWrite(bb, rw, &s.meta, s.buffers, 0)
	err := remoteWrite.Write(s.ctx, expectedStatus)
	s.Require().NoError(err)

	s.Equal(expectedStatus.Code, rw.statusCode)
	s.Equal(expectedStatus.Message, rw.buf.String())
}

type bufferCloser struct {
	bytes.Buffer
}

// Close implementation.
func (bc *bufferCloser) Close() error {
	return nil
}
