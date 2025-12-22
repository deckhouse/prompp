//go:build !stringlabels && !dedupelabels && !cpplabels

package labels

import (
	"context"
	"strings"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

// NewLabelsWithLSS init LabelsCpp with LabelSetSnapshot and ls id.
func NewLabelsWithLSS(lss *cppbridge.LabelSetSnapshot, id uint32, builder *ScratchBuilder) Labels {
	if lss == nil {
		return EmptyLabels()
	}

	_ = lss.RangeLabelSet(id, false, func(l cppbridge.Label) error {
		// copy string from cpp memory
		builder.Add(strings.Clone(l.Name), strings.Clone(l.Value))

		return nil
	})

	return builder.Labels()
}

// RenewSnapshot renew ls snapshot. Implementation cpplabels.
func (*Labels) RenewSnapshot() {
	// no-op
}

//
// ScratchBuilder
//

// SetSkipCache set the flag to ignore caches. Implementation cpplabels.
func (*ScratchBuilder) SetSkipCache() {
	// no-op
}

//
// Storage
//

// Storage global label set storage. Implementation cpplabels.
var Storage = &noopStorage{}

// noopStorage for label set. Implementation cpplabels.
type noopStorage struct{}

// SetAdapter store [Adapter]. Implementation cpplabels.
func (*noopStorage) SetAdapter(any) {
	// no-op
}

// Run starts goroutine of the metric collector and the cleaner.
func (*noopStorage) Run(context.Context) {
	// no-op
}
