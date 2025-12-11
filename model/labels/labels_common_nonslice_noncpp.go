//go:build stringlabels || dedupelabels

package labels

import "github.com/prometheus/prometheus/pp/go/cppbridge"

// NewLabelsWithLSS init LabelsCpp with LabelSetSnapshot and ls id.
func NewLabelsWithLSS(lss *cppbridge.LabelSetSnapshot, id uint32, builder ScratchBuilder) Labels {
	if lss == nil {
		return EmptyLabels()
	}

	_ = lss.RangeLabelSet(id, func(l cppbridge.Label) error {
		builder.Add(l.Name, l.Value)

		return nil
	})

	return builder.Labels()
}
