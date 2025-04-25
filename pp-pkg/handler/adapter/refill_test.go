package adapter_test

import (
	"bytes"
	"context"
	"hash/crc32"
	"net/http"
	"testing"

	"github.com/go-faker/faker/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp-pkg/handler/adapter"
	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/util/pool"
)

type RefillSuite struct {
	suite.Suite

	ctx     context.Context
	meta    model.Metadata
	buffers *pool.Pool
}

func TestRefillSuite(t *testing.T) {
	suite.Run(t, new(RefillSuite))
}

func (s *RefillSuite) SetupSuite() {
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

func (s *RefillSuite) TestRead() {
	body := faker.Paragraph()
	expectedSegment := model.Segment{
		ID:   42,
		Size: uint32(len(body)),
		CRC:  crc32.ChecksumIEEE([]byte(body)),
		Body: []byte(body),
	}

	bb := &bytes.Buffer{}
	rw := &responseWriter{}

	err := adapter.EncodeToRefill(bb, expectedSegment)
	s.Require().NoError(err)

	refill := adapter.NewRefill(bb, rw, s.buffers, &s.meta)
	actualSegment, err := refill.Read(s.ctx)
	s.Require().NoError(err)
	defer actualSegment.Destroy()

	s.Require().True(actualSegment.IsValid())
	s.Equal(expectedSegment.ID, actualSegment.ID)
	s.Equal(expectedSegment.Size, actualSegment.Size)
	s.Equal(expectedSegment.CRC, actualSegment.CRC)
	s.Equal(body, string(actualSegment.Body))

	s.Equal(s.meta, refill.Metadata())
}

func (s *RefillSuite) TestReadEmpty() {
	var body []byte
	expectedSegment := model.Segment{
		ID:   42,
		Size: uint32(len(body)),
		CRC:  crc32.ChecksumIEEE(body),
		Body: body,
	}

	bb := &bytes.Buffer{}
	rw := &responseWriter{}

	err := adapter.EncodeToRefill(bb, expectedSegment)
	s.Require().NoError(err)

	refill := adapter.NewRefill(bb, rw, s.buffers, &s.meta)
	actualSegment, err := refill.Read(s.ctx)
	s.Require().NoError(err)
	defer actualSegment.Destroy()

	s.Require().True(actualSegment.IsValid())
	s.Equal(expectedSegment.ID, actualSegment.ID)
	s.Equal(expectedSegment.Size, actualSegment.Size)
	s.Equal(expectedSegment.CRC, actualSegment.CRC)
	s.Equal(body, actualSegment.Body)
}

func (s *RefillSuite) TestReadError() {
	body := faker.Paragraph()
	expectedSegment := model.Segment{
		ID:   42,
		Size: uint32(len(body)),
		CRC:  crc32.ChecksumIEEE([]byte(body)) - 1,
		Body: []byte(body),
	}

	bb := &bytes.Buffer{}
	rw := &responseWriter{}

	err := adapter.EncodeToRefill(bb, expectedSegment)
	s.Require().NoError(err)

	refill := adapter.NewRefill(bb, rw, s.buffers, &s.meta)
	_, err = refill.Read(s.ctx)
	s.Require().ErrorIs(err, model.ErrCorruptedSegment)
}

func (s *RefillSuite) TestWrite() {
	msg := faker.Paragraph()
	expectedStatus := model.RefillProcessingStatus{
		Code:    http.StatusOK,
		Message: msg,
	}

	bb := &bytes.Buffer{}
	rw := &responseWriter{}

	refill := adapter.NewRefill(bb, rw, s.buffers, &s.meta)
	err := refill.Write(s.ctx, expectedStatus)
	s.Require().NoError(err)

	s.Equal(expectedStatus.Code, rw.statusCode)
	s.Equal(expectedStatus.Message, rw.buf.String())
}

// responseWriter implementation http.ResponseWriter.
type responseWriter struct {
	header     http.Header
	buf        bytes.Buffer
	statusCode int
}

// Header implementation http.ResponseWriter.
func (rw *responseWriter) Header() http.Header {
	return rw.header
}

// Write implementation http.ResponseWriter.
func (rw *responseWriter) Write(b []byte) (int, error) {
	return rw.buf.Write(b)
}

// WriteHeader implementation http.ResponseWriter.
func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
}
