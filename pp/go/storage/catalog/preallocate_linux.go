//go:build linux

package catalog

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/sys/unix"
)

// preallocateFile allocates size bytes for the file with fallocate(2) without writing data.
// Returns [errPreallocUnsupported] when the filesystem does not support preallocation.
func preallocateFile(f *os.File, size int64) error {
	if err := unix.Fallocate(int(f.Fd()), 0, 0, size); err != nil {
		if errors.Is(err, unix.EOPNOTSUPP) || errors.Is(err, unix.ENOSYS) {
			return fmt.Errorf("%w: %v", errPreallocUnsupported, err)
		}

		return fmt.Errorf("fallocate: %w", err)
	}

	return nil
}
