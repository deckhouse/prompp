//go:build !asan

package querier_test

import (
	"fmt"
	"math/rand"
	"reflect"
	"testing"
	"testing/quick"
	"time"

	"github.com/go-kit/log"
	"github.com/prometheus/common/model"
	"github.com/prometheus/prometheus/promql"
	"github.com/stretchr/testify/suite"
)

//
// constants
//

// defaultJitterMs is the default jitter.
const defaultJitterMs = 600000

//
// QueryParam
//

// QueryParam is the struct for query parameter.
type QueryParam struct {
	Start time.Time
	End   time.Time
	Step  time.Duration
}

// Generate generates a random query parameter.
func (qp QueryParam) Generate(rd *rand.Rand, _ int) reflect.Value {
	qp.gen(rd)

	return reflect.ValueOf(qp)
}

// gen generates a random query parameter.
func (qp *QueryParam) gen(rd *rand.Rand) {
	qp.Start = time.UnixMilli(defaultStartMs - defaultJitterMs + rd.Int63n(2*defaultJitterMs))
	qp.End = qp.Start.Add(
		time.Duration(rd.Int63n((defaultStep*defaultCountOfSteps).Milliseconds()+defaultJitterMs) * 1e6), // ms to ns
	)

	qp.Step = 100 * time.Millisecond

	diff := qp.End.Sub(qp.Start)
	if diff <= qp.Step {
		return
	}

	rndStep := time.Duration(rd.Int63n(diff.Milliseconds()) * 1e6) // ms to ns
	if rndStep <= qp.Step {
		return
	}

	qp.Step = rndStep
}

//
// SubQueryParams
//

type SubQueryParams struct {
	QueryParam
	SubQueryStep  time.Duration
	SubQueryRange time.Duration
}

// Generate generates a random query parameter.
func (sqp SubQueryParams) Generate(rd *rand.Rand, _ int) reflect.Value {
	sqp.subGen(rd)

	return reflect.ValueOf(sqp)
}

// subGen generates a random subquery parameter.
func (sqp *SubQueryParams) subGen(rd *rand.Rand) {
	sqp.gen(rd)

	sqp.SubQueryStep = 100 * time.Millisecond
	sqp.SubQueryRange = 100 * time.Millisecond
	diff := sqp.End.Sub(sqp.Start)

	if diff > sqp.SubQueryStep {
		rndStep := time.Duration(rd.Int63n(diff.Milliseconds()) * 1e6) // ms to ns
		if rndStep > sqp.SubQueryStep {
			sqp.SubQueryStep = rndStep
		}
	}

	if diff > sqp.SubQueryRange {
		rndStep := time.Duration(rd.Int63n(diff.Milliseconds()) * 1e6) // ms to ns
		if rndStep > sqp.SubQueryRange {
			sqp.SubQueryRange = rndStep
		}
	}
}

//
// ModifierQueryParams
//

type ModifierQueryParams struct {
	SubQueryParams
	ModifierAt time.Time
}

// Generate generates a random query parameter.
//
//nolint:gocritic // hugeParam // this is a test function
func (mqp ModifierQueryParams) Generate(rd *rand.Rand, _ int) reflect.Value {
	mqp.modGen(rd)

	return reflect.ValueOf(mqp)
}

// modGen generates a random modifier parameter.
func (mqp *ModifierQueryParams) modGen(rd *rand.Rand) {
	mqp.subGen(rd)

	shiftMs := 100 * time.Millisecond
	diff := mqp.End.Sub(mqp.Start)

	mqp.ModifierAt = mqp.Start.Add(shiftMs)
	if diff <= shiftMs {
		return
	}

	rndShiftMs := time.Duration(rd.Int63n(diff.Milliseconds()) * 1e6) // ms to ns
	if rndShiftMs <= shiftMs {
		return
	}

	mqp.ModifierAt = mqp.Start.Add(rndShiftMs)
}

//
// OffsetQueryParams
//

type OffsetQueryParams struct {
	ModifierQueryParams
	Offset time.Duration
}

// Generate generates a random query parameter.
//
//nolint:gocritic // hugeParam // this is a test function
func (oqp OffsetQueryParams) Generate(rd *rand.Rand, _ int) reflect.Value {
	oqp.offsetGen(rd)

	return reflect.ValueOf(oqp)
}

// offsetGen generates a random offset parameter.
func (oqp *OffsetQueryParams) offsetGen(rd *rand.Rand) {
	oqp.modGen(rd)

	oqp.Offset = time.Duration(0)
	diff := oqp.End.Sub(oqp.Start)

	if diff == oqp.Offset {
		return
	}

	oqp.Offset = time.Duration(rd.Int63n(diff.Milliseconds()) * 1e6) // ms to ns
}

