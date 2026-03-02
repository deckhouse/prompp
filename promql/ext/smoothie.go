package promqlext

import (
	"math"

	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/annotations"
)

const (
	OpSmoothie = "op_smoothie"
)

// RegisterOPSmoothie registers promql `op_smoothie` function
func RegisterOPSmoothie() {
	parser.Functions[OpSmoothie] = &parser.Function{
		Name:       OpSmoothie,
		ArgTypes:   []parser.ValueType{parser.ValueTypeMatrix},
		ReturnType: parser.ValueTypeVector,
	}
	promql.FunctionCalls[OpSmoothie] = funcOPSmoothie
}

// === op_smoothie(Matrix parser.ValueTypeMatrix) Vector ===
// `op_smoothie` is a window function that replaces vector component value with the average one over the interval
func funcOPSmoothie(vals []parser.Value, args parser.Expressions, enh *promql.EvalNodeHelper) (promql.Vector, annotations.Annotations) {
	return aggrOverTime(vals, enh, func(values []promql.FPoint) float64 {
		var mean, count, c float64
		if len(values) == 0 {
			return math.NaN()
		}
		v := values[len(values)-1]
		if math.IsNaN(v.F) {
			return v.F
		}
		if enh.Ts != v.T {
			return math.Float64frombits(value.StaleNaN)
		}
		for _, v := range values {
			count++
			if math.IsInf(mean, 0) {
				if math.IsInf(v.F, 0) && (mean > 0) == (v.F > 0) {
					// The `mean` and `v.V` values are `Inf` of the same sign.  They
					// can't be subtracted, but the value of `mean` is correct
					// already.
					continue
				}
				if !math.IsInf(v.F, 0) && !math.IsNaN(v.F) {
					// At this stage, the mean is an infinite. If the added
					// value is neither an Inf or a Nan, we can keep that mean
					// value.
					// This is required because our calculation below removes
					// the mean value, which would look like Inf += x - Inf and
					// end up as a NaN.
					continue
				}
			}
			mean, c = kahanSumInc(v.F/count-mean/count, mean, c)
		}

		if math.IsInf(mean, 0) {
			return mean
		}
		return mean + c
	}), nil
}

func aggrOverTime(vals []parser.Value, enh *promql.EvalNodeHelper, aggrFn func([]promql.FPoint) float64) promql.Vector {
	el := vals[0].(promql.Matrix)[0]
	v := aggrFn(el.Floats)
	if value.IsStaleNaN(v) {
		return enh.Out
	}

	return append(enh.Out, promql.Sample{
		F: v,
	})
}

func kahanSumInc(inc, sum, c float64) (newSum, newC float64) {
	t := sum + inc
	// Using Neumaier improvement, swap if next term larger than sum.
	if math.Abs(sum) >= math.Abs(inc) {
		c += (sum - t) + inc
	} else {
		c += (inc - t) + sum
	}
	return t, c
}
