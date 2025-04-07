package cppbridge

import (
	"github.com/prometheus/prometheus/model/labels"
)

//
// LabelsCpp
//

// LabelsCpp is a sorted set of labels. Is implemented by a cpp lss.
type LabelsCpp struct {
	serializedLS labels.Labels
	lss          *LabelSetStorage
	id           uint32
	length       uint16
}

// NewLabelsCpp init LabelsCpp with LabelSetStorage and ls id.
func NewLabelsCpp(lss *LabelSetStorage, length int, id uint32) *LabelsCpp {
	return &LabelsCpp{
		lss:    lss,
		id:     id,
		length: uint16(length), // #nosec G115 // no overflow
	}
}

// IsZero returns true if ls lss referece is nil.
// Implements yaml.IsZeroer - if we don't have this then 'omitempty' fields are always omitted.
func (ls *LabelsCpp) IsZero() bool {
	return ls.lss == nil
}

// Len returns the number of labels.
func (ls *LabelsCpp) Len() int {
	if ls.IsZero() {
		return 0
	}

	if ls.length != 0 {
		return int(ls.length)
	}

	length := int(primitivesLabelSetLength(ls.lss.Pointer(), ls.id)) // #nosec G115 // no overflow

	return length
}

// Labels returns the name/value pairs added as a Labels object.
func (ls *LabelsCpp) Labels() labels.Labels {
	if ls.IsZero() || ls.Len() == 0 {
		return labels.Labels{}
	}

	if !ls.serializedLS.IsEmpty() {
		return ls.serializedLS
	}

	labelSet := primitivesLabelSetSerialize(ls.lss.Pointer(), ls.id)

	sb := labels.NewScratchBuilder(ls.Len())
	for i := range labelSet {
		sb.Add(labelSet[i].Name, labelSet[i].Value)
	}
	sb.Overwrite(&ls.serializedLS)

	primitivesLabelSetFree(labelSet)

	return ls.serializedLS
}
