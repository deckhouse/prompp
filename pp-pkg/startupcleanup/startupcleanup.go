// Package startupcleanup performs a best-effort cleanup of the local storage
// directory before Prom++ starts its background goroutines: it removes leftover
// tmp block dirs and then the blocks that no longer fit the configured
// retention. Both phases are deliberately filesystem-only and do not depend on
// the heads catalog, because a disk left full is the very reason the catalog
// cannot be opened.
package startupcleanup

import (
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/prometheus/prometheus/tsdb"
)

// component is the value of the logger component field.
const component = "startupcleanup"

// Enabled reports whether the pre-start cleanup runs.
// Turned on with PROMPP_FEATURES=enable_startup_cleanup.
var Enabled bool

// RemoveLeftoverTmpDirs only needs the data dir, so it runs before the heads
// catalog is opened: leftover garbage can be the very reason the catalog cannot
// be created (e.g. no free space left). Only tmp block dirs are touched, which
// are identified by a ULID name plus a tmp suffix, so no catalog knowledge is
// needed to tell them apart from live data.
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
