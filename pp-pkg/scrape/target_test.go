// Copyright The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package scrape

import (
	"testing"

	"github.com/prometheus/prometheus/model/labels"
)

// TestTargetSampleLimit covers the __sample_limit__ annotation (see
// PP_CHANGES.md). The label comes from relabelling, so any string can reach it;
// the negative case is a regression found by FuzzTargetsFromGroup, where a
// negative limit used to be handed to the C++ relabeler as an unsigned value
// and silenced the configured limit instead of being ignored.
func TestTargetSampleLimit(t *testing.T) {
	for _, tc := range []struct {
		name  string
		limit string
		want  int
	}{
		{name: "unset", limit: "", want: 0},
		{name: "positive", limit: "100", want: 100},
		{name: "zero", limit: "0", want: 0},
		{name: "negative", limit: "-1", want: 0},
		{name: "out of range", limit: "99999999999999999999", want: 0},
		{name: "not a number", limit: "many", want: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			builder := labels.NewBuilder(labels.EmptyLabels())
			if tc.limit != "" {
				builder.Set("__sample_limit__", tc.limit)
			}
			target := NewTarget(builder.Labels(), labels.EmptyLabels(), nil)

			if got := target.SampleLimit(); got != tc.want {
				t.Errorf("SampleLimit() with __sample_limit__=%q = %d, want %d", tc.limit, got, tc.want)
			}
		})
	}
}
