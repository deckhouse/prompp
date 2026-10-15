package storage

import "github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"

// DisableWalWriterV2 restores the default V1 shard WAL writer switched by [EnableWalWriterV2].
func DisableWalWriterV2() {
	walVersion = uint8(wal.FileFormatVersion)

	walWriterCtor = walWriterCtorV1
}
