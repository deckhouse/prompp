// Copyright 2026 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// This file pins down PromQL semantics over a real prompp Adapter (cppbridge
// head + querier). It mirrors the four CPM scenarios from
// pp-pkg/rules/control_plane_expr_test.go that have been validated against
// the upstream test storage.
//
// If a scenario behaves differently here than against upstream storage we have
// pinpointed a bug in the prompp querier path.

package storage

import (
	"testing"
	"time"

	"github.com/go-kit/log"
	"github.com/jonboulle/clockwork"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/suite"

	pp_model "github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/storage"
)

const cpmExpr = `max by (node) (kube_node_role{role="master"} unless kube_node_role{role="master"}` +
	` * on (node) group_left () ((kube_pod_status_ready{condition="true"} == 1) *` +
	` on (pod, namespace) group_right () kube_controller_pod{controller_name="d8-control-plane-manager",controller_type="DaemonSet",namespace="kube-system"}))`

// scrapePoint represents a single (labels, ts, value) tuple to ingest.
type scrapePoint struct {
	labels []string // key1, val1, key2, val2, ...
	tMs    int64
	v      float64
}

// AdapterPromQLSuite reuses the boilerplate from BatchStorageSuite to set up a
// real Adapter, then drives PromQL queries against it through the promql.Engine.
type AdapterPromQLSuite struct {
	BatchStorageSuite
}

func TestAdapterPromQLSuite(t *testing.T) {
	suite.Run(t, new(AdapterPromQLSuite))
}

// engine returns a PromQL engine configured the same way it is in production
// (lookback-delta=1m as in the affected cluster's prompp).
func (s *AdapterPromQLSuite) engine() *promql.Engine {
	return promql.NewEngine(promql.EngineOpts{
		Logger:                   log.NewNopLogger(),
		Reg:                      nil,
		MaxSamples:               100_000,
		Timeout:                  10 * time.Second,
		LookbackDelta:            1 * time.Minute,
		NoStepSubqueryIntervalFn: func(int64) int64 { return 60 * 1000 },
		EnableAtModifier:         true,
		EnableNegativeOffset:     true,
	})
}

// ingest appends the supplied scrape points to the active head.
//
// Implementation note: prompp's Adapter is shard-aware, but writing through
// AppendTimeSeries with a freshly built batch lets the Adapter do the routing.
// We re-use the global StateV2 just like a real scrape would.
func (s *AdapterPromQLSuite) ingest(points []scrapePoint) {
	batch := &testTimeSeriesBatch{
		timeSeries: make([]pp_model.TimeSeries, 0, len(points)),
	}
	for _, p := range points {
		b := pp_model.NewLabelSetBuilder()
		for i := 0; i < len(p.labels); i += 2 {
			b.Set(p.labels[i], p.labels[i+1])
		}
		batch.timeSeries = append(batch.timeSeries, pp_model.TimeSeries{
			LabelSet:  b.Build(),
			Timestamp: uint64(p.tMs), // #nosec G115
			Value:     p.v,
		})
	}
	_, err := s.adapter.AppendTimeSeries(s.ctx, batch, s.state, false)
	s.Require().NoError(err)
}

// queryAt runs an instant PromQL query at the given wall-clock timestamp and
// returns the resulting Vector. We use the same plumbing the rule manager uses.
func (s *AdapterPromQLSuite) queryAt(expr string, ts time.Time) promql.Vector {
	q, err := s.engine().NewInstantQuery(s.ctx, s.adapter, nil, expr, ts)
	s.Require().NoError(err)
	defer q.Close()

	res := q.Exec(s.ctx)
	s.Require().NoError(res.Err)

	v, err := res.Vector()
	s.Require().NoError(err)
	return v
}

// SetupTest extends the parent setup with a real (non-fake) clock. The fake
// clock from BatchStorageSuite breaks the engine's deadline plumbing.
func (s *AdapterPromQLSuite) SetupTest() {
	s.BatchStorageSuite.SetupTest()
	s.clock = clockwork.NewRealClock()
}

