//go:build race || asan

package rules

// tolerantTiming is true under -race or -asan, where the runtime is
// heavily instrumented and the artificialDelay-based timing bounds in
// TestAsyncRuleEvaluation become flaky in CI (especially on shared
// runners under additional -coverage instrumentation).
const tolerantTiming = true
