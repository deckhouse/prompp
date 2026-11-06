package querysplit

import (
	"time"

	"github.com/prometheus/prometheus/promql/parser"
)

// lookbackWindow returns the longest time that one selector of expr reads before an evaluation step.
// Selectors with an @ modifier read a fixed time, so they do not add to the window.
func lookbackWindow(expr parser.Node, lookbackDelta time.Duration) time.Duration {
	switch n := expr.(type) {
	case *parser.VectorSelector:
		if n.Timestamp != nil || n.StartOrEnd != 0 {
			return 0
		}
		return lookbackDelta + max(n.OriginalOffset, 0)

	case *parser.MatrixSelector:
		vs, ok := n.VectorSelector.(*parser.VectorSelector)
		if !ok || vs.Timestamp != nil || vs.StartOrEnd != 0 {
			return 0
		}
		return n.Range + max(vs.OriginalOffset, 0)

	case *parser.SubqueryExpr:
		if n.Timestamp != nil || n.StartOrEnd != 0 {
			return 0
		}
		return n.Range + max(n.OriginalOffset, 0) + lookbackWindow(n.Expr, lookbackDelta)
	}

	var w time.Duration
	for _, child := range parser.Children(expr) {
		w = max(w, lookbackWindow(child, lookbackDelta))
	}
	return w
}

// hasVaryingAggregationParam reports whether an aggregation parameter in expr can change
// between steps. The engine evaluates the parameter only at the first step of the query range,
// so a split query would use other values of such a parameter.
func hasVaryingAggregationParam(expr parser.Expr) bool {
	var varying bool
	parser.Inspect(expr, func(node parser.Node, _ []parser.Node) error {
		if agg, ok := node.(*parser.AggregateExpr); ok && agg.Param != nil && !isConstant(agg.Param) {
			varying = true
		}
		return nil
	})
	return varying
}

// isConstant reports whether expr contains only literals.
func isConstant(expr parser.Node) bool {
	switch expr.(type) {
	case *parser.NumberLiteral, *parser.StringLiteral:
		return true
	case *parser.ParenExpr, *parser.UnaryExpr, *parser.BinaryExpr:
		for _, child := range parser.Children(expr) {
			if !isConstant(child) {
				return false
			}
		}
		return true
	}
	return false
}
