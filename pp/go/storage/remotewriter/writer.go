package remotewriter

import (
	"context"

	"github.com/prometheus/prometheus/storage/remote"
)

// protobufWriter the wrapper over the [remote.WriteClient].
type protobufWriter struct {
	client remote.WriteClient
}

// newProtobufWriter init new [protobufWriter].
func newProtobufWriter(client remote.WriteClient) *protobufWriter {
	return &protobufWriter{
		client: client,
	}
}

// Close implementation [io.Closer].
func (*protobufWriter) Close() error {
	return nil
}

// Write [cppbridge.SnappyProtobufEncodedData] to [remote.WriteClient]
func (w *protobufWriter) Write(ctx context.Context, protobuf []byte) error {
	if len(protobuf) == 0 {
		return nil
	}

	// TODO WriteResponseStats
	_, err := w.client.Store(ctx, protobuf, 0)
	return err
}
