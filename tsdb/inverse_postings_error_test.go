package tsdb

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/tsdb/index"
)

// inverseErrorIndex verifies that inverse matching preserves reader error chains.
type inverseErrorIndex struct {
	IndexReader
	err error
}

func (r inverseErrorIndex) PostingsForAllLabelValues(context.Context, string) index.Postings {
	return index.ErrPostings(r.err)
}

func (r inverseErrorIndex) PostingsForLabelMatching(context.Context, string, func(string) bool) index.Postings {
	return index.ErrPostings(r.err)
}

func TestInversePostingsReaderErrors(t *testing.T) {
	failure := errors.New("index read failed")
	reader := inverseErrorIndex{err: fmt.Errorf("decode postings: %w", failure)}

	for _, matcher := range []*labels.Matcher{
		labels.MustNewMatcher(labels.MatchEqual, "target", ""),
		labels.MustNewMatcher(labels.MatchRegexp, "target", ""),
		labels.MustNewMatcher(labels.MatchRegexp, "target", "a.*"),
	} {
		t.Run(matcher.String(), func(t *testing.T) {
			_, err := inversePostingsForMatcher(t.Context(), reader, matcher)
			require.ErrorIs(t, err, failure)
		})
	}
}

func TestOOOCompactionAllLabelValuesUnsupported(t *testing.T) {
	reader := &OOOCompactionHeadIndexReader{}

	p := reader.PostingsForAllLabelValues(t.Context(), "target")

	require.False(t, p.Next())
	require.ErrorContains(t, p.Err(), "not supported")
}
