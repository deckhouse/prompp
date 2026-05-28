//go:build asan

package querier_test

import (
	"time"

	"github.com/prometheus/common/model"
)

const (
	// defaultCountOfSteps is the default count of steps.
	defaultCountOfSteps = 64
)

// defaultSteps is the default steps.
var defaultSteps = []time.Duration{
	defaultStep - time.Second,       // [14s]
	(defaultStep - time.Second) * 2, // [29s]
	defaultStep * 2,                 // [30s]
}

// defaultSubQueries is the default subqueries.
var defaultSubQueries = []subQuery{
	{window: model.Duration(defaultStep), step: 0},                 // [15s]
	{window: model.Duration(defaultStep * 4), step: 0},             // [60s]
	{window: model.Duration(defaultStep*4 + time.Second), step: 0}, // [61s]
}

// defaultModifiers is the default modifiers.
var defaultModifiers = []modifier{
	modifierNone,
	modifierEnd,
	modifierStart,
}

// defaultOffsets is the default offsets.
var defaultOffsets = []offset{
	newOffset(0),
	newOffset(defaultStep * defaultCountOfSteps / 2),
	newOffset(-defaultStep * defaultCountOfSteps / 2),
}
