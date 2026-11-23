package stagestats

import "time"

// epoch is the reference point of [Now].
var epoch = time.Now()

// Now returns the monotonic time in nanoseconds since the package initialization.
// It does not depend on the system wall clock changes.
func Now() int64 {
	return int64(time.Since(epoch))
}
