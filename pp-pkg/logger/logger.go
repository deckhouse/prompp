package logger

import (
	"fmt"

	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/prometheus/prometheus/pp/go/logger"
)

// InitLogHandler init log handler for pp.
func InitLogHandler(l log.Logger) {
	l = log.With(l, "pp_caller", log.Caller(4))

	logger.Debugf = func(template string, args ...any) {
		_ = level.Debug(l).Log("msg", fmt.Sprintf(template, args...))
	}

	logger.Infof = func(template string, args ...any) {
		_ = level.Info(l).Log("msg", fmt.Sprintf(template, args...))
	}

	logger.Warnf = func(template string, args ...any) {
		_ = level.Warn(l).Log("msg", fmt.Sprintf(template, args...))
	}

	logger.Errorf = func(template string, args ...any) {
		_ = level.Error(l).Log("msg", fmt.Sprintf(template, args...))
	}
}