// TestCPM_HappyPath_NoFiring_PromppStorage mirrors
// pp-pkg/rules/control_plane_expr_test.go::TestCPM_HappyPath_NoFiring but
// against the prompp Adapter. If this passes we know the prompp querier
// returns the same shape of data as the upstream test storage for the
// fully-healthy case.
func (s *AdapterPromQLSuite) TestCPM_HappyPath_NoFiring_PromppStorage() {
	const stepMs = 60_000 // 1 minute
	var points []scrapePoint

	for step := int64(0); step < 11; step++ {
		ts := step * stepMs
		// kube_node_role
		for _, node := range []string{"m0", "m1", "m2"} {
			points = append(points, scrapePoint{
				labels: []string{
					"__name__", "kube_node_role",
					"role", "master",
					"node", node,
					"instance", "ksm",
					"job", "kube-state-metrics",
				},
				tMs: ts, v: 1,
			})
		}
		// kube_pod_status_ready
		for _, pod := range []string{"cpm-a", "cpm-b", "cpm-c"} {
			points = append(points, scrapePoint{
				labels: []string{
					"__name__", "kube_pod_status_ready",
					"condition", "true",
					"namespace", "kube-system",
					"pod", pod,
					"instance", "ksm",
					"job", "kube-state-metrics",
				},
				tMs: ts, v: 1,
			})
		}
		// kube_controller_pod
		for _, pair := range [][2]string{{"m0", "cpm-a"}, {"m1", "cpm-b"}, {"m2", "cpm-c"}} {
			points = append(points, scrapePoint{
				labels: []string{
					"__name__", "kube_controller_pod",
					"controller_name", "d8-control-plane-manager",
					"controller_type", "DaemonSet",
					"namespace", "kube-system",
					"node", pair[0],
					"pod", pair[1],
					"job", "kube-state-metrics",
				},
				tMs: ts, v: 1,
			})
		}
	}
	s.ingest(points)

	// Sanity probe: kube_node_role at t=5m must return 3 series.
	probe := s.queryAt(`kube_node_role{role="master"}`, model.Time(5*60_000).Time())
	s.Require().Len(probe, 3, "sanity: expected 3 master roles at t=5m, got %v", probe)

	// Real assertion: the alert expression must be empty.
	res := s.queryAt(cpmExpr, model.Time(5*60_000).Time())
	s.Require().Empty(res, "alert expr must be empty when all masters have healthy pods — got %v", res)
}

// TestCPM_PodGoneForOneMaster_PromppStorage mirrors the upstream test that
// stops scraping master-1's kube_pod_status_ready: outside the 1m lookback
// past the last point, master-1 must escape the unless and the alert must fire
// for exactly one master.
func (s *AdapterPromQLSuite) TestCPM_PodGoneForOneMaster_PromppStorage() {
	const stepMs = 60_000
	var points []scrapePoint

	// 21 minutes of data for everything that doesn't disappear.
	for step := int64(0); step < 21; step++ {
		ts := step * stepMs
		for _, node := range []string{"m0", "m1", "m2"} {
			points = append(points, scrapePoint{
				labels: []string{
					"__name__", "kube_node_role",
					"role", "master",
					"node", node,
					"instance", "ksm",
					"job", "kube-state-metrics",
				},
				tMs: ts, v: 1,
			})
		}
		// kube_pod_status_ready: cpm-a and cpm-c always; cpm-b only for first 4 minutes.
		for _, pod := range []string{"cpm-a", "cpm-c"} {
			points = append(points, scrapePoint{
				labels: []string{
					"__name__", "kube_pod_status_ready",
					"condition", "true",
					"namespace", "kube-system",
					"pod", pod,
					"instance", "ksm",
					"job", "kube-state-metrics",
				},
				tMs: ts, v: 1,
			})
		}
		if step < 4 {
			points = append(points, scrapePoint{
				labels: []string{
					"__name__", "kube_pod_status_ready",
					"condition", "true",
					"namespace", "kube-system",
					"pod", "cpm-b",
					"instance", "ksm",
					"job", "kube-state-metrics",
				},
				tMs: ts, v: 1,
			})
		}
		for _, pair := range [][2]string{{"m0", "cpm-a"}, {"m1", "cpm-b"}, {"m2", "cpm-c"}} {
			points = append(points, scrapePoint{
				labels: []string{
					"__name__", "kube_controller_pod",
					"controller_name", "d8-control-plane-manager",
					"controller_type", "DaemonSet",
					"namespace", "kube-system",
					"node", pair[0],
					"pod", pair[1],
					"job", "kube-state-metrics",
				},
				tMs: ts, v: 1,
			})
		}
	}
	s.ingest(points)

	res := s.queryAt(cpmExpr, model.Time(12*60_000).Time())
	s.Require().Len(res, 1, "exactly one master must escape the unless")
	s.Require().Equal("m1", res[0].Metric.Get("node"))
}

