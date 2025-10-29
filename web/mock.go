package web

import (
	"github.com/prometheus/prometheus/pp/go/storage/ready"
	v1 "github.com/prometheus/prometheus/web/api/v1"
)

//go:generate -command moq go tool github.com/matryer/moq --rm --skip-ensure --pkg mock --out

//go:generate moq mock/tsdb_admin_stats.go . TSDBAdminStats
type TSDBAdminStats = v1.TSDBAdminStats

//go:generate moq mock/ready_notifier.go . ReadyNotifier
type ReadyNotifier = ready.Notifier
