package handler

import (
	"context"

	"github.com/odarix/odarix-core-go/cppbridge"
	"github.com/odarix/odarix-core-go/relabeler"
)

// Receiver implements.
type Receiver interface {
	AppendProtobuf(ctx context.Context, data relabeler.ProtobufData, relabelerID string) error
	AppendHashdex(ctx context.Context, hashdex cppbridge.ShardedData, relabelerID string) error
	RelabelerIDIsExist(relabelerID string) bool
}
