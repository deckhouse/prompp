package tsdb

import "github.com/prometheus/client_golang/prometheus"

// DBOpts exposes db.opts (usage: pp-pkg/tsdb).
func DBOpts(db *DB) *Options {
	return db.opts
}

// DBTimeRetentionCount exposes time retention metric (usage: pp-pkg/tsdb).
func DBTimeRetentionCount(db *DB) prometheus.Counter {
	return db.metrics.timeRetentionCount
}

// DBSizeRetentionCount exposes size retention metric (usage: pp-pkg/tsdb).
func DBSizeRetentionCount(db *DB) prometheus.Counter {
	return db.metrics.sizeRetentionCount
}

// DBSetBlocksToDelete safely injects blocksToDelete closure (usage cmd/prometheus/main.go)
func DBSetBlocksToDelete(db *DB, blocksToDelete BlocksToDeleteFunc) {
	db.cmtx.Lock()
	defer db.cmtx.Unlock()

	db.blocksToDelete = blocksToDelete
}
