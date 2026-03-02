package promqlext

import (
	"math"

	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/annotations"
)

const (
	opReplaceNaN = "op_replace_nan"
)

// RegisterOPReplaceNaN registers promql `op_replace_nan` function
func RegisterOPReplaceNaN() {
	parser.Functions[opReplaceNaN] = &parser.Function{
		Name:       opReplaceNaN,
		ArgTypes:   []parser.ValueType{parser.ValueTypeMatrix, parser.ValueTypeScalar, parser.ValueTypeScalar},
		Variadic:   1,
		ReturnType: parser.ValueTypeVector,
	}
	promql.FunctionCalls[opReplaceNaN] = funcOPReplaceNaN
}

// === op_replace_nan(Matrix parser.ValueTypeMatrix, Value parser.ValueTypeScalar, Ms parser.ValueTypeScalar) Vector ===
// `op_replace_nan` is a window function that replaces vector component value with the second parameter `Value`
// if value is outdated (older than ActualInterval + 1 minute), third parameter is used to cut off values before now-`Ms` (milliseconds)
func funcOPReplaceNaN(vals []parser.Value, args parser.Expressions, enh *promql.EvalNodeHelper) (promql.Vector, annotations.Annotations) {
	vec := vals[0].(promql.Matrix)
	val := vals[1].(promql.Vector)[0].F
	var ms int64 = math.MinInt64
	if len(vals) > 2 {
		ms = int64(vals[2].(promql.Vector)[0].F)
	}
	for _, el := range vec {
		p := takeLast(el)
		if p.T < ms {
			continue
		}
		value := p.F
		if isNone(p, enh) || math.IsNaN(value) {
			value = val
		}
		enh.Out = append(enh.Out, promql.Sample{
			Metric: el.Metric.DropMetricName(),
			F:      value,
		})
	}
	return enh.Out, nil
}
