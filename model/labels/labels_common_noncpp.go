//go:build !cpplabels

package labels

import "github.com/prometheus/prometheus/pp/go/cppbridge"

// NewLabelsCppWithLSS init LabelsCpp with LabelSetStorage and ls id.
func NewLabelsWithLSS(
	lss *cppbridge.LabelSetStorage,
	id uint32,
	length uint16,
) Labels {
	return Labels{
		lss:    lss,
		id:     id,
		length: length,
	}
}
