//go:build !stringlabels && !dedupelabels && !cpplabels

package cppbridge

import (
	"strings"

	"github.com/prometheus/prometheus/model/labels"
)

// NewLabelsWithLSS init LabelsCpp with LabelSetSnapshot and ls id.
func NewLabelsWithLSS(lss *LabelSetSnapshot, id uint32, length uint16) labels.Labels {
	if lss == nil {
		return labels.EmptyLabels()
	}

	builder := labels.NewScratchBuilder(int(length))
	_ = lss.RangeLabelSet(id, func(l Label) error {
		// copy string from cpp memory
		builder.Add(strings.Clone(l.Name), strings.Clone(l.Value))

		return nil
	})

	return builder.Labels()
}
