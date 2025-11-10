package cppbridge_test

import (
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"testing"
)

func TestName(t *testing.T) {
	t.Log(cppbridge.EncodersVersion())
}