// TestCPM_PodReplaced_PromppStorage exercises the production-like scenario
// where the OLD pod stops being scraped at minute 5 and the NEW pod starts at
// minute 5 with a different `pod` label. After the lookback past the
// transition, the alert MUST stay silent — if it fires, we have a bug in the
// prompp querier (e.g. the old `pod` lingers in the LSS).
func (s *AdapterPromQLSuite) TestCPM_PodReplaced_PromppStorage() {
	const stepMs = 60_000
	var points []scrapePoint

	for step := int64(0); step < 30; step++ {
		ts := step * stepMs
		for _, node := range []string{"m0", "m1", "m2"} {
			points = append(points, scrapePoint{
				labels: []string{
					"__name__", "kube_node_role",
					"role", "master",
					"node", node,
					"instance", "ksm",
					"job", "kube-state-metrics",
				},
				tMs: ts, v: 1,
			})
		}

		// OLD generation: present only for steps 0..4 (5 minutes).
		if step < 5 {
			for _, oldPair := range [][2]string{{"m0", "cpm-a-old"}, {"m1", "cpm-b-old"}, {"m2", "cpm-c-old"}} {
				points = append(points, scrapePoint{
					labels: []string{
						"__name__", "kube_pod_status_ready",
						"condition", "true",
						"namespace", "kube-system",
						"pod", oldPair[1],
						"instance", "ksm",
						"job", "kube-state-metrics",
					},
					tMs: ts, v: 1,
				})
				points = append(points, scrapePoint{
					labels: []string{
						"__name__", "kube_controller_pod",
						"controller_name", "d8-control-plane-manager",
						"controller_type", "DaemonSet",
						"namespace", "kube-system",
						"node", oldPair[0],
						"pod", oldPair[1],
						"job", "kube-state-metrics",
					},
					tMs: ts, v: 1,
				})
			}
		}

		// NEW generation: present from step 5 onwards.
		if step >= 5 {
			for _, newPair := range [][2]string{{"m0", "cpm-a-new"}, {"m1", "cpm-b-new"}, {"m2", "cpm-c-new"}} {
				points = append(points, scrapePoint{
					labels: []string{
						"__name__", "kube_pod_status_ready",
						"condition", "true",
						"namespace", "kube-system",
						"pod", newPair[1],
						"instance", "ksm",
						"job", "kube-state-metrics",
					},
					tMs: ts, v: 1,
				})
				points = append(points, scrapePoint{
					labels: []string{
						"__name__", "kube_controller_pod",
						"controller_name", "d8-control-plane-manager",
						"controller_type", "DaemonSet",
						"namespace", "kube-system",
						"node", newPair[0],
						"pod", newPair[1],
						"job", "kube-state-metrics",
					},
					tMs: ts, v: 1,
				})
			}
		}
	}
	s.ingest(points)

	// Way past the transition (t=20m), well outside the 1m lookback:
	res := s.queryAt(cpmExpr, model.Time(20*60_000).Time())
	s.Require().Empty(res, "after pod replacement settled, the alert MUST stay silent — got %v", res)
}

// rotateHead manually replays what services/rotator.go does on a rotation tick:
// move the current active head into the keeper and install a fresh new active
// head built off the next generation. This lets us test multi-head behavior
// (active + keeper'ed old heads) without spinning the real Rotator.
func (s *AdapterPromQLSuite) rotateHead() {
	oldHead := s.manager.Proxy().Get()
	newHead, err := s.manager.Builder().Build(oldHead.Generation()+1, 2)
	s.Require().NoError(err)

	// Mirror rotator.rotate: keep the old head accessible via Proxy.Heads().
	s.Require().NoError(s.manager.Proxy().AddWithReplace(oldHead, 0))
	s.Require().NoError(s.manager.Proxy().Replace(s.ctx, newHead))
	oldHead.SetReadOnly()
}

