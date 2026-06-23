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
	defaultStep - time.Second,
	defaultStep * 2,
}

// defaultSubQueries is the default subqueries.
var defaultSubQueries = []subQuery{
	{window: model.Duration(defaultStep), step: 0},
	{window: model.Duration(defaultStep*4 + time.Second), step: 0},
}

// defaultModifiers is the default modifiers.
var defaultModifiers = []modifier{
	modifierNone,
	modifierEnd,
}

// defaultOffsets is the default offsets.
var defaultOffsets = []offset{
	newOffset(0),
	newOffset(defaultStep * defaultCountOfSteps / 2),
}
