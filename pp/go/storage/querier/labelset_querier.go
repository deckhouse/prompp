package querier

import (
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

//
// Shard
//

// Shard the minimum required head [Shard] implementation.
type Shard interface {
	// LSSFindByHash find label set by hash in cache.
	LSSFindByHash(
		hash uint64,
		builderSortedAdd []cppbridge.Label,
		builderSortedDel []string,
		builderSnapshot *cppbridge.LabelSetSnapshot,
		builderLSID uint32,
	) (labels.Labels, bool)

	// LSSFindFromBuilder label set from builder in lss, return length ls, lsid and bool ok.
	LSSFindFromBuilder(
		sortedAdd []cppbridge.Label,
		sortedDel []string,
		snapshot *cppbridge.LabelSetSnapshot,
		hash uint64,
		lsID uint32,
		skipCache bool,
	) (labels.Labels, bool)
}

//
// HeadShard
//

// HeadShard the minimum required [Head] implementation.
type HeadShard[TShard Shard] interface {
	// NumberOfShards returns current number of shards in to [Head].
	NumberOfShards() uint16

	// ShardByID returns the [Shard] by ID.
	ShardByID(shardID uint16) TShard
}

// FindByHash label set by hash in cache.
func FindByHash[TShard Shard, THeadShard HeadShard[TShard]](
	head THeadShard,
	builderSortedAdd []cppbridge.Label,
	builderSortedDel []string,
	builderSnapshot *cppbridge.LabelSetSnapshot,
	hash uint64,
	builderLSID uint32,
) (labels.Labels, bool) {
	return head.ShardByID(
		uint16(hash%uint64(head.NumberOfShards())), // #nosec G115 // no overflow // shardID by hash
	).LSSFindByHash(hash, builderSortedAdd, builderSortedDel, builderSnapshot, builderLSID)
}

// FindFromBuilder label set from builder in lss, if not found return EmptyLabels.
//
//revive:disable-next-line:flag-parameter this is not a flag, but a parameter
func FindFromBuilder[TShard Shard, THeadShard HeadShard[TShard]](
	head THeadShard,
	builderSortedAdd []cppbridge.Label,
	builderSortedDel []string,
	builderSnapshot *cppbridge.LabelSetSnapshot,
	hash uint64,
	builderLSID uint32,
	skipCache bool,
) (labels.Labels, bool) {
	shard := head.ShardByID(
		uint16(hash % uint64(head.NumberOfShards())), // #nosec G115 // no overflow // shardID by hash
	)

	if ls, ok := shard.LSSFindByHash(hash, builderSortedAdd, builderSortedDel, builderSnapshot, builderLSID); ok {
		return ls, true
	}

	return shard.LSSFindFromBuilder(builderSortedAdd, builderSortedDel, builderSnapshot, hash, builderLSID, skipCache)
}
