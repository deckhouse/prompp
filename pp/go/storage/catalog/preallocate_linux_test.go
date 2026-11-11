//go:build linux

package catalog

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func TestPreallocationError(t *testing.T) {
	for _, tc := range []struct {
		name     string
		err      error
		fallback bool
	}{
		{name: "success"},
		{name: "unsupported", err: unix.EOPNOTSUPP, fallback: true},
		{name: "unavailable syscall", err: unix.ENOSYS, fallback: true},
		{name: "invalid argument", err: unix.EINVAL, fallback: true},
		{name: "unavailable device", err: unix.ENODEV, fallback: true},
		{name: "access denied", err: unix.EACCES, fallback: true},
		{name: "no space", err: unix.ENOSPC},
		{name: "quota exceeded", err: unix.EDQUOT},
		{name: "wrapped no space", err: fmt.Errorf("allocation: %w", unix.ENOSPC)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := preallocationError(tc.err)
			if tc.err == nil {
				require.NoError(t, err)
				return
			}

			require.ErrorIs(t, err, tc.err)
			if tc.fallback {
				require.ErrorIs(t, err, errPreallocUnsupported)
			} else {
				require.NotErrorIs(t, err, errPreallocUnsupported)
			}
		})
	}
}
