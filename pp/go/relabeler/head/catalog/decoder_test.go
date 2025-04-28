package catalog_test

import (
	"bytes"
	"github.com/google/uuid"
	"github.com/prometheus/prometheus/pp/go/relabeler/head/catalog"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestDecoderV3_Decode(t *testing.T) {
	buffer := bytes.NewBuffer(nil)
	record := catalog.NewRecordWithDataV3(uuid.New(), 5, 25, 26, 27, true, catalog.StatusActive, 25, 2, 3)

	encoder := catalog.NewEncoderV3()
	require.NoError(t, encoder.Encode(buffer, record))
	decoder := catalog.NewDecoderV3()
	decodedRecord := &catalog.Record{}
	require.NoError(t, decoder.Decode(buffer, decodedRecord))

	require.Equal(t, record.ID(), decodedRecord.ID())
	require.Equal(t, record.NumberOfShards(), decodedRecord.NumberOfShards())
	require.Equal(t, record.CreatedAt(), decodedRecord.CreatedAt())
	require.Equal(t, record.UpdatedAt(), decodedRecord.UpdatedAt())
	require.Equal(t, record.DeletedAt(), decodedRecord.DeletedAt())
	require.Equal(t, record.Corrupted(), decodedRecord.Corrupted())
	require.Equal(t, record.Status(), decodedRecord.Status())
	require.Equal(t, record.NumberOfSegments(), decodedRecord.NumberOfSegments())
	require.Equal(t, record.Maxt(), decodedRecord.Maxt())
	require.Equal(t, record.Mint(), decodedRecord.Mint())
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
		require.NoError(b, encoder.Encode(buffer, record))
		decoder := catalog.NewDecoderV3()
		b.StartTimer()
		require.NoError(b, decoder.Decode(buffer, decodedRecord))
		b.StopTimer()
	}
}
