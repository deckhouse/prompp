// Copyright 2026 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Tests in this file run the EXACT alert expression from
//   debug/run/rules/control-plane-manager.yaml
// (D8ControlPlaneManagerPodNotRunning)
// against synthetic timeseries shaped like real kube-state-metrics output,
// to lock down what conditions cause the alert to fire vs. stay silent.
//
// We use the upstream PromQL test storage so this file does NOT depend on
// cppbridge or the prompp-specific querier — it isolates the expression
// semantics layer.

package rules

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/prometheus/promql/promqltest"
	"github.com/stretchr/testify/require"
)

const cpmExpr = `max by (node) (kube_node_role{role="master"} unless kube_node_role{role="master"}` +
	` * on (node) group_left () ((kube_pod_status_ready{condition="true"} == 1) *` +
	` on (pod, namespace) group_right () kube_controller_pod{controller_name="d8-control-plane-manager",controller_type="DaemonSet",namespace="kube-system"}))`

// TestCPM_HappyPath_NoFiring: all 3 masters have a healthy DaemonSet pod, all
// labels match, the `unless` cancels everything → no alert series.
func TestCPM_HappyPath_NoFiring(t *testing.T) {
	storage := promqltest.LoadedStorage(t, `
load 1m
  kube_node_role{role="master",node="m0",instance="ksm",job="kube-state-metrics"} 1+0x10
  kube_node_role{role="master",node="m1",instance="ksm",job="kube-state-metrics"} 1+0x10
  kube_node_role{role="master",node="m2",instance="ksm",job="kube-state-metrics"} 1+0x10
  kube_pod_status_ready{condition="true",namespace="kube-system",pod="cpm-a",instance="ksm",job="kube-state-metrics"} 1+0x10
  kube_pod_status_ready{condition="true",namespace="kube-system",pod="cpm-b",instance="ksm",job="kube-state-metrics"} 1+0x10
  kube_pod_status_ready{condition="true",namespace="kube-system",pod="cpm-c",instance="ksm",job="kube-state-metrics"} 1+0x10
  kube_controller_pod{controller_name="d8-control-plane-manager",controller_type="DaemonSet",namespace="kube-system",node="m0",pod="cpm-a",job="kube-state-metrics"} 1+0x10
  kube_controller_pod{controller_name="d8-control-plane-manager",controller_type="DaemonSet",namespace="kube-system",node="m1",pod="cpm-b",job="kube-state-metrics"} 1+0x10
  kube_controller_pod{controller_name="d8-control-plane-manager",controller_type="DaemonSet",namespace="kube-system",node="m2",pod="cpm-c",job="kube-state-metrics"} 1+0x10
`)
	t.Cleanup(func() { _ = storage.Close() })

	ng := testEngine(t)
	q := EngineQueryFunc(ng, storage)
	res, err := q(context.TODO(), cpmExpr, time.Unix(5*60, 0)) // t=5m
	require.NoError(t, err)
	require.Empty(t, res, "alert expr must be empty when all masters have healthy pods — got %v", res)
}

