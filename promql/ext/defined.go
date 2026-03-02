package promqlext

import (
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/prometheus/prometheus/util/annotations"
)

const (
	opDefined = "op_defined"
)

// RegisterOPDefined registers promql `op_defined` function
func RegisterOPDefined() {
	parser.Functions[opDefined] = &parser.Function{
		Name:       opDefined,
		ArgTypes:   []parser.ValueType{parser.ValueTypeMatrix},
		ReturnType: parser.ValueTypeVector,
	}
	promql.FunctionCalls[opDefined] = funcOPDefined
}

// === op_defined(Matrix parser.ValueTypeMatrix) Vector ===
// `op_defined` is a window function that replaces vector component value with 0
// (if value is outdated e.q older than ActualInterval + 1 minute) or 1 otherwise
func funcOPDefined(vals []parser.Value, args parser.Expressions, enh *promql.EvalNodeHelper) (promql.Vector, annotations.Annotations) {
	vec := vals[0].(promql.Matrix)
	for _, el := range vec {
		var value float64 = 1
		if isNone(takeLast(el), enh) {
			value = 0
		}
		enh.Out = append(enh.Out, promql.Sample{
			Metric: el.Metric.DropMetricName(),
			F:      value,
		})
	}
	return enh.Out, nil
}
