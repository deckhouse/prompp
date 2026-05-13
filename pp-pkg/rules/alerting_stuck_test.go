// Copyright 2026 The Prometheus Authors
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
//
// Tests in this file investigate the "stuck D8ControlPlaneManagerPodNotRunning"
// alert behavior, where an alert keeps firing for hours even after the source
// data clearly indicates it should be inactive.
//
// Each test pins down ONE behavioral assumption underlying our diagnosis.

package rules

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/promql/parser"
	"github.com/stretchr/testify/require"
)

// scriptedQuery returns a QueryFunc that hands out canned promql.Vector values
// from a script. The i-th call returns script[i]; the timestamp/identity is
// tagged onto each Sample's T so the rule sees consistent evaluation time.
func scriptedQuery(script []promql.Vector) (QueryFunc, func() int) {
	var (
		mu sync.Mutex
		i  int
	)
	q := func(_ context.Context, _ string, t time.Time) (promql.Vector, error) {
		mu.Lock()
		defer mu.Unlock()
		if i >= len(script) {
			panic("scriptedQuery exhausted")
		}
		out := make(promql.Vector, len(script[i]))
		for j, s := range script[i] {
			s.T = t.UnixMilli()
			out[j] = s
		}
		i++
		return out, nil
	}
	return q, func() int {
		mu.Lock()
		defer mu.Unlock()
		return i
	}
}

func mustExpr(t *testing.T, src string) parser.Expr {
	t.Helper()
	e, err := parser.ParseExpr(src)
	require.NoError(t, err)
	return e
}

// TestAlertingRule_OnePassEmpty_TransitionsToInactive verifies that ONE
// evaluation with an empty query result is enough to move a Firing alert into
// the Inactive state. This is the baseline guarantee for "an alert cannot be
// stuck if the query returns nothing".
func TestAlertingRule_OnePassEmpty_TransitionsToInactive(t *testing.T) {
	rule := NewAlertingRule(
		"X", mustExpr(t, `up == 0`),
		time.Minute, // holdDuration
		0,           // keepFiringFor
		labels.EmptyLabels(), labels.EmptyLabels(), labels.EmptyLabels(), "",
		true, nil, // restored=true → ALERTS series will be emitted
	)

	hot := promql.Vector{{
		Metric: labels.FromStrings("__name__", "up", "job", "x"),
		F:      0,
	}}
	q, _ := scriptedQuery([]promql.Vector{
		hot,         // t=0  : pending starts (new entry in r.active)
		hot,         // t=1m : transitions to firing (holdDuration met)
		nil,         // t=2m : empty → must transition to Inactive
	})

	t0 := time.Unix(0, 0).UTC()
	for step := 0; step < 3; step++ {
		ts := t0.Add(time.Duration(step) * time.Minute)
		_, err := rule.Eval(context.TODO(), 0, ts, q, nil, 0)
		require.NoError(t, err)
	}

	require.Len(t, rule.active, 1, "alert entry must still be in r.active (Inactive but kept for resolvedRetention)")
	for _, a := range rule.active {
		require.Equal(t, StateInactive, a.State,
			"after a single empty Eval, an active alert MUST be Inactive — got %s", a.State)
		require.False(t, a.ResolvedAt.IsZero(), "ResolvedAt must be set on transition to Inactive")
	}
}

// TestAlertingRule_StaysFiringWhileQueryReturnsSameFingerprint pins down the
// other side of the contract: as long as the query returns a sample with the
// same fingerprint, the alert keeps firing indefinitely. This is exactly the
// behaviour we observe in production — so the only way a stuck alert can exist
// is if query() consistently returns a non-empty result.
func TestAlertingRule_StaysFiringWhileQueryReturnsSameFingerprint(t *testing.T) {
	rule := NewAlertingRule(
		"X", mustExpr(t, `up == 0`),
		time.Minute, 0,
		labels.EmptyLabels(), labels.EmptyLabels(), labels.EmptyLabels(), "",
		true, nil,
	)

	hot := promql.Vector{{
		Metric: labels.FromStrings("__name__", "up", "job", "x"),
		F:      0,
	}}
	// 100 consecutive non-empty evaluations: the alert MUST stay firing the entire time.
	const evals = 100
	script := make([]promql.Vector, evals)
	for i := range script {
		script[i] = hot
	}
	q, _ := scriptedQuery(script)

	t0 := time.Unix(0, 0).UTC()
	for step := 0; step < evals; step++ {
		ts := t0.Add(time.Duration(step) * time.Minute)
		_, err := rule.Eval(context.TODO(), 0, ts, q, nil, 0)
		require.NoError(t, err)
	}

	require.Len(t, rule.active, 1)
	for _, a := range rule.active {
		require.Equal(t, StateFiring, a.State,
			"alert must stay Firing while query keeps returning the same fingerprint")
	}
}

// TestAlertingRule_DroppedFromActiveAfterResolvedRetention verifies the
// 15-minute resolvedRetention window: once the alert is Inactive AND
// resolvedRetention is exceeded, it must be deleted from r.active.
//
// (This eliminates the hypothesis that some glitch could leave a "stale entry"
// sitting in r.active forever — by design, the entry is GC'd after 15min idle.)
func TestAlertingRule_DroppedFromActiveAfterResolvedRetention(t *testing.T) {
	rule := NewAlertingRule(
		"X", mustExpr(t, `up == 0`),
		time.Minute, 0,
		labels.EmptyLabels(), labels.EmptyLabels(), labels.EmptyLabels(), "",
		true, nil,
	)

	hot := promql.Vector{{
		Metric: labels.FromStrings("__name__", "up", "job", "x"),
		F:      0,
	}}
	// pending(0m), firing(1m), then 20 empty evals one minute apart.
	script := []promql.Vector{hot, hot}
	for i := 0; i < 20; i++ {
		script = append(script, nil)
	}
	q, _ := scriptedQuery(script)

	t0 := time.Unix(0, 0).UTC()
	for step := 0; step < len(script); step++ {
		ts := t0.Add(time.Duration(step) * time.Minute)
		_, err := rule.Eval(context.TODO(), 0, ts, q, nil, 0)
		require.NoError(t, err)
	}

	require.Empty(t, rule.active,
		"after >resolvedRetention(15min) of empty evaluations, the alert MUST be removed from r.active")
}
