//go:build asan

package cppbridge

import (
	"runtime"
	"testing"
)

func testGC() {
	if testing.Testing() {
		runtime.GC()
		runtime.GC()
	}
}
