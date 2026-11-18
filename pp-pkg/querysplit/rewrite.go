package querysplit

import (
	"github.com/prometheus/prometheus/promql/parser"
)

// resolveStartEnd replaces @ start() and @ end() in expr with the absolute timestamps (ms)
// of the original query range, so that each sub-query keeps the meaning of the original query.
func resolveStartEnd(expr parser.Expr, start, end int64) {
	resolve := func(startOrEnd *parser.ItemType, ts **int64) {
		var t int64
		switch *startOrEnd {
		case parser.START:
			t = start
		case parser.END:
			t = end
		default:
			return
		}

		*ts = &t
		*startOrEnd = 0
	}

	parser.Inspect(expr, func(node parser.Node, _ []parser.Node) error {
		switch n := node.(type) {
		case *parser.VectorSelector:
			resolve(&n.StartOrEnd, &n.Timestamp)
		case *parser.SubqueryExpr:
			resolve(&n.StartOrEnd, &n.Timestamp)
		}
		return nil
	})
}
