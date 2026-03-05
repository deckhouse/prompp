package promql

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

func init() {
	RegisterOPDefined()
	RegisterOPZeroIfNone()
	RegisterOPReplaceNaN()
	RegisterOPSmoothie()
}

// OP_FUNCTIONS

func isOPTop(expr *parser.AggregateExpr) bool {
	if expr == nil {
		return false
	}
	return expr.Op == parser.OP_TOP
}

func parseAggExpr(node parser.Node) *parser.AggregateExpr {
	n, ok := node.(*parser.AggregateExpr)
	if !ok {
		return nil
	}
	return n
}

// ExtractOptTop checks op_top if it is top level aggregate expr and returns new expression without op_top and result modifier.
func ExtractOptTop(expr parser.Expr, start, end, step int64) (parser.Expr, func(matrix Matrix) Matrix, error) {
	// return values
	var (
		opTopResultModifier func(matrix Matrix) Matrix
		internalQuery       parser.Expr
		err                 error
	)

	// auxiliary variables
	var (
		opFuncFound bool
	)

	parser.Inspect(expr, func(node parser.Node, path []parser.Node) error {
		aggExpr := parseAggExpr(node)
		if isOPTop(aggExpr) {
			if len(path) == 0 {
				opFuncFound = true
				internalQuery = aggExpr.Expr
				var optoperr error
				opTopResultModifier, optoperr = newOPTop(aggExpr, start, end, step)
				if optoperr != nil {
					err = optoperr
					return nil
				}
			} else {
				err = errors.New("invalid op_top placement in query")
			}
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	if !opFuncFound {
		return expr, nil, nil
	}

	return internalQuery, opTopResultModifier, nil
}

func newOPTop(node *parser.AggregateExpr, start, stop, step int64) (func(Matrix) Matrix, error) {
	var (
		weightFunc   weightFuncKind
		limit        int
		includeOther bool
	)
	// parse limit
	limit, _ = strconv.Atoi(node.Param.String())
	// parse op_top arguments
	switch len(node.Grouping) {
	case 0: // op_top($expr) or op_top($limit, $expr)
		// includeOther = false
		// weightFunc = weightFuncEws
	case 1: // op_top($includeOther, $expr) or op_top($limit, $includeOther, $expr)
		includeOther, _ = strconv.ParseBool(strings.ToLower(node.Grouping[0]))
		// weightFunc = weightFuncEws
	case 2: // op_top($includeOther, $weightFunc, $expr) or op_top($limit, $includeOther, $weightFunc, $expr)
		includeOther, _ = strconv.ParseBool(strings.ToLower(node.Grouping[0]))
		weightFunc = weightFuncKind(node.Grouping[1])
	}
	params := &OPTopQueryParams{
		weightFunc:   weightFunc,
		includeOther: includeOther,
		limit:        limit,
	}
	if limit <= 0 {
		return nil, fmt.Errorf("limit value must be set")
	}
	return func(initial Matrix) Matrix {
		return OPTop(params, initial, start, stop, step)
	}, nil
}

type OPTopQueryParams struct {
	weightFunc   weightFuncKind
	includeOther bool
	limit        int
}

// calculatePointsRequired returns required amount of data-points for current start, stop, and end
// ex:
//
//	step = 10
//	---------
//	[start=0]       1
//	[start+step=10] 2
//	[start+step=20] 3
//	[start+step=30] 4
//	[stop=40]       5
func calculatePointsRequired(start, stop, step int64) int {
	if step == 0 {
		return 1
	}
	req := (stop - start) / step
	return int(req) + 1
}

// calculatePointInd returns point index in series by given timestamp
// ex:
//
//	    timestamp = 30
//	    step      = 10
//		---------
//		[start=0]       1 ind=0
//		[start+step=10] 2 ind=1
//		[start+step=20] 3 ind=2
//		[start+step=30] 4 ind=3 <- at timestamp = 30
//		[stop=40]       5 ind=3
//
// so (30-0)/10 = 3.
func calculatePointInd(start, step, timestamp int64) int {
	if step == 0 {
		return 0
	}
	return int((timestamp - start) / step)
}

func markedAsOtherSeries(ls labels.Labels) bool {
	marked := false
	ls.Range(func(l labels.Label) {
		if l.Value == "~other" {
			marked = true
		}
	})
	return marked
}

type weightFuncKind string

const (
	weightFuncSum weightFuncKind = "sum"
	weightFuncMax weightFuncKind = "max"
	weightFuncExp weightFuncKind = "exp"
	weightFuncEws weightFuncKind = "ews"
	weightFuncEwm weightFuncKind = "ewn"
)

func nonEmptyValue(val float64) float64 {
	if math.IsNaN(val) || math.IsInf(val, 0) {
		return 0
	}
	return val
}

func weightBySum(samples []FPoint) (res float64) {
	for _, sample := range samples {
		res += nonEmptyValue(sample.F)
	}
	return
}

func weightByMax(samples []FPoint) (res float64) {
	for _, sample := range samples {
		if sample.F > res {
			res = nonEmptyValue(sample.F)
		}
	}
	return
}

const magicExp = 1.001 // avoid too high values from older logic

func weightByExp(samples []FPoint) (res float64) {
	for sampleInd := range samples {
		res += nonEmptyValue(samples[sampleInd].F) * math.Pow(magicExp, float64(sampleInd))
	}
	res = nonEmptyValue(res / float64(len(samples)))
	return
}

func weightByEws(samples []FPoint) (res float64) {
	for sampleInd := range samples {
		res += nonEmptyValue(samples[sampleInd].F) * math.Pow(magicExp, float64(sampleInd))
	}
	return
}

func weightByEwm(samples []FPoint) (res float64) {
	for sampleInd := range samples {
		val := nonEmptyValue(samples[sampleInd].F) * math.Pow(magicExp, float64(sampleInd))
		if val > res {
			res = val
		}
	}
	return
}

type member struct {
	index  int
	weight float64
}

type memberData struct {
	members []member
}

func (m memberData) Len() int {
	return len(m.members)
}

func (m memberData) Less(i, j int) bool {
	return m.members[i].weight < m.members[j].weight
}

func (m memberData) Swap(i, j int) {
	m.members[i], m.members[j] = m.members[j], m.members[i]
}

func OPTop(params *OPTopQueryParams, initial Matrix, start, stop, step int64) Matrix {
	// 0. ignore empty series collections
	if len(initial) == 0 {
		return initial
	}
	// 1. evaluate weight evaluation function
	var weightFunc func([]FPoint) float64
	switch params.weightFunc {
	case weightFuncSum:
		weightFunc = weightBySum
	case weightFuncMax:
		weightFunc = weightByMax
	case weightFuncExp:
		weightFunc = weightByExp
	case weightFuncEws:
		weightFunc = weightByEws
	case weightFuncEwm:
		weightFunc = weightByEwm
	default:
		weightFunc = weightByEws
	}
	// 2. collect array of pairs initial Ind: weightFunc(initial.samples)
	m := memberData{members: make([]member, 0, len(initial))}
	for streamInd := range initial {
		m.members = append(m.members, member{
			index:  streamInd,
			weight: weightFunc(initial[streamInd].Floats),
		})
	}
	// 3. recalculate the limit to avoid out of bounds panic
	limit := params.limit
	if limit <= 0 || len(m.members) < limit {
		// limit <= 0 : [invalid limit, limit hasn't been set, limit equal to zero] but request already made
		// len(m.members) < limit : we've received less time series than limit was set
		limit = len(m.members)
	}
	// 4. evaluate if we need to add our own ~other series
	otherSeriesRequired := params.includeOther && limit < len(m.members)
	// 5. set zero weight for initial ~other metric if otherSeriesRequired is true
	// it has to be done before we sort the time-series collection
	for memberInd := range m.members {
		initialLookupInd := m.members[memberInd].index
		if markedAsOtherSeries(initial[initialLookupInd].Metric) && otherSeriesRequired {
			m.members[memberInd].weight = 0
		}
	}
	// 6. sort the array in descending order
	sort.Sort(sort.Reverse(m))
	// 7. fill main K series using their indies from pt. 2
	res := make(Matrix, 0, limit+1)
	mainIndices := m.members[:limit]
	for _, index := range mainIndices {
		res = append(res, initial[index.index])
	}
	// 8. append (if required) "~other" series using indices left
	if otherSeriesRequired {
		auxiliaryIndices := m.members[limit:]
		otherSamplesCount := calculatePointsRequired(start, stop, step)
		// prepare otherSamples and otherLabels collection
		var (
			otherSamples   = make([]FPoint, otherSamplesCount)
			otherLabelsRaw = map[string]string{}
		)
		currentTimestamp := start
		for i := 0; i < otherSamplesCount; i++ {
			otherSamples[i].T = currentTimestamp
			currentTimestamp += step
		}
		// fill the prepared collections
		var labelsFilled bool
		for _, index := range auxiliaryIndices {
			// increment every datapoint in otherSamples
			for _, sample := range initial[index.index].Floats {
				otherSampleInd := calculatePointInd(start, step, sample.T)
				otherSamples[otherSampleInd].F += nonEmptyValue(sample.F)
			}
			// copy all the label values once
			if labelsFilled {
				continue
			}

			initial[index.index].Metric.Range(func(l labels.Label) {
				if l.Name == "__name__" {
					otherLabelsRaw[l.Name] = l.Value
				} else {
					otherLabelsRaw[l.Name] = "~other"
				}
			})
			labelsFilled = true
		}
		// create and append other series into response

		lsb := labels.NewScratchBuilder(len(otherLabelsRaw))
		for label, labelValue := range otherLabelsRaw {
			lsb.Add(label, labelValue)
		}

		otherSeries := Series{
			Metric: lsb.Labels(),
			Floats: otherSamples,
		}

		res = append(res, otherSeries)
	}
	return res
}