// TestCPM_PodGoneForOneMaster_AlertsForThatMaster: kube_pod_status_ready for
// master-1's pod stops being scraped. Within lookback the right-hand side of
// `unless` loses master-1 → master-1 escapes → expr returns one series.
//
// This is the canonical "cause" of the alert firing: a missing pod_status_ready
// readout for one of the masters.
func TestCPM_PodGoneForOneMaster_AlertsForThatMaster(t *testing.T) {
	// We deliberately stop kube_pod_status_ready for cpm-b (master-1) at minute 3
	// (load 1m → 4 points 0,1,2,3). Default lookback is 5m, so at t=10m the
	// querier would still return the last point from t=3m as a stale value —
	// we therefore evaluate at t=12m, which is well outside the 5m lookback.
	storage := promqltest.LoadedStorage(t, `
load 1m
  kube_node_role{role="master",node="m0",instance="ksm",job="kube-state-metrics"} 1+0x20
  kube_node_role{role="master",node="m1",instance="ksm",job="kube-state-metrics"} 1+0x20
  kube_node_role{role="master",node="m2",instance="ksm",job="kube-state-metrics"} 1+0x20
  kube_pod_status_ready{condition="true",namespace="kube-system",pod="cpm-a",instance="ksm",job="kube-state-metrics"} 1+0x20
  kube_pod_status_ready{condition="true",namespace="kube-system",pod="cpm-b",instance="ksm",job="kube-state-metrics"} 1 1 1 1
  kube_pod_status_ready{condition="true",namespace="kube-system",pod="cpm-c",instance="ksm",job="kube-state-metrics"} 1+0x20
  kube_controller_pod{controller_name="d8-control-plane-manager",controller_type="DaemonSet",namespace="kube-system",node="m0",pod="cpm-a",job="kube-state-metrics"} 1+0x20
  kube_controller_pod{controller_name="d8-control-plane-manager",controller_type="DaemonSet",namespace="kube-system",node="m1",pod="cpm-b",job="kube-state-metrics"} 1+0x20
  kube_controller_pod{controller_name="d8-control-plane-manager",controller_type="DaemonSet",namespace="kube-system",node="m2",pod="cpm-c",job="kube-state-metrics"} 1+0x20
`)
	t.Cleanup(func() { _ = storage.Close() })

	ng := testEngine(t)
	q := EngineQueryFunc(ng, storage)
	res, err := q(context.TODO(), cpmExpr, time.Unix(12*60, 0))
	require.NoError(t, err)
	require.Len(t, res, 1, "exactly one master must escape the unless")
	require.Equal(t, "m1", res[0].Metric.Get("node"))
}

// TestCPM_RecoveryClearsAlert: same as the previous test, but the missing
// kube_pod_status_ready readout reappears at minute 13. By minute 14 the
// expression must again return empty (the alert would transition Inactive in
// AlertingRule.Eval).
//
// This test pins down: "as soon as the data comes back, the expression must
// stop returning the master". If this fails, we have a real engine/data bug.
func TestCPM_RecoveryClearsAlert(t *testing.T) {
	storage := promqltest.LoadedStorage(t, `
load 1m
  kube_node_role{role="master",node="m0",instance="ksm",job="kube-state-metrics"} 1+0x20
  kube_node_role{role="master",node="m1",instance="ksm",job="kube-state-metrics"} 1+0x20
  kube_node_role{role="master",node="m2",instance="ksm",job="kube-state-metrics"} 1+0x20
  kube_pod_status_ready{condition="true",namespace="kube-system",pod="cpm-a",instance="ksm",job="kube-state-metrics"} 1+0x20
  kube_pod_status_ready{condition="true",namespace="kube-system",pod="cpm-b",instance="ksm",job="kube-state-metrics"} 1 1 1 1 _ _ _ _ _ _ _ _ _ _ 1 1 1 1 1 1 1
  kube_pod_status_ready{condition="true",namespace="kube-system",pod="cpm-c",instance="ksm",job="kube-state-metrics"} 1+0x20
  kube_controller_pod{controller_name="d8-control-plane-manager",controller_type="DaemonSet",namespace="kube-system",node="m0",pod="cpm-a",job="kube-state-metrics"} 1+0x20
  kube_controller_pod{controller_name="d8-control-plane-manager",controller_type="DaemonSet",namespace="kube-system",node="m1",pod="cpm-b",job="kube-state-metrics"} 1+0x20
  kube_controller_pod{controller_name="d8-control-plane-manager",controller_type="DaemonSet",namespace="kube-system",node="m2",pod="cpm-c",job="kube-state-metrics"} 1+0x20
`)
	t.Cleanup(func() { _ = storage.Close() })

	ng := testEngine(t)
	q := EngineQueryFunc(ng, storage)

	// While the gap is open (t=10m), we expect 1 alerting series.
	res, err := q(context.TODO(), cpmExpr, time.Unix(10*60, 0))
	require.NoError(t, err)
	require.Len(t, res, 1, "expected 1 alerting series during the gap")

	// After recovery (t=16m, beyond default 5m lookback past the recovered point at 14m),
	// the expression MUST be empty again.
	res, err = q(context.TODO(), cpmExpr, time.Unix(16*60, 0))
	require.NoError(t, err)
	require.Empty(t, res, "after recovery the expression MUST be empty — got %v", res)
}

