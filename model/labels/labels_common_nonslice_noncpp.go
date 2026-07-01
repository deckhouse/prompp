//go:build stringlabels

package labels

import "github.com/prometheus/prometheus/pp/go/cppbridge"

// NewLabelsWithLSS init LabelsCpp with LabelSetSnapshot and ls id.
func NewLabelsWithLSS(lss *cppbridge.LabelSetSnapshot, id uint32) Labels {
	if lss == nil {
		return EmptyLabels()
	}

	return Labels{data: lss.Serialize(id)}
}
