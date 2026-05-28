//go:build !race && !asan

package rules

// tolerantTiming is false in the default build profile, so wall-clock
// timing assertions in TestAsyncRuleEvaluation remain active.
const tolerantTiming = false