// TestCPM_PodReplaced_AcrossRotation_PromppStorage is the most production-like
// scenario for the stuck D8ControlPlaneManagerPodNotRunning alert:
//
//  1. Initial pods (cpm-X-old) are scraped into HEAD-A. HEAD-A becomes the
//     keeper'ed read-only head after rotation.
//  2. A rotation occurs (mimicking the periodic rotator tick).
//  3. The DaemonSet controller recreates pods (cpm-X-new) and KSM exposes the
//     new pod labels. These new series are scraped into the fresh HEAD-B
//     (active).
//
// At a query time well past the rotation and well past the 1m lookback, the
// alert expression MUST stay silent — the OLD pods' last sample is more than
// 1m in the past and therefore fall outside lookback. If the unless join
// becomes inconsistent and the alert fires, we have reproduced the prod bug.
func (s *AdapterPromQLSuite) TestCPM_PodReplaced_AcrossRotation_PromppStorage() {
	const stepMs = 60_000

	// Phase 1: write OLD generation into HEAD-A (steps 0..4).
	{
		var points []scrapePoint
		for step := int64(0); step < 5; step++ {
			ts := step * stepMs
			for _, node := range []string{"m0", "m1", "m2"} {
				points = append(points, scrapePoint{
					labels: []string{
						"__name__", "kube_node_role",
						"role", "master",
						"node", node,
						"instance", "ksm",
						"job", "kube-state-metrics",
					},
					tMs: ts, v: 1,
				})
			}
			for _, oldPair := range [][2]string{{"m0", "cpm-a-old"}, {"m1", "cpm-b-old"}, {"m2", "cpm-c-old"}} {
				points = append(points, scrapePoint{
					labels: []string{
						"__name__", "kube_pod_status_ready",
						"condition", "true",
						"namespace", "kube-system",
						"pod", oldPair[1],
						"instance", "ksm",
						"job", "kube-state-metrics",
					},
					tMs: ts, v: 1,
				})
				points = append(points, scrapePoint{
					labels: []string{
						"__name__", "kube_controller_pod",
						"controller_name", "d8-control-plane-manager",
						"controller_type", "DaemonSet",
						"namespace", "kube-system",
						"node", oldPair[0],
						"pod", oldPair[1],
						"job", "kube-state-metrics",
					},
					tMs: ts, v: 1,
				})
			}
		}
		s.ingest(points)
	}

	// Phase 2: rotate. Old head with OLD pods now lives in the keeper.
	s.rotateHead()

	// Phase 3: write NEW generation into HEAD-B (steps 5..29).
	{
		var points []scrapePoint
		for step := int64(5); step < 30; step++ {
			ts := step * stepMs
			for _, node := range []string{"m0", "m1", "m2"} {
				points = append(points, scrapePoint{
					labels: []string{
						"__name__", "kube_node_role",
						"role", "master",
						"node", node,
						"instance", "ksm",
						"job", "kube-state-metrics",
					},
					tMs: ts, v: 1,
				})
			}
			for _, newPair := range [][2]string{{"m0", "cpm-a-new"}, {"m1", "cpm-b-new"}, {"m2", "cpm-c-new"}} {
				points = append(points, scrapePoint{
					labels: []string{
						"__name__", "kube_pod_status_ready",
						"condition", "true",
						"namespace", "kube-system",
						"pod", newPair[1],
						"instance", "ksm",
						"job", "kube-state-metrics",
					},
					tMs: ts, v: 1,
				})
				points = append(points, scrapePoint{
					labels: []string{
						"__name__", "kube_controller_pod",
						"controller_name", "d8-control-plane-manager",
						"controller_type", "DaemonSet",
						"namespace", "kube-system",
						"node", newPair[0],
						"pod", newPair[1],
						"job", "kube-state-metrics",
					},
					tMs: ts, v: 1,
				})
			}
		}
		s.ingest(points)
	}

	// Sanity: we have 2 heads (one in keeper, one active).
	s.Require().NotEmpty(s.manager.Proxy().Heads(), "keeper should hold the rotated head")

	// Probe at t=20m (14m past the last OLD point at t=4m, well outside 1m lookback).
	t20m := model.Time(20 * 60_000).Time()

	// First sanity-check the building blocks of the expression independently:
	leftV := s.queryAt(`kube_node_role{role="master"}`, t20m)
	s.Require().Len(leftV, 3, "expected 3 master roles, got %v", leftV)

	innerV := s.queryAt(
		`(kube_pod_status_ready{condition="true"} == 1) * on (pod, namespace) group_right () `+
			`kube_controller_pod{controller_name="d8-control-plane-manager",controller_type="DaemonSet",namespace="kube-system"}`, t20m)
	if !s.Equal(3, len(innerV), "INNER must yield exactly 3 series at t=20m — got %d:\n%v", len(innerV), innerV) {
		for i, ss := range innerV {
			s.T().Logf("  inner[%d] = %s", i, ss.Metric.String())
		}
	}

	// The actual assertion:
	res := s.queryAt(cpmExpr, t20m)
	if !s.Empty(res, "after rotation+pod replacement settled, alert MUST stay silent — got %v", res) {
		for i, ss := range res {
			s.T().Logf("  firing[%d] = %s", i, ss.Metric.String())
		}
	}
}

