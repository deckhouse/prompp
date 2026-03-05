package promql_test

// OP_FUNCTIONS

import (
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/prometheus/prometheus/util/teststorage"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestExtractOptTop(t *testing.T) {
	query := "sum without (l)(rate(a_X[1m])) / sum without (l)(rate(b_X[1m]))"
	_ = query
	opTopQuery := "op_top(5, max by(database, user) (op_zero_if_none(postgresql__connections__max_transaction_age{instance=\"pg\", conf=\"cfg\"}[1m])))"
	_ = opTopQuery
	invalidOpTopQuery := "op_top(5, max by(database, user) (op_top(postgresql__connections__max_transaction_age{instance=\"pg\", conf=\"cfg\"}[1m])))"
	_ = opTopQuery

	stor := teststorage.New(t)
	defer stor.Close()
	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 50000000,
		Timeout:    100 * time.Second,
	}
	// Enable experimental functions testing
	parser.EnableExperimentalFunctions = true
	engine := promqltest.NewTestEngineWithOpts(t, opts)

	const interval = 10000 // 10s interval.
	// A day of data plus 10k steps.
	numIntervals := 8640 + 10000
	err := setupRangeQueryTestData(stor, engine, interval, numIntervals)
	require.NoError(t, err)

	q, err := engine.NewRangeQuery(t.Context(), stor, nil, query, time.Now(), time.Now().Add(time.Minute), time.Second*10)
	require.NoError(t, err)
	q.Close()

	q, err = engine.NewRangeQuery(t.Context(), stor, nil, opTopQuery, time.Now(), time.Now().Add(time.Minute), time.Second*10)
	require.NoError(t, err)
	q.Close()

	q, err = engine.NewRangeQuery(t.Context(), stor, nil, invalidOpTopQuery, time.Now(), time.Now().Add(time.Minute), time.Second*10)
	require.Error(t, err)
}
