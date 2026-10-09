package wal

import (
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp/go/util/stagestats"
)

// Stages of the [Wal] operations.
const (
	stageWrite stagestats.Stage = iota
	stageCommit
	stageLongCommit
	stageFlush
	stageSync
)

// stageNames names of the [Wal] operations stages.
var stageNames = []string{
	"write",
	"commit",
	"long_commit",
	"flush",
	"sync",
}

// newStageRecorder returns the stage stats recorder of the [Wal] operations registered in r,
// all wals of the registerer share one recorder.
func newStageRecorder(r prometheus.Registerer) *stagestats.Recorder {
	return stagestats.NewRecorder(
		r,
		stagestats.Opts{
			Name: "prompp_shard_wal_stage",
			Help: "Shard wal operations.",
		},
		stageNames,
	)
}