// TestCPM_RecordingRuleLagsBeyondLookback_AlertFires reproduces the production
// scenario hypothesised by the user:
//
// `kube_controller_pod` is a RECORDING RULE (no `instance` label, generated by
// kube-prometheus-stack from `kube_pod_owner`). It lives in a different group
// than our alerting group. If that recording group falls behind (e.g. heavy
// eval, missed iterations, scrape lag of `kube_pod_owner`) and stops writing
// fresh `kube_controller_pod` samples for longer than the lookback delta:
//   - the INNER subexpression of the alert becomes empty
//   - `unless` cancels nothing
//   - the alert fires for ALL masters
//
// We model "recording rule lagged" by ingesting kube_controller_pod up to a
// cutoff that's older than `lookback delta` from the query time.
func (s *AdapterPromQLSuite) TestCPM_RecordingRuleLagsBeyondLookback_AlertFires() {
	const stepMs = 60_000

	var points []scrapePoint
	for step := int64(0); step < 30; step++ {
		ts := step * stepMs
		// kube_node_role: scraped continuously
		for _, node := range []string{"m0", "m1", "m2"} {
			points = append(points, scrapePoint{
				labels: []string{
					"__name__", "kube_node_role",
					"role", "master",
					"node", node,
					"instance", "ksm",
					"job", "kube-state-metrics",
					"scrape_endpoint", "main",
				},
				tMs: ts, v: 1,
			})
		}
		// kube_pod_status_ready: scraped continuously
		for _, pod := range []string{"cpm-a", "cpm-b", "cpm-c"} {
			points = append(points, scrapePoint{
				labels: []string{
					"__name__", "kube_pod_status_ready",
					"condition", "true",
					"namespace", "kube-system",
					"pod", pod,
					"instance", "ksm",
					"job", "kube-state-metrics",
				},
				tMs: ts, v: 1,
			})
		}
		// kube_controller_pod: recording rule that STOPS being recorded after step 10.
		if step <= 10 {
			for _, pair := range [][2]string{{"m0", "cpm-a"}, {"m1", "cpm-b"}, {"m2", "cpm-c"}} {
				points = append(points, scrapePoint{
					labels: []string{
						"__name__", "kube_controller_pod",
						"controller", "ds/d8-control-plane-manager",
						"controller_name", "d8-control-plane-manager",
						"controller_type", "DaemonSet",
						"job", "kube-state-metrics",
						"namespace", "kube-system",
						"node", pair[0],
						"pod", pair[1],
					},
					tMs: ts, v: 1,
				})
			}
		}
	}
	s.ingest(points)

	// At t=20m the last kube_controller_pod sample (t=10m) is 10 minutes in the
	// past — far beyond the 1m lookback. INNER becomes empty, the alert fires
	// for all three masters.
	res := s.queryAt(cpmExpr, model.Time(20*60_000).Time())
	s.Require().Len(res, 3, "alert MUST fire for all 3 masters when kube_controller_pod recording rule lags out — got %v", res)
	nodes := map[string]bool{}
	for _, r := range res {
		nodes[r.Metric.Get("node")] = true
	}
	s.Require().True(nodes["m0"] && nodes["m1"] && nodes["m2"],
		"all three masters expected, got: %v", nodes)
}