// TestCPM_PodReplaced_OldFingerprintMustNotLinger: simulates the production
// scenario where a DaemonSet pod is recreated (UID/pod-name change). The OLD
// pod stops being scraped and the NEW one starts scraping with a different
// `pod` label. The expression has no business reporting either pod after the
// 5m lookback — but if the old `pod` label lingers in storage and the engine
// still sees it via Select(), the join becomes inconsistent and triggers the
// alert.
//
// This pins down: pod replacement WITHOUT a real outage must not fire the alert.
func TestCPM_PodReplaced_OldFingerprintMustNotLinger(t *testing.T) {
	storage := promqltest.LoadedStorage(t, `
load 1m
  kube_node_role{role="master",node="m0",instance="ksm",job="kube-state-metrics"} 1+0x30
  kube_node_role{role="master",node="m1",instance="ksm",job="kube-state-metrics"} 1+0x30
  kube_node_role{role="master",node="m2",instance="ksm",job="kube-state-metrics"} 1+0x30
  kube_pod_status_ready{condition="true",namespace="kube-system",pod="cpm-a-old",instance="ksm",job="kube-state-metrics"} 1 1 1 1 1
  kube_pod_status_ready{condition="true",namespace="kube-system",pod="cpm-b-old",instance="ksm",job="kube-state-metrics"} 1 1 1 1 1
  kube_pod_status_ready{condition="true",namespace="kube-system",pod="cpm-c-old",instance="ksm",job="kube-state-metrics"} 1 1 1 1 1
  kube_controller_pod{controller_name="d8-control-plane-manager",controller_type="DaemonSet",namespace="kube-system",node="m0",pod="cpm-a-old",job="kube-state-metrics"} 1 1 1 1 1
  kube_controller_pod{controller_name="d8-control-plane-manager",controller_type="DaemonSet",namespace="kube-system",node="m1",pod="cpm-b-old",job="kube-state-metrics"} 1 1 1 1 1
  kube_controller_pod{controller_name="d8-control-plane-manager",controller_type="DaemonSet",namespace="kube-system",node="m2",pod="cpm-c-old",job="kube-state-metrics"} 1 1 1 1 1
  kube_pod_status_ready{condition="true",namespace="kube-system",pod="cpm-a-new",instance="ksm",job="kube-state-metrics"} _ _ _ _ _ 1+0x25
  kube_pod_status_ready{condition="true",namespace="kube-system",pod="cpm-b-new",instance="ksm",job="kube-state-metrics"} _ _ _ _ _ 1+0x25
  kube_pod_status_ready{condition="true",namespace="kube-system",pod="cpm-c-new",instance="ksm",job="kube-state-metrics"} _ _ _ _ _ 1+0x25
  kube_controller_pod{controller_name="d8-control-plane-manager",controller_type="DaemonSet",namespace="kube-system",node="m0",pod="cpm-a-new",job="kube-state-metrics"} _ _ _ _ _ 1+0x25
  kube_controller_pod{controller_name="d8-control-plane-manager",controller_type="DaemonSet",namespace="kube-system",node="m1",pod="cpm-b-new",job="kube-state-metrics"} _ _ _ _ _ 1+0x25
  kube_controller_pod{controller_name="d8-control-plane-manager",controller_type="DaemonSet",namespace="kube-system",node="m2",pod="cpm-c-new",job="kube-state-metrics"} _ _ _ _ _ 1+0x25
`)
	t.Cleanup(func() { _ = storage.Close() })

	ng := testEngine(t)
	q := EngineQueryFunc(ng, storage)

	// Well after replacement and well outside lookback for the OLD generation:
	res, err := q(context.TODO(), cpmExpr, time.Unix(20*60, 0))
	require.NoError(t, err)
	require.Empty(t, res, "after pod replacement settled, the alert MUST stay silent — got %v", res)
}
