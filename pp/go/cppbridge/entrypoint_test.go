package cppbridge_test

import (
	"testing"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

func BenchmarkEmptyIUnsafeCall2(b *testing.B) {
	for i := 0; i < b.N; i++ {
		cppbridge.EmptyIUnsafeCall2()
	}
}

func BenchmarkEmptyUnsafeCall2(b *testing.B) {
	for i := 0; i < b.N; i++ {
		cppbridge.EmptyUnsafeCall2()
	}
}
