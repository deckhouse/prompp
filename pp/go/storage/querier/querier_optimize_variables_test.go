//go:build !asan

package querier_test

import (
	"fmt"
	"time"

	"github.com/prometheus/common/model"
)

const (
	// defaultCountOfSteps is the default count of steps.
	defaultCountOfSteps = 240
)

// defaultSteps is the default steps.
var defaultSteps = []time.Duration{
	defaultStep - time.Second,       // [14s]
	defaultStep,                     // [15s]
	(defaultStep - time.Second) * 2, // [29s]
	defaultStep * 2,                 // [30s]
	(defaultStep - time.Second) * 4, // [59s]
	defaultStep * 4,                 // [60s]
	(defaultStep - time.Second) * 5, // [70s]
}

// defaultSubQueries is the default subqueries.
var defaultSubQueries = []subQuery{
	{window: model.Duration(defaultStep), step: 0},                                    // [15s]
	{window: model.Duration(defaultStep * 4), step: 0},                                // [60s]
	{window: model.Duration(defaultStep*4 - time.Second), step: 0},                    // [59s]
	{window: model.Duration(defaultStep*4 + time.Second), step: 0},                    // [61s]
	{window: model.Duration(defaultStep * 4), step: 0, defaultStep: true},             // [60s:]
	{window: model.Duration(defaultStep * 16), step: model.Duration(defaultStep * 4)}, // [240s:60s]
	{window: model.Duration(defaultStep*16 - time.Second), step: 0},                   // [239s]
	{window: model.Duration(defaultStep * 16), step: 0},                               // [240s]
	{window: model.Duration(defaultStep*16 + time.Second), step: 0},                   // [241s]
}

// defaultModifiers is the default modifiers.
var defaultModifiers = []modifier{
	modifierNone,
	modifier(fmt.Sprintf(modifierAt, defaultStartMs/1e3+defaultStep*defaultCountOfSteps/2e9)), // middle of the range
	modifierEnd,
	modifierStart,
}

// defaultOffsets is the default offsets.
var defaultOffsets = []offset{
	newOffset(0),
	newOffset(defaultStep * defaultCountOfSteps / 2),
	newOffset(-defaultStep * defaultCountOfSteps / 2),
	newOffset(defaultStep * defaultCountOfSteps),
	newOffset(-defaultStep * defaultCountOfSteps),
}

// TestQueryRangeWithStep long test the query range with step.
func (s *MatrixQuerierOptimizeSuiteSuite) TestQueryRangeWithStep() {
	ctx := s.T().Context()
	w := s.end.Sub(s.start) / 3
	stepWindow := w * 2 / 3

	for st, end := s.start, s.start.Add(w); !end.After(s.end); st, end = st.Add(stepWindow), end.Add(stepWindow) {
		s.rangeArgs(func(qFunc queryFunc, mName string, step time.Duration, subq subQuery, mod modifier, o offset) {
			query := qFunc.toQueryString(mName, subq, mod, o)
			s.Run(fmt.Sprintf("%s_step_%s", query, step), func() {
				res, err := queryRange(ctx, "none", s.queryEngine, s, s.queryOpts, query, st, end, step)
				s.Require().NoError(err)
				defer res.qry.Close()

				resOpt, err := queryRange(ctx, "all", s.queryEngine, s, s.queryOpts, query, st, end, step)
				s.Require().NoError(err)
				defer resOpt.qry.Close()

				s.Require().True(resultEqual(res.res, resOpt.res, query))
			})
		})
	}
}