// TestCPM_RecordingRuleCatchesUp_AlertClears extends the previous test: after
// the recording rule resumes producing samples, within 1m+ the alert MUST
// stop matching. This nails down "is the bug just lag, or is there something
// stickier in storage that prevents recovery?".
func (s *AdapterPromQLSuite) TestCPM_RecordingRuleCatchesUp_AlertClears() {
	const stepMs = 60_000

	var points []scrapePoint
	for step := int64(0); step < 30; step++ {
		ts := step * stepMs
		for _, node := range []string{"m0", "m1", "m2"} {
			points = append(points, scrapePoint{
				labels: []string{
					"__name__", "kube_node_role",
					"role", "master",
					"node", node,
					"instance", "ksm",
					"job", "kube-state-metrics",
					"scrape_endpoint", "main",
				},
				tMs: ts, v: 1,
			})
		}
		for _, pod := range []string{"cpm-a", "cpm-b", "cpm-c"} {
			points = append(points, scrapePoint{
				labels: []string{
					"__name__", "kube_pod_status_ready",
					"condition", "true",
					"namespace", "kube-system",
					"pod", pod,
					"instance", "ksm",
					"job", "kube-state-metrics",
				},
				tMs: ts, v: 1,
			})
		}
		// kube_controller_pod: lag from step 11..15, resumes at step 16.
		if step <= 10 || step >= 16 {
			for _, pair := range [][2]string{{"m0", "cpm-a"}, {"m1", "cpm-b"}, {"m2", "cpm-c"}} {
				points = append(points, scrapePoint{
					labels: []string{
						"__name__", "kube_controller_pod",
						"controller", "ds/d8-control-plane-manager",
						"controller_name", "d8-control-plane-manager",
						"controller_type", "DaemonSet",
						"job", "kube-state-metrics",
						"namespace", "kube-system",
						"node", pair[0],
						"pod", pair[1],
					},
					tMs: ts, v: 1,
				})
			}
		}
	}
	s.ingest(points)

	// At t=14m INNER is empty (last sample was at t=10m, lookback is 1m).
	resDuringLag := s.queryAt(cpmExpr, model.Time(14*60_000).Time())
	s.Require().Len(resDuringLag, 3, "during lag, alert must fire for all 3 masters")

	// At t=20m the recording rule is producing again (last sample t=20m).
	// INNER must be non-empty for all 3 masters → unless cancels everything.
	resAfter := s.queryAt(cpmExpr, model.Time(20*60_000).Time())
	s.Require().Empty(resAfter, "after recording rule recovers, alert MUST clear — got %v", resAfter)
}

