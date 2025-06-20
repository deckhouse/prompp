//go:build !stringlabels && !dedupelabels && !cpplabels

package labels

import (
	"strings"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

// NewLabelsWithLSS init LabelsCpp with LabelSetSnapshot and ls id.
func NewLabelsWithLSS(lss *cppbridge.LabelSetSnapshot, id uint32, length uint16) Labels {
	if lss == nil {
		return EmptyLabels()
	}

	builder := NewScratchBuilder(int(length))
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

// SetReceiver store Receiver. Implementation cpplabels.
func (*noopStorage) SetReceiver(_ any) {
	// no-op
}
