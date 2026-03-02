package promqlext

import (
	"math"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/annotations"
)

const (
	opZeroIfNone = "op_zero_if_none"
)

// RegisterOPZeroIfNone registers promql `op_zero_if_none` function
func RegisterOPZeroIfNone() {
	parser.Functions[opZeroIfNone] = &parser.Function{
		Name:       opZeroIfNone,
		ArgTypes:   []parser.ValueType{parser.ValueTypeMatrix, parser.ValueTypeScalar},
		Variadic:   1,
		ReturnType: parser.ValueTypeVector,
	}
	promql.FunctionCalls[opZeroIfNone] = funcOPZeroIfNone
}

// === op_zero_if_none(Matrix parser.ValueTypeMatrix, Ms parser.ValueTypeScalar) Vector ===
// `op_zero_if_none` is a window function that replaces vector component value with 0 if empty
func funcOPZeroIfNone(vals []parser.Value, args parser.Expressions, enh *promql.EvalNodeHelper) (promql.Vector, annotations.Annotations) {
	vec := vals[0].(promql.Matrix)
	var ms int64 = math.MinInt64
	if len(vals) > 1 {
		ms = int64(vals[1].(promql.Vector)[0].F)
	}
	for _, el := range vec {
		p := takeLast(el)
		if p.T < ms {
			continue
		}
		value := p.F
		if isNone(p, enh) || math.IsNaN(value) {
			value = 0
		}
		enh.Out = append(enh.Out, promql.Sample{
			Metric: el.Metric.DropMetricName(),
			F:      value,
		})
	}
	return enh.Out, nil
}
