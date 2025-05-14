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

// EqualLabelSets returns whether the two label sets are equal.
func EqualLabelSets(aLSS, bLSS *LabelSetStorage, aLsID, bLsID uint32) bool {
	if aLSS == nil && bLSS == nil {
		return true
	}

	if aLSS == nil || bLSS == nil {
		return false
	}

	if aLSS.Pointer() == bLSS.Pointer() && aLsID == bLsID {
		return true
	}

	return primitivesLabelSetEqual(aLSS.Pointer(), bLSS.Pointer(), aLsID, bLsID)
}

// CompareLabelSets compares the two label sets.
// The result will be 0 if a==b, <0 if a < b, and >0 if a > b.
func CompareLabelSets(aLSS, bLSS *LabelSetStorage, aLsID, bLsID uint32) int {
	// quick exit if empty LabelsCpp
	if aLSS == nil && bLSS == nil {
		return 0
	}

	if aLSS == nil {
		return -1
	}

	if bLSS == nil {
		return 1
	}

	if aLSS.Pointer() == bLSS.Pointer() && aLsID == bLsID {
		return 0
	}

	return int(primitivesLabelSetCompare(aLSS.Pointer(), bLSS.Pointer(), aLsID, bLsID))
}
