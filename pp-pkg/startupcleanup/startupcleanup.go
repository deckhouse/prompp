// Package startupcleanup performs a best-effort cleanup of the local storage
// directory before Prom++ starts its background goroutines. It runs in two
// phases because the garbage left on disk can be the very reason the heads
// catalog cannot be opened, so the filesystem-only phase must not depend on it.
package startupcleanup

import (
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/prometheus/prometheus/tsdb"
)

// component is the value of the logger component field for both phases.
const component = "startupcleanup"

// Enabled reports whether the pre-start cleanup phases run.
// Turned on with PROMPP_FEATURES=enable_startup_cleanup.
var Enabled bool

//
// HeadsGC
//

// HeadsGC runs a single garbage collection pass over the heads catalog.
type HeadsGC interface {
	// Iterate over the catalog list and remove old heads.
	Iterate()
}

// RemoveLeftoverTmpDirs is the first cleanup phase. It only needs the data dir,
// so it runs before the heads catalog is opened: leftover garbage can be the very
// reason the catalog cannot be created (e.g. no free space left). Only tmp block
// dirs are touched, which are identified by a ULID name plus a tmp suffix, so no
// catalog knowledge is needed to tell them apart from live data.
func RemoveLeftoverTmpDirs(logger log.Logger, dir string) {
	if !Enabled {
		return
	}

	logger = log.With(logger, "component", component)
	_ = level.Info(logger).Log("msg", "Removing leftover tmp block dirs before start", "dir", dir)

	if err := tsdb.RemoveBestEffortTmpDirs(logger, dir); err != nil {
		_ = level.Warn(logger).Log("msg", "failed to remove leftover tmp block dirs", "dir", dir, "err", err)
	}
}

// CollectHeads is the second cleanup phase: a single catalog GC pass over heads
// already eligible for deletion, e.g. persisted before a restart that happened
// before the periodic collector got to them. It needs the GC, so it runs once the
// catalog is up, but still before any background goroutine starts.
func CollectHeads(gc HeadsGC, logger log.Logger) {
	if !Enabled || gc == nil {
		return
	}

	_ = level.Info(log.With(logger, "component", component)).Log("msg", "Collecting heads before start")
	gc.Iterate()
}
