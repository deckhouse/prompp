package writer_test

import (
	"math"
	"testing"

	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/writer"
	"github.com/stretchr/testify/suite"
)

type SegmentWriteNotifierSuite struct {
	suite.Suite
}

func TestSegmentWriteNotifierSuite(t *testing.T) {
	suite.Run(t, new(SegmentWriteNotifierSuite))
}

func (s *SegmentWriteNotifierSuite) TestHappyPath() {
	actualSegmentID := uint32(math.MaxUint32)

	numberOfShards := uint16(2)
	swn := writer.NewSegmentWriteNotifier(numberOfShards, func(segmentID uint32) { actualSegmentID = segmentID })

	for id := range numberOfShards {
		swn.NotifySegmentWrite(id)
	}
	swn.NotifySegmentIsWritten()

	s.Equal(uint32(0), actualSegmentID)
}

func (s *SegmentWriteNotifierSuite) TestNotifyOnlyOneShard() {
	actualSegmentID := uint32(math.MaxUint32)

	numberOfShards := uint16(2)
	swn := writer.NewSegmentWriteNotifier(numberOfShards, func(segmentID uint32) { actualSegmentID = segmentID })

	swn.NotifySegmentWrite(0)
	swn.NotifySegmentIsWritten()

	s.Equal(uint32(math.MaxUint32), actualSegmentID)
}

func (s *SegmentWriteNotifierSuite) TestSetAndNotifyOnlyOneShard() {
	actualSegmentID := uint32(math.MaxUint32)

	numberOfShards := uint16(2)
	swn := writer.NewSegmentWriteNotifier(numberOfShards, func(segmentID uint32) { actualSegmentID = segmentID })
	swn.Set(0, 42)

	swn.NotifySegmentWrite(0)
	swn.NotifySegmentIsWritten()

	s.Equal(uint32(math.MaxUint32), actualSegmentID)
}

func (s *SegmentWriteNotifierSuite) TestSetAndNotifyOnlyOneShard_2() {
	actualSegmentID := uint32(math.MaxUint32)

	numberOfShards := uint16(2)
	swn := writer.NewSegmentWriteNotifier(numberOfShards, func(segmentID uint32) { actualSegmentID = segmentID })
	swn.Set(1, 42)

	swn.NotifySegmentWrite(0)
	swn.NotifySegmentIsWritten()

	s.Equal(uint32(0), actualSegmentID)
}
