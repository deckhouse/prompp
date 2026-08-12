package main

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"

	"github.com/prometheus/common/promlog"
	"github.com/prometheus/common/version"

	"github.com/alecthomas/kingpin/v2"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"

	"github.com/prometheus/prometheus/pp-pkg/featuresflags"
	pppkglogger "github.com/prometheus/prometheus/pp-pkg/logger"
)

func main() {
	app := kingpin.New(
		filepath.Base(os.Args[0]),
		"Tooling for the Prom++ monitoring system.",
	).UsageWriter(os.Stdout)
	app.Version(version.Print("prompptool"))
	app.HelpFlag.Short('h')

	verbose := app.Flag("verbose", "Print debug logs.").Short('v').Bool()

	workingDir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	app.Flag("working-dir", "Working dir with wals and blocks").
		Default(workingDir).
		ExistingDirVar(&workingDir)

	walppClause := app.Command("walpp", "Converting prom++ wal to tsdb-blocks.")
	var walppCmd cmdWALPPToBlock
	registerCmdWALPPToBlock(&walppCmd, walppClause)

	walvanillaClause := app.Command("walvanilla", "Converting prometheus wal to tsdb-blocks.")
	var walvanillaCmd cmdWALVanillaToBlock
	registerCmdWALVanillaToBlock(&walvanillaCmd, walvanillaClause)

	persistHeadClause := app.Command("persist-head", "Persist a single prom++ head to tsdb-blocks by its directory path.")
	var persistHeadCmd cmdPersistHead
	registerCmdPersistHead(&persistHeadCmd, persistHeadClause)

	cmd := kingpin.MustParse(app.Parse(os.Args[1:]))
	logger := initLogger(*verbose)
	logger = log.With(logger, "cmd", cmd)

	featuresflags.ReadPromPPFeatures(logger, noopFlagConfig{})

	ctx, _ := signal.NotifyContext(context.Background(), os.Interrupt)
	switch cmd {
	case walppClause.FullCommand():
		if err := walppCmd.Do(ctx, workingDir, logger, nil); err != nil {
			level.Error(logger).Log("msg", "fail to convert", "error", err)
			os.Exit(1)
		}
	case walvanillaClause.FullCommand():
		if err := walvanillaCmd.Do(ctx, workingDir, logger); err != nil {
			level.Error(logger).Log("msg", "fail to convert", "error", err)
			os.Exit(1)
		}
	case persistHeadClause.FullCommand():
		if err := persistHeadCmd.Do(ctx, logger, nil); err != nil {
			level.Error(logger).Log("msg", "fail to persist head", "error", err)
			os.Exit(1)
		}
	}
}

func initLogger(verbose bool) log.Logger {
	l := &promlog.AllowedLevel{}
	if verbose {
		_ = l.Set("debug")
	} else {
		_ = l.Set("info")
	}
	f := &promlog.AllowedFormat{}
	_ = f.Set("logfmt")

	logger := promlog.New(&promlog.Config{Level: l, Format: f})

	initLogHandler(logger)

	return logger
}

func initLogHandler(logger log.Logger) {
	pppkglogger.InitLogHandler(logger)
}

//
// noopFlagConfig
//

// noopFlagConfig is a no-op implementation of the FlagConfig interface, used when no feature flags are set.
type noopFlagConfig struct{}

// DisableBlockManagerStorage is a no-op implementation of the FlagConfig interface, used when no feature flags are set.
func (noopFlagConfig) DisableBlockManagerStorage() {}