// TestCPM_RecordingRuleMissedIterations_AlertStaysFiring models the most
// realistic prod scenario:
//
//   - LookbackDelta = 1m AND alert group interval = 1m AND recording rule
//     interval = 1m  (all three confirmed against the affected cluster's flags
//     and rule_group.json)
//   - The recording rule that produces `kube_controller_pod` runs in a
//     DIFFERENT group, and that group has steady
//     `prometheus_rule_group_iterations_missed_total` (≥ every 2nd iteration).
//     The cluster snapshot shows ≥20 groups with this counter > 0.
//
// When iterations are missed, the recording rule produces samples no more
// often than once per `interval * 2 = 2m`. With LookbackDelta = 1m there is
// always a window of at least 1 minute when the latest visible sample is
// already older than the lookback. Querying inside that window:
//   - kube_controller_pod selector returns empty
//   - INNER is empty → `unless` cancels nothing
//   - alert fires for ALL three masters
//
// Repeating the query on every alert-eval tick (also every 60s) of the
// affected group: half the time the alert sees firing, half the time it
// doesn't. State machine then keeps the alert in r.active continuously
// because at least one out of two evals returns the firing set, never letting
// it drop to Inactive long enough to expire (`resolvedRetention` = 15m).
//
// Concretely we write kube_controller_pod every 120s (sparse) and verify that
// queries at the midpoints (offset +90s after the last sample, well outside
// the 60s lookback window) all fire.
func (s *AdapterPromQLSuite) TestCPM_RecordingRuleMissedIterations_AlertStaysFiring() {
	const oneMin = int64(60_000)
	const twoMin = int64(120_000)

	var points []scrapePoint
	// 30 minutes worth of healthy direct scrapes for kube_node_role +
	// kube_pod_status_ready (every 60s).
	for ts := int64(0); ts <= 30*oneMin; ts += oneMin {
		for _, node := range []string{"m0", "m1", "m2"} {
			points = append(points, scrapePoint{
				labels: []string{
					"__name__", "kube_node_role",
					"role", "master",
					"node", node,
					"instance", "ksm",
					"job", "kube-state-metrics",
					"scrape_endpoint", "main",
				},
				tMs: ts, v: 1,
			})
		}
		for _, pod := range []string{"cpm-a", "cpm-b", "cpm-c"} {
			points = append(points, scrapePoint{
				labels: []string{
					"__name__", "kube_pod_status_ready",
					"condition", "true",
					"namespace", "kube-system",
					"pod", pod,
					"instance", "ksm",
					"job", "kube-state-metrics",
				},
				tMs: ts, v: 1,
			})
		}
	}
	// kube_controller_pod recording rule writes sparsely — every 2 minutes
	// (every 2nd iteration is missed).
	for ts := int64(0); ts <= 30*oneMin; ts += twoMin {
		for _, pair := range [][2]string{{"m0", "cpm-a"}, {"m1", "cpm-b"}, {"m2", "cpm-c"}} {
			points = append(points, scrapePoint{
				labels: []string{
					"__name__", "kube_controller_pod",
					"controller", "ds/d8-control-plane-manager",
					"controller_name", "d8-control-plane-manager",
					"controller_type", "DaemonSet",
					"job", "kube-state-metrics",
					"namespace", "kube-system",
					"node", pair[0],
					"pod", pair[1],
				},
				tMs: ts, v: 1,
			})
		}
	}
	s.ingest(points)

	// Pick query times that fall in the "blind" half of every 2-minute cycle:
	// 90 seconds after the last recording-rule sample. The last sample is at
	// t-90s; lookback window is (t-60s, t]; so t-90s is OUTSIDE the window.
	for _, atMs := range []int64{
		3*oneMin + oneMin/2, // 3:30  → last RR sample @ 2:00, gap 90s
		5*oneMin + oneMin/2, // 5:30  → last RR sample @ 4:00, gap 90s
		7*oneMin + oneMin/2,
		9*oneMin + oneMin/2,
		15*oneMin + oneMin/2,
		25*oneMin + oneMin/2,
	} {
		res := s.queryAt(cpmExpr, model.Time(atMs).Time())
		s.Require().Lenf(res, 3,
			"alert MUST fire for all 3 masters at t=%.1fmin (recording rule misses every 2nd iteration) — got %v",
			float64(atMs)/60_000.0, res)
	}

	// And on the "lucky" tick — exactly when the recording rule did write —
	// the alert correctly clears. This proves the firing is not state-machine
	// stickiness in storage: it's a pure expression-vs-data mismatch.
	for _, atMs := range []int64{4 * oneMin, 6 * oneMin, 10 * oneMin, 20 * oneMin} {
		res := s.queryAt(cpmExpr, model.Time(atMs).Time())
		s.Require().Emptyf(res,
			"alert must NOT fire at t=%dmin (recording rule sample fresh) — got %v",
			atMs/60_000, res)
	}
}

