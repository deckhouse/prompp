package block

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/oklog/ulid"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb"
	"github.com/prometheus/prometheus/tsdb/index"
)

// checkContextEveryNIterations is used in some tight loops to check if the context is done.
const checkContextEveryNIterations = 100

//
// Functions
//

// CloseAll closes all given closers.
func CloseAll(cs []io.Closer) error {
	errs := make([]error, 0, len(cs))
	for _, c := range cs {
		errs = append(errs, c.Close())
	}

	return errors.Join(errs...)
}

// DirsOfBlocks returns a list of block directories in the given directory.
func DirsOfBlocks(dir string) ([]string, error) {
	files, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	dirs := make([]string, 0, len(files))
	for _, f := range files {
		if isBlockDir(f) {
			dirs = append(dirs, filepath.Join(dir, f.Name()))
		}
	}

	return dirs, nil
}

// ChunkDir returns the directory of the chunks.
func ChunkDir(dir string) string { return filepath.Join(dir, ChunksDirname) }

// getBlock iterates a given block range to find a block by a given id.
// If found it returns the block itself and a boolean to indicate that it was found.
func getBlock(allBlocks []*Block, id ulid.ULID) (*Block, bool) {
	for _, b := range allBlocks {
		if b.Meta().ULID == id {
			return b, true
		}
	}

	return nil, false
}

// isBlockDir check dir is a block directory by checking if the directory name is a valid ULID.
func isBlockDir(fi fs.DirEntry) bool {
	if !fi.IsDir() {
		return false
	}

	_, err := ulid.ParseStrict(fi.Name())
	return err == nil
}

// labelNamesWithMatchers returns all the label names for the series referred to by the postings.
// The names returned are sorted.
func labelNamesWithMatchers(ctx context.Context, r tsdb.IndexReader, matchers ...*labels.Matcher) ([]string, error) {
	p, err := tsdb.PostingsForMatchers(ctx, r, matchers...)
	if err != nil {
		return nil, err
	}

	return r.LabelNamesFor(ctx, p)
}

// labelValuesWithMatchers returns all the label values for the series referred to by the postings.
// The values returned are sorted.
//
//revive:disable-next-line:function-length // copied from upstream
//revive:disable-next-line:cyclomatic // copied from upstream
//revive:disable-next-line:cognitive-complexity // copied from upstream
func labelValuesWithMatchers(
	ctx context.Context,
	r tsdb.IndexReader,
	name string,
	matchers ...*labels.Matcher,
) ([]string, error) {
	allValues, err := r.LabelValues(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("fetching values of label %s: %w", name, err)
	}

	// If we have a matcher for the label name, we can filter out values that don't match
	// before we fetch postings. This is especially useful for labels with many values.
	// e.g. __name__ with a selector like {__name__="xyz"}
	hasMatchersForOtherLabels := false
	for _, m := range matchers {
		if m.Name != name {
			hasMatchersForOtherLabels = true
			continue
		}

		// re-use the allValues slice to avoid allocations
		// this is safe because the iteration is always ahead of the append
		filteredValues := allValues[:0]
		count := 1
		for _, v := range allValues {
			if count%checkContextEveryNIterations == 0 && ctx.Err() != nil {
				return nil, ctx.Err()
			}

			count++
			if m.Matches(v) {
				filteredValues = append(filteredValues, v)
			}
		}

		allValues = filteredValues
	}

	if len(allValues) == 0 {
		return nil, nil
	}

	// If we don't have any matchers for other labels, then we're done.
	if !hasMatchersForOtherLabels {
		return allValues, nil
	}

	p, err := tsdb.PostingsForMatchers(ctx, r, matchers...)
	if err != nil {
		return nil, fmt.Errorf("fetching postings for matchers: %w", err)
	}

	valuesPostings := make([]index.Postings, len(allValues))
	for i, value := range allValues {
		valuesPostings[i], err = r.Postings(ctx, name, value)
		if err != nil {
			return nil, fmt.Errorf("fetching postings for %s=%q: %w", name, value, err)
		}
	}

	indexes, err := index.FindIntersectingPostings(p, valuesPostings)
	if err != nil {
		return nil, fmt.Errorf("intersecting postings: %w", err)
	}

	values := make([]string, 0, len(indexes))
	for _, idx := range indexes {
		values = append(values, allValues[idx])
	}

	return values, nil
}
