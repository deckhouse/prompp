package model_test

import (
	"hash/crc32"
	"testing"
	"time"

	"github.com/go-faker/faker/v4"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
)

type SegmentSuite struct {
	suite.Suite
}

func TestSegmentSuite(t *testing.T) {
	suite.Run(t, new(SegmentSuite))
}

func (s *SegmentSuite) TestIsValid() {
	buf := []byte(faker.Paragraph())
	segment := &model.Segment{
		Timestamp: time.Now().UnixMilli(),
		ID:        42,
		Size:      uint32(len(buf)),
		CRC:       crc32.ChecksumIEEE(buf),
		Body:      buf,
	}

	s.True(segment.IsValid())
}

func (s *SegmentSuite) TestIsInValid() {
	buf := []byte(faker.Paragraph())
	segment := &model.Segment{
		Timestamp: time.Now().UnixMilli(),
		ID:        42,
		Size:      uint32(len(buf)),
		CRC:       42,
		Body:      buf,
	}

	s.False(segment.IsValid())
}

func (s *SegmentSuite) TestDestroy() {
	buf := []byte(faker.Paragraph())
	segment := &model.Segment{
		Timestamp: time.Now().UnixMilli(),
		ID:        42,
		Size:      uint32(len(buf)),
		CRC:       crc32.ChecksumIEEE(buf),
		Body:      buf,
	}

	segment.DestroyFn = func() {
		segment.Body = nil
	}

	segment.Destroy()

	s.Nil(segment.Body)
}
