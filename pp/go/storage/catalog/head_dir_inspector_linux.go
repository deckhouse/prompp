package catalog

import (
	"fmt"
	"time"

	"golang.org/x/sys/unix"
)

// directoryBirthTime reads the filesystem birth time without substituting mutable mtime or ctime.
func directoryBirthTime(path string) (time.Time, error) {
	var stat unix.Statx_t
	if err := unix.Statx(unix.AT_FDCWD, path, 0, unix.STATX_BTIME, &stat); err != nil {
		return time.Time{}, err
	}

	if stat.Mask&unix.STATX_BTIME == 0 {
		return time.Time{}, fmt.Errorf("directory birth time unavailable: %s", path)
	}

	return time.Unix(stat.Btime.Sec, int64(stat.Btime.Nsec)), nil
}
