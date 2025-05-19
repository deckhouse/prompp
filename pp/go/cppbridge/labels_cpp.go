package cppbridge

//
// LabelsCpp
//

// LabelsCpp is a sorted set of labels. Is implemented by a cpp lss.
type LabelsCpp struct {
	lss    *LabelSetStorage
	id     uint32
	length uint16
}

// NewLabelsCpp init LabelsCpp with LabelSetStorage and ls id.
func NewLabelsCpp(lss *LabelSetStorage, id uint32, length uint16) *LabelsCpp {
	return &LabelsCpp{
		lss:    lss,
		id:     id,
		length: length,
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

func (ls *LabelsCpp) ID() uint32 {
	return ls.id
}

func (ls *LabelsCpp) LSS() *LabelSetStorage {
	return ls.lss
}

func (ls *LabelsCpp) Length() uint16 {
	return ls.length
}