// TestCPM_OffsetCollisionRace_FixedByQueryOffset is the prod-confirmed scenario.
//
// In the affected v0.7.6 cluster the alert group
// `control-plane-manager-control-plane-manager-0` and the recording-rule group
// `monitoring-kubernetes-kubernetes-controller-0` (which writes
// `kube_controller_pod` from `kube_pod_owner`) ended up with practically
// identical hash-derived eval offsets inside the minute:
//
//	last_evaluation_timestamp_seconds (alert     group) = T + 6.329 s
//	last_evaluation_timestamp_seconds (rec.rule  group) = T + 6.310 s
//	kube_controller_pod sample TS                       = T + 6.308 s
//
// i.e. alert eval starts only 19 ms after recording-rule eval, while the
// cppbridge head ingest path needs more than 19 ms to make the new sample
// visible to a querier. Result:
//
//	- alert eval @ T+6.329 sees the LATEST committed sample at T-60+6.308
//	  (previous minute) → distance = 60.021 s > LookbackDelta (60 s)
//	- INNER subexpression is empty → `unless` cancels nothing
//	- `max by (node)` returns all three masters → alert FIRING
//	- API queries (run later) see the new sample already committed → empty
//
// The fix is to add `query_offset` (per-group or global). An offset of 30 s
// pushes alert eval to ask storage for time `T-23.671`; the previous-minute
// sample at `T-53.692` lies 30.021 s back, well within the 60 s lookback.
// INNER becomes non-empty, `unless` cancels everything, alert is silent.
//
// We model the race by ingesting kube_controller_pod up to step N-1 and
// querying at exactly step N's eval time — i.e. "the new sample doesn't
// exist in storage yet from the alert eval's point of view". This gives the
// same lookback geometry as the prod race (latest visible sample > 60 s
// behind the alert eval timestamp).
func (s *AdapterPromQLSuite) TestCPM_OffsetCollisionRace_FixedByQueryOffset() {
	const (
		stepMs       = int64(60_000)
		offsetMs     = int64(6_000)            // shared offset of both groups inside the minute
		alertJitterMs = int64(329)             // alert eval starts 329 ms after minute+offset (matches prod 6.329)
		nLastWritten = int64(29)               // we write kube_controller_pod up to and including this step
		alertStep    = int64(30)               // alert eval happens at this step's tick
	)

	var points []scrapePoint
	for step := int64(0); step <= alertStep; step++ {
		ts := step*stepMs + offsetMs

		for _, node := range []string{"m0", "m1", "m2"} {
			points = append(points, scrapePoint{
				labels: []string{
					"__name__", "kube_node_role",
					"role", "master",
					"node", node,
					"instance", "ksm",
					"job", "kube-state-metrics",
					"scrape_endpoint", "main",
				},
				tMs: ts, v: 1,
			})
		}
		for _, pod := range []string{"cpm-a", "cpm-b", "cpm-c"} {
			points = append(points, scrapePoint{
				labels: []string{
					"__name__", "kube_pod_status_ready",
					"condition", "true",
					"namespace", "kube-system",
					"pod", pod,
					"instance", "ksm",
					"job", "kube-state-metrics",
				},
				tMs: ts, v: 1,
			})
		}
		// kube_controller_pod: recording rule output. STOP one step earlier than
		// the alert tick — this models "the sample for alertStep has been
		// produced by the recording rule but is NOT YET committed when alert
		// eval starts 19 ms later".
		if step <= nLastWritten {
			for _, pair := range [][2]string{{"m0", "cpm-a"}, {"m1", "cpm-b"}, {"m2", "cpm-c"}} {
				points = append(points, scrapePoint{
					labels: []string{
						"__name__", "kube_controller_pod",
						"controller", "ds/d8-control-plane-manager",
						"controller_name", "d8-control-plane-manager",
						"controller_type", "DaemonSet",
						"job", "kube-state-metrics",
						"namespace", "kube-system",
						"node", pair[0],
						"pod", pair[1],
					},
					tMs: ts, v: 1,
				})
			}
		}
	}
	s.ingest(points)

	alertEvalMs := alertStep*stepMs + offsetMs + alertJitterMs

	// (1) Without query_offset: alert eval queries storage at exactly
	// alertEvalMs. Latest committed kube_controller_pod sample is from
	// step nLastWritten = 29 → ts = 29*60_000 + 6_000 = 1_746_000 ms.
	// Distance from alert eval = alertEvalMs - 1_746_000 = 60_329 ms,
	// which is > 60_000 ms (LookbackDelta) → kube_controller_pod is OUT of
	// the lookback window → INNER empty → unless does not cancel anything →
	// alert fires for ALL three masters. This reproduces the prod race.
	resNoOffset := s.queryAt(cpmExpr, model.Time(alertEvalMs).Time())
	s.Require().Lenf(resNoOffset, 3,
		"prod race must reproduce: without query_offset, alert fires for all 3 masters; got %v", resNoOffset)

	// (2) With query_offset = 30s: alert eval queries storage at
	// alertEvalMs - 30_000 ms. Distance from latest committed sample drops to
	// 30_329 ms (< 60_000 ms) → kube_controller_pod is INSIDE the lookback →
	// INNER non-empty → unless cancels all three masters → alert silent.
	const queryOffsetMs = int64(30_000)
	resWithOffset := s.queryAt(cpmExpr, model.Time(alertEvalMs-queryOffsetMs).Time())
	s.Require().Emptyf(resWithOffset,
		"with query_offset=30s race must NOT fire: got %v", resWithOffset)

	// (3) Boundary safety: 19 ms (= prod ingest delay) is enough — anything
	// strictly larger than (60_000 ms - distance_to_latest_visible_sample) lets
	// the previous sample cover the lookback. We pick 1 s as the practical
	// minimum that's bigger than any sane ingest jitter.
	const minimalOffsetMs = int64(1_000)
	resWithMinOffset := s.queryAt(cpmExpr, model.Time(alertEvalMs-minimalOffsetMs).Time())
	s.Require().Emptyf(resWithMinOffset,
		"with query_offset=1s race must already NOT fire: got %v", resWithMinOffset)
}

// Static check that we still satisfy storage.Queryable (compile-time only).
var _ storage.Queryable = (*Adapter)(nil)
