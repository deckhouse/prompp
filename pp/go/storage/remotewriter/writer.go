package remotewriter

import (
	"context"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/storage/remote"
)

// protobufWriter
type protobufWriter struct {
	client remote.WriteClient
}

func newProtobufWriter(client remote.WriteClient) *protobufWriter {
	return &protobufWriter{
		client: client,
	}
}

func (w *protobufWriter) Write(ctx context.Context, protobuf *cppbridge.SnappyProtobufEncodedData) error {
	return protobuf.Do(func(buf []byte) error {
		if len(buf) == 0 {
			return nil
		}

		// TODO WriteResponseStats
		_, err := w.client.Store(ctx, buf, 0)
		return err
	})
}

func (w *protobufWriter) Close() error {
	return nil
}
