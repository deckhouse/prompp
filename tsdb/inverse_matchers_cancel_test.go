package tsdb

import (
	"context"
	"fmt"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/util/testutil"
)

func TestInverseMatchersHonorCancellation(t *testing.T) {
	opts := DefaultHeadOptions()
	opts.ChunkDirRoot = t.TempDir()
	h, err := NewHead(nil, nil, nil, nil, opts, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, h.Close()) })
	app := h.Appender(t.Context())

	for i := range 2000 {
		_, err = app.Append(0, labels.FromStrings("__name__", "inverse_metric", "target", strconv.Itoa(i)), 0, 1)
		require.NoError(t, err)
	}

	require.NoError(t, app.Commit())
	ir, err := h.Index()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ir.Close()) })

	for _, typ := range []labels.MatchType{labels.MatchRegexp, labels.MatchNotRegexp} {
		t.Run(fmt.Sprint(typ), func(t *testing.T) {
			ctx := &testutil.MockContextErrAfter{FailAfter: 2}
			p, err := PostingsForMatchers(ctx, ir, labels.MustNewMatcher(typ, "target", ".+"))
			if err == nil {
				err = p.Err()
			}

			require.ErrorIs(t, err, context.Canceled)
		})
	}
}
