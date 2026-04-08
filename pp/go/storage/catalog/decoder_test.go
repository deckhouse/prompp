package catalog_test

import (
	"bytes"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/storage/catalog"
)

type DecoderSuite struct {
	suite.Suite
}

func TestDecoderSuite(t *testing.T) {
	suite.Run(t, new(DecoderSuite))
}

func (s *DecoderSuite) TestDecoderV3Decode() {
	buffer := bytes.NewBuffer(nil)
	record := catalog.NewRecordWithDataV3(uuid.New(), 5, 25, 26, 27, true, catalog.StatusActive, 25, 2, 3)

	encoder := catalog.NewEncoderV3()
	s.Require().NoError(encoder.EncodeTo(buffer, &record.SerializedRecord))

	decoder := catalog.NewDecoderV3()
	decodedRecord := &catalog.Record{}
	s.Require().NoError(decoder.DecodeFrom(buffer, &decodedRecord.SerializedRecord))

	s.Require().Equal(record.ID(), decodedRecord.ID())
	s.Require().Equal(record.NumberOfShards(), decodedRecord.NumberOfShards())
	s.Require().Equal(record.CreatedAt(), decodedRecord.CreatedAt())
	s.Require().Equal(record.UpdatedAt(), decodedRecord.UpdatedAt())
	s.Require().Equal(record.DeletedAt(), decodedRecord.DeletedAt())
	s.Require().Equal(record.Corrupted(), decodedRecord.Corrupted())
	s.Require().Equal(record.Status(), decodedRecord.Status())
	s.Require().Equal(record.NumberOfSegments(), decodedRecord.NumberOfSegments())
	s.Require().Equal(record.Maxt(), decodedRecord.Maxt())
	s.Require().Equal(record.Mint(), decodedRecord.Mint())
}

func BenchmarkDecodeV3(b *testing.B) {
	buffer := bytes.NewBuffer(nil)
	record := catalog.NewRecordWithDataV3(uuid.New(), 5, 25, 26, 27, true, catalog.StatusActive, 25, 2, 3)
	var encoder catalog.Encoder
	decodedRecord := &catalog.Record{}
	b.StopTimer()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buffer.Reset()
		encoder = catalog.NewEncoderV3()
		require.NoError(b, encoder.EncodeTo(buffer, &record.SerializedRecord))
		decoder := catalog.NewDecoderV3()
		b.StartTimer()
		require.NoError(b, decoder.DecodeFrom(buffer, &decodedRecord.SerializedRecord))
		b.StopTimer()
	}
}

func BenchmarkDecodeV3_State(b *testing.B) {
	buffer := bytes.NewBuffer(nil)
	record := catalog.NewRecordWithDataV3(uuid.New(), 5, 25, 26, 27, true, catalog.StatusActive, 25, 2, 3)
	encoder := catalog.NewEncoderV3()
	decodedRecord := &catalog.Record{}
	decoder := catalog.NewDecoderV3()
	b.StopTimer()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		buffer.Reset()
		require.NoError(b, encoder.EncodeTo(buffer, &record.SerializedRecord))
		b.StartTimer()
		require.NoError(b, decoder.DecodeFrom(buffer, &decodedRecord.SerializedRecord))
		b.StopTimer()
	}
}
