package scrape

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/model/labels"
)

func TestTargetSampleLimit(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit string
		want  int
	}{
		{name: "unset", limit: "", want: 0},
		{name: "positive", limit: "100", want: 100},
		{name: "explicit plus sign", limit: "+5", want: 5},
		{name: "zero", limit: "0", want: 0},
		{name: "negative", limit: "-1", want: 0},
		{name: "out of range", limit: "99999999999999999999", want: 0},
		{name: "not a number", limit: "many", want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target := NewTarget(labels.FromStrings("__sample_limit__", tc.limit), labels.EmptyLabels(), nil)

			require.Equal(t, tc.want, target.SampleLimit())
		})
	}
}
