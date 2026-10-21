package catalog

import (
	"errors"
	"fmt"
	"os"
)

// errPreallocUnsupported allows the caller to try writing zeros after a preallocation failure.
var errPreallocUnsupported = errors.New("fallocate unsupported")

// writeZeros extends the file up to size bytes by writing zero chunks of at most 1 MiB.
func writeZeros(f *os.File, size int64) error {
	const chunkSize = 1 << 20

	buf := make([]byte, chunkSize)
	for offset := int64(0); offset < size; {
		n := min(size-offset, int64(chunkSize))

		if _, err := f.WriteAt(buf[:n], offset); err != nil {
			return fmt.Errorf("write zeros: %w", err)
		}

		offset += n
	}

	return nil
}
