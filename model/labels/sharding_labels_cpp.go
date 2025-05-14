//go:build !slicelabels && !dedupelabels && !stringlabels

package labels

import (
	"github.com/cespare/xxhash/v2"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

// StableHash is a labels hashing implementation which is guaranteed to not change over time.
// This function should be used whenever labels hashing backward compatibility must be guaranteed.
func StableHash(ls Labels) uint64 {
	if ls.IsZero() {
		return 0
	}

	// Use xxhash.Sum64(b) for fast path as it's faster.
	b := make([]byte, 0, 1024) //revive:disable-line:add-constant // this is already constants

	_ = ls.lss.RangeLabelSet(ls.id, func(l cppbridge.Label) error {
		b = append(b, l.Name...)
		b = append(b, seps[0])
		b = append(b, l.Value...)
		b = append(b, seps[0])

		return nil
	})

	return xxhash.Sum64(b)
}