//
// QuickQuerierOptimizeSuite
//

type QuickQuerierOptimizeSuite struct {
	QuerierOptimizeSuite

	quickQE *promql.Engine
}

func TestQuickQuerierOptimizeSuite(t *testing.T) {
	suite.Run(t, new(QuickQuerierOptimizeSuite))
}

func (s *QuickQuerierOptimizeSuite) SetupSuite() {
	s.QuerierOptimizeSuite.SetupSuite()

	s.quickQE = promql.NewEngine(promql.EngineOpts{
		Logger:                   log.NewNopLogger(),
		MaxSamples:               500000,
		Timeout:                  10 * time.Second,
		LookbackDelta:            s.lookbackDelta,
		NoStepSubqueryIntervalFn: func(int64) int64 { return s.lookbackDelta.Milliseconds() },
		EnableAtModifier:         true,
		EnableNegativeOffset:     true,
	})
}

func (s *QuickQuerierOptimizeSuite) TestQueryRangeQuickQueryParam() {
	ctx := s.T().Context()

	for _, qFunc := range s.queryFuncs {
		for _, metricName := range s.metricNames {
			f := func(qp QueryParam) bool {
				query := qFunc.toQueryString(
					metricName,
					subQuery{window: model.Duration(s.step * 4), step: 0},
					modifierNone,
					newOffset(0),
				)

				res, err := queryRange(ctx, "none", s.quickQE, s, s.queryOpts, query, qp.Start, qp.End, qp.Step)
				s.Require().NoError(err)
				defer res.qry.Close()

				resOpt, err := queryRange(ctx, "all", s.quickQE, s, s.queryOpts, query, qp.Start, qp.End, qp.Step)
				s.Require().NoError(err)
				defer resOpt.qry.Close()

				return s.Equal(res.res, resOpt.res)
			}

			s.Require().NoError(quick.Check(f, nil))
		}
	}
}

func (s *QuickQuerierOptimizeSuite) TestQueryRangeQuickSubQueryParams() {
	ctx := s.T().Context()

	for _, qFunc := range s.queryFuncs {
		for _, metricName := range s.metricNames {
			f := func(sqp SubQueryParams) bool {
				query := qFunc.toQueryString(
					metricName,
					subQuery{window: model.Duration(sqp.SubQueryRange), step: 0},
					modifierNone,
					newOffset(0),
				)

				res, err := queryRange(ctx, "none", s.quickQE, s, s.queryOpts, query, sqp.Start, sqp.End, sqp.Step)
				s.Require().NoError(err)
				defer res.qry.Close()

				resOpt, err := queryRange(ctx, "all", s.quickQE, s, s.queryOpts, query, sqp.Start, sqp.End, sqp.Step)
				s.Require().NoError(err)
				defer resOpt.qry.Close()

				return s.Equal(res.res, resOpt.res)
			}

			s.Require().NoError(quick.Check(f, nil))
		}
	}
}

func (s *QuickQuerierOptimizeSuite) TestQueryRangeQuickModifierQueryParams() {
	ctx := s.T().Context()

	for _, qFunc := range s.queryFuncs {
		for _, metricName := range s.metricNames {
			f := func(mqp ModifierQueryParams) bool {
				query := qFunc.toQueryString(
					metricName,
					subQuery{window: model.Duration(mqp.SubQueryRange), step: 0},
					modifier(fmt.Sprintf(modifierAt, mqp.ModifierAt.UnixMilli())),
					newOffset(0),
				)

				res, err := queryRange(ctx, "none", s.quickQE, s, s.queryOpts, query, mqp.Start, mqp.End, mqp.Step)
				s.Require().NoError(err)
				defer res.qry.Close()

				resOpt, err := queryRange(ctx, "all", s.quickQE, s, s.queryOpts, query, mqp.Start, mqp.End, mqp.Step)
				s.Require().NoError(err)
				defer resOpt.qry.Close()

				return s.Equal(res.res, resOpt.res)
			}

			s.Require().NoError(quick.Check(f, nil))
		}
	}
}

