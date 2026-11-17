package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"text/tabwriter"
	"time"

	"github.com/alecthomas/kingpin/v2"
	"github.com/alecthomas/units"
	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/pp-pkg/blocks/block"
)

// cmdBlocks is the command for listing blocks.
type cmdBlocks struct {
	humanReadable bool
}

// registerCmdBlocks registers the blocks command.
func registerCmdBlocks(cmd *cmdBlocks, clause *kingpin.CmdClause) {
	clause.Flag(
		"human-readable",
		"Print human readable values. Default is false.",
	).Default("false").Short('r').BoolVar(&cmd.humanReadable)
}

// Do lists the blocks in the directory.
func (cmd *cmdBlocks) Do(
	ctx context.Context,
	workingDir string,
	logger log.Logger,
	registerer prometheus.Registerer,
) error {
	return listBlocks(logger, workingDir, cmd.humanReadable)
}

// listBlocks lists the blocks in the directory.
func listBlocks(logger log.Logger, path string, humanReadable bool) error {
	blocks, _, err := block.OpenBlocks(logger, path, nil, nil)
	if err != nil {
		return err
	}
	defer func() {
		err = errors.Join(err, block.CloseAll(blocks))
	}()

	slices.SortFunc(blocks, func(a, b *block.Block) int {
		switch {
		case a.Meta().MinTime < b.Meta().MinTime:
			return -1
		case a.Meta().MinTime > b.Meta().MinTime:
			return 1
		default:
			return 0
		}
	})

	printBlocks(blocks, true, humanReadable)

	return nil
}

// printBlocks prints the blocks to the console.
func printBlocks(blocks []*block.Block, writeHeader, humanReadable bool) {
	tw := tabwriter.NewWriter(os.Stdout, 13, 0, 2, ' ', 0)
	defer tw.Flush()

	if writeHeader {
		fmt.Fprintln(
			tw,
			"BLOCK ULID\tMIN TIME\tMAX TIME\tDURATION\tNUM SAMPLES\tNUM CHUNKS\tNUM SERIES\tSIZE\tRESOLUTION\tLABELS",
		)
	}

	for _, b := range blocks {
		meta := b.Metadata()

		fmt.Fprintf(tw,
			"%v\t%v\t%v\t%v\t%v\t%v\t%v\t%v\t%v\t%v\n",
			meta.ULID,
			getFormatedTime(meta.MinTime, humanReadable),
			getFormatedTime(meta.MaxTime, humanReadable),
			time.Duration(meta.MaxTime-meta.MinTime)*time.Millisecond,
			meta.Stats.NumSamples,
			meta.Stats.NumChunks,
			meta.Stats.NumSeries,
			getFormatedBytes(b.Size(), humanReadable),
			getFormatedDuration(meta.Thanos.Downsample.Resolution, humanReadable),
			labelsToString(meta.Thanos.Labels),
		)
	}
}

// getFormatedTime converts a timestamp to a human readable string.
func getFormatedTime(timestamp int64, humanReadable bool) string {
	if humanReadable {
		return time.Unix(timestamp/1000, 0).UTC().String()
	}

	return strconv.FormatInt(timestamp, 10)
}

// getFormatedBytes converts a number of bytes to a human readable string.
func getFormatedBytes(bytes int64, humanReadable bool) string {
	if humanReadable {
		return units.Base2Bytes(bytes).String()
	}

	return strconv.FormatInt(bytes, 10)
}

// getFormatedDuration converts a duration to a human readable string.
func getFormatedDuration(duration int64, humanReadable bool) string {
	if humanReadable {
		return (time.Duration(duration) * time.Millisecond).String()
	}

	return strconv.FormatInt(duration, 10)
}

// labelsToString converts a map[string]string to a JSON string.
func labelsToString(ls map[string]string) string {
	data, err := json.Marshal(ls)
	if err != nil {
		return err.Error()
	}

	return string(data)
}
