//go:build !stringlabels && !dedupelabels && !cpplabels

package labels

import (
	"strings"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

// NewLabelsWithLSS init LabelsCpp with LabelSetStorage and ls id.
func NewLabelsWithLSS(lss *cppbridge.LabelSetStorage, id uint32, length uint16) Labels {
	if lss == nil {
		return EmptyLabels()
	}

	builder := NewScratchBuilder(int(length))
	_ = lss.RangeLabelSet(id, func(l cppbridge.Label) error {
		// copy string from cpp memory
		builder.Add(strings.Clone(l.Name), strings.Clone(l.Value))

		return nil
	})

	return builder.Labels()
}