func (s *QuickQuerierOptimizeSuite) TestQueryRangeQuickOffsetQueryParams() {
	ctx := s.T().Context()

	for _, qFunc := range s.queryFuncs {
		for _, metricName := range s.metricNames {
			f := func(oqp OffsetQueryParams) bool {
				query := qFunc.toQueryString(
					metricName,
					subQuery{window: model.Duration(oqp.SubQueryRange), step: 0},
					modifier(fmt.Sprintf(modifierAt, oqp.ModifierAt.UnixMilli())),
					newOffset(oqp.Offset),
				)

				res, err := queryRange(ctx, "none", s.quickQE, s, s.queryOpts, query, oqp.Start, oqp.End, oqp.Step)
				s.Require().NoError(err)
				defer res.qry.Close()

				resOpt, err := queryRange(ctx, "all", s.quickQE, s, s.queryOpts, query, oqp.Start, oqp.End, oqp.Step)
				s.Require().NoError(err)
				defer resOpt.qry.Close()

				return s.Equal(res.res, resOpt.res)
			}

			s.Require().NoError(quick.Check(f, nil))
		}
	}
}

func (s *QuickQuerierOptimizeSuite) TestQueryInstantQuickQueryParam() {
	ctx := s.T().Context()

	for _, qFunc := range s.queryFuncs {
		for _, metricName := range s.metricNames {
			f := func(qp QueryParam) bool {
				query := qFunc.toQueryString(
					metricName,
					subQuery{window: model.Duration(s.step * 4), step: 0},
					modifierNone,
					newOffset(0),
				)

				res, err := queryInstant(ctx, "none", s.quickQE, s, s.queryOpts, query, qp.Start)
				s.Require().NoError(err)
				defer res.qry.Close()

				resOpt, err := queryInstant(ctx, "all", s.quickQE, s, s.queryOpts, query, qp.Start)
				s.Require().NoError(err)
				defer resOpt.qry.Close()

				return s.Equal(res.res, resOpt.res)
			}

			s.Require().NoError(quick.Check(f, nil))
		}
	}
}

func (s *QuickQuerierOptimizeSuite) TestQueryInstantQuickSubQueryParams() {
	ctx := s.T().Context()

	for _, qFunc := range s.queryFuncs {
		for _, metricName := range s.metricNames {
			f := func(sqp SubQueryParams) bool {
				query := qFunc.toQueryString(
					metricName,
					subQuery{window: model.Duration(sqp.SubQueryRange), step: 0},
					modifierNone,
					newOffset(0),
				)

				res, err := queryInstant(ctx, "none", s.quickQE, s, s.queryOpts, query, sqp.Start)
				s.Require().NoError(err)
				defer res.qry.Close()

				resOpt, err := queryInstant(ctx, "all", s.quickQE, s, s.queryOpts, query, sqp.Start)
				s.Require().NoError(err)
				defer resOpt.qry.Close()

				return s.Equal(res.res, resOpt.res)
			}

			s.Require().NoError(quick.Check(f, nil))
		}
	}
}

func (s *QuickQuerierOptimizeSuite) TestQueryInstantQuickModifierQueryParams() {
	ctx := s.T().Context()

	for _, qFunc := range s.queryFuncs {
		for _, metricName := range s.metricNames {
			f := func(mqp ModifierQueryParams) bool {
				query := qFunc.toQueryString(
					metricName,
					subQuery{window: model.Duration(mqp.SubQueryRange), step: 0},
					modifier(fmt.Sprintf(modifierAt, mqp.ModifierAt.UnixMilli())),
					newOffset(0),
				)

				res, err := queryInstant(ctx, "none", s.quickQE, s, s.queryOpts, query, mqp.Start)
				s.Require().NoError(err)
				defer res.qry.Close()

				resOpt, err := queryInstant(ctx, "all", s.quickQE, s, s.queryOpts, query, mqp.Start)
				s.Require().NoError(err)
				defer resOpt.qry.Close()

				return s.Equal(res.res, resOpt.res)
			}

			s.Require().NoError(quick.Check(f, nil))
		}
	}
}

func (s *QuickQuerierOptimizeSuite) TestQueryInstantQuickOffsetQueryParams() {
	ctx := s.T().Context()

	for _, qFunc := range s.queryFuncs {
		for _, metricName := range s.metricNames {
			f := func(oqp OffsetQueryParams) bool {
				query := qFunc.toQueryString(
					metricName,
					subQuery{window: model.Duration(oqp.SubQueryRange), step: 0},
					modifier(fmt.Sprintf(modifierAt, oqp.ModifierAt.UnixMilli())),
					newOffset(oqp.Offset),
				)

				res, err := queryInstant(ctx, "none", s.quickQE, s, s.queryOpts, query, oqp.Start)
				s.Require().NoError(err)
				defer res.qry.Close()

				resOpt, err := queryInstant(ctx, "all", s.quickQE, s, s.queryOpts, query, oqp.Start)
				s.Require().NoError(err)
				defer resOpt.qry.Close()

				return s.Equal(res.res, resOpt.res)
			}

			s.Require().NoError(quick.Check(f, nil))
		}
	}
}
