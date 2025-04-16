package adapter_test

import (
	"bytes"
	"context"
	"hash/crc32"
	"testing"
	"time"

	"github.com/go-faker/faker/v4"
	"github.com/google/uuid"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp-pkg/handler/adapter"
	"github.com/prometheus/prometheus/pp-pkg/handler/model"
	"github.com/prometheus/prometheus/util/pool"
)

type StreamSuite struct {
	suite.Suite

	ctx     context.Context
	meta    model.Metadata
	buffers *pool.Pool
}

func TestStreamSuite(t *testing.T) {
	suite.Run(t, new(StreamSuite))
}

func (s *StreamSuite) SetupSuite() {
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

func (s *StreamSuite) TestSegmentEncodeDecode() {
	body := faker.Paragraph()
	expectedSegment := model.Segment{
		Timestamp: time.Now().UnixMilli(),
		ID:        42,
		Size:      uint32(len(body)),
		CRC:       crc32.ChecksumIEEE([]byte(body)),
		Body:      []byte(body),
	}

	bb := &bytes.Buffer{}

	err := adapter.EncodeToStream(bb, expectedSegment)
	s.Require().NoError(err)

	stream := adapter.NewStream(bb, s.buffers, &s.meta)
	actualSegment, err := stream.Read(s.ctx)
	s.Require().NoError(err)
	defer actualSegment.Destroy()

	s.Require().True(actualSegment.IsValid())
	s.Equal(expectedSegment.Timestamp, actualSegment.Timestamp)
	s.Equal(expectedSegment.ID, actualSegment.ID)
	s.Equal(expectedSegment.Size, actualSegment.Size)
	s.Equal(expectedSegment.CRC, actualSegment.CRC)
	s.Equal(body, string(actualSegment.Body))
}

func (s *StreamSuite) TestSegmentEncodeDecodeEmpty() {
	var body []byte
	expectedSegment := model.Segment{
		Timestamp: time.Now().UnixMilli(),
		ID:        42,
		Size:      uint32(len(body)),
		CRC:       crc32.ChecksumIEEE(body),
		Body:      body,
	}

	bb := &bytes.Buffer{}

	err := adapter.EncodeToStream(bb, expectedSegment)
	s.Require().NoError(err)

	stream := adapter.NewStream(bb, s.buffers, &s.meta)
	actualSegment, err := stream.Read(s.ctx)
	s.Require().NoError(err)
	defer actualSegment.Destroy()

	s.Require().True(actualSegment.IsValid())
	s.Equal(expectedSegment.Timestamp, actualSegment.Timestamp)
	s.Equal(expectedSegment.ID, actualSegment.ID)
	s.Equal(expectedSegment.Size, actualSegment.Size)
	s.Equal(expectedSegment.CRC, actualSegment.CRC)
	s.Equal(body, actualSegment.Body)
}

func (s *StreamSuite) TestSegmentEncodeDecodeError() {
	body := faker.Paragraph()
	expectedSegment := model.Segment{
		Timestamp: time.Now().UnixMilli(),
		ID:        42,
		Size:      uint32(len(body)),
		CRC:       crc32.ChecksumIEEE([]byte(body)) - 1,
		Body:      []byte(body),
	}

	bb := &bytes.Buffer{}

	err := adapter.EncodeToStream(bb, expectedSegment)
	s.Require().NoError(err)

	stream := adapter.NewStream(bb, s.buffers, &s.meta)
	_, err = stream.Read(s.ctx)
	s.Require().ErrorIs(err, model.ErrCorruptedSegment)
}
