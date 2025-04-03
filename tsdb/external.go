package tsdb

import "github.com/prometheus/client_golang/prometheus"

func DBOpts(db *DB) *Options {
	return db.opts
}

func DBTimeRetentionCount(db *DB) prometheus.Counter {
	return db.metrics.timeRetentionCount
}

func DBSizeRetentionCount(db *DB) prometheus.Counter {
	return db.metrics.sizeRetentionCount
}

func DBSetBlocksToDelete(db *DB, blocksToDeleteFactory func(db *DB) BlocksToDeleteFunc) {
	db.cmtx.Lock()
	defer db.cmtx.Unlock()

	db.blocksToDelete = blocksToDeleteFactory(db)
}
