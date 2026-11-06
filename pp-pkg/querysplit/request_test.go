package querysplit

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestParseSettings(t *testing.T) {
	for _, tc := range []struct {
		name     string
		value    string
		expected Settings
		err      bool
	}{
		{name: "empty", value: ""},
		{name: "parallelism", value: "query_range_split_parallelism=8", expected: Settings{Parallelism: 8}},
		{
			name:  "all options",
			value: "query_range_split_parallelism=2, query_range_split_merge_factor=1,query_range_split_min_blocks=6,query_range_split_min_series=5000",
			expected: Settings{
				Parallelism: 2,
				MergeFactor: 1,
				MinBlocks:   6,
				MinSeries:   5000,
			},
		},
		{name: "unknown option", value: "query_range_split_speed=2", err: true},
		{name: "no value", value: "query_range_split_parallelism", err: true},
		{name: "zero value", value: "query_range_split_min_series=0", err: true},
		{name: "not a number", value: "query_range_split_parallelism=many", err: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, err := ParseSettings(tc.value)
			if tc.err {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tc.expected, s)
		})
	}
}

func TestInstrument(t *testing.T) {
	for _, tc := range []struct {
		name           string
		header         []string
		expectedCode   int
		expectedOK     bool
		expectedResult Settings
	}{
		{name: "no header", expectedCode: http.StatusOK},
		{
			name:           "header",
			header:         []string{"query_range_split_parallelism=2"},
			expectedCode:   http.StatusOK,
			expectedOK:     true,
			expectedResult: Settings{Parallelism: 2},
		},
		{name: "incorrect header", header: []string{"query_range_split_parallelism=0"}, expectedCode: http.StatusBadRequest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			var actual Settings
			var actualOK bool
			handler := Instrument("query_range", func(_ http.ResponseWriter, r *http.Request) {
				actual, actualOK = settingsFrom(r.Context())
			})
			r := httptest.NewRequest(http.MethodGet, "/query_range", http.NoBody)
			for _, v := range tc.header {
				r.Header.Add(Header, v)
			}
			w := httptest.NewRecorder()

			// Act
			handler(w, r)

			// Assert
			require.Equal(t, tc.expectedCode, w.Code)
			require.Equal(t, tc.expectedOK, actualOK)
			require.Equal(t, tc.expectedResult, actual)
		})
	}
}

func TestEngineHeaderSettings(t *testing.T) {
	for _, tc := range []struct {
		name                string
		header              *Settings
		expectedSplit       bool
		expectedParallelism int
	}{
		{name: "no header"},
		{name: "empty header", header: &Settings{}, expectedSplit: true, expectedParallelism: DefaultParallelism},
		{name: "header with parallelism", header: &Settings{Parallelism: 2}, expectedSplit: true, expectedParallelism: 2},
		{name: "header turns the split off", header: &Settings{Parallelism: 1}},
		{name: "header with few blocks", header: &Settings{MinBlocks: 20}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			inner := &stubEngine{exec: emptyResult}
			engine := newStubSplitEngine(inner, DefaultParallelism, nil)
			ctx := context.Background()
			if tc.header != nil {
				ctx = WithSettings(ctx, *tc.header)
			}

			// Act
			q, err := engine.NewRangeQuery(ctx, nil, nil, `rate(x[5m])`,
				time.UnixMilli(0), time.UnixMilli(ms(12*time.Hour)), time.Minute)

			// Assert
			require.NoError(t, err)
			sq, split := q.(*splitQuery)
			require.Equal(t, tc.expectedSplit, split)
			if split {
				require.Equal(t, tc.expectedParallelism, sq.limit)
			}
		})
	}
}
