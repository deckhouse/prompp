//go:build !linux

package catalog

import "os"

// preallocateFile returns [errPreallocUnsupported]; the caller falls back to writing zeros.
func preallocateFile(_ *os.File, _ int64) error {
	return errPreallocUnsupported
}
