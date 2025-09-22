package writer_test

import (
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
	//

	numberOfShards := uint16(2)
	swn := writer.NewSegmentWriteNotifier(numberOfShards, func(segmentID uint32) { s.T().Log(segmentID) })

	swn.NotifySegmentIsWritten(1)
	swn.NotifySegmentIsWritten(0)
}
