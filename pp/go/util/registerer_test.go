package util_test

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/prometheus/prometheus/pp/go/util"
)

func TestMustRegisterOrGetReturnsRegisteredCollector(t *testing.T) {
	// Arrange
	registry := prometheus.NewRegistry()
	opts := prometheus.CounterOpts{Name: "test_total", Help: "Test counter."}
	first := util.MustRegisterOrGet(registry, prometheus.NewCounter(opts))

	// Act
	second := util.MustRegisterOrGet(registry, prometheus.NewCounter(opts))

	// Assert
	require.Same(t, first, second)
}

func TestMustRegisterOrGetWithNilRegisterer(t *testing.T) {
	// Arrange
	counter := prometheus.NewCounter(prometheus.CounterOpts{Name: "test_total", Help: "Test counter."})

	// Act
	result := util.MustRegisterOrGet(nil, counter)

	// Assert
	require.Same(t, counter, result)
}
