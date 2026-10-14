//go:build linux

package catalog

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// preallocateFile allocates size bytes for the file with fallocate(2) without writing data.
// Returns [errPreallocUnsupported] to allow a zero-write fallback, except for space and quota errors.
func preallocateFile(f *os.File, size int64) error {
	return preallocationError(unix.Fallocate(int(f.Fd()), 0, 0, size))
}

func preallocationError(err error) error {
	if err == nil {
		return nil
	}

	if errors.Is(err, unix.ENOSPC) || errors.Is(err, unix.EDQUOT) {
		return fmt.Errorf("fallocate: %w", err)
	}

	return fmt.Errorf("%w: fallocate: %w", errPreallocUnsupported, err)
}
