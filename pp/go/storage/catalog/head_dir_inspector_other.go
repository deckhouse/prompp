//go:build !linux

package catalog

import (
	"errors"
	"time"
)

// directoryBirthTime reports unsupported birth-time inspection outside Linux.
func directoryBirthTime(_ string) (time.Time, error) {
	return time.Time{}, errors.New("directory birth time unavailable")
}
