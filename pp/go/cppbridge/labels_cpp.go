package cppbridge

//
// help func
//

// EqualLabelSets returns whether the two label sets are equal.
//
//revive:disable-next-line:flag-parameter this is a flag, but it's more convenient this way
func EqualLabelSets(aLSS, bLSS *LabelSetStorage, aLsID, bLsID uint32, dropMetricNameA, dropMetricNameB bool) bool {
	if aLSS == nil && bLSS == nil {
		return true
	}

	if aLSS == nil || bLSS == nil {
		return false
	}

	if aLSS.Pointer() == bLSS.Pointer() && aLsID == bLsID && dropMetricNameA == dropMetricNameB {
		return true
	}

	return primitivesLabelSetEqual(aLSS.Pointer(), bLSS.Pointer(), aLsID, bLsID, dropMetricNameA, dropMetricNameB)
}

// CompareLabelSets compares the two label sets.
// The result will be 0 if a==b, <0 if a < b, and >0 if a > b.
//
//revive:disable-next-line:flag-parameter this is a flag, but it's more convenient this way
func CompareLabelSets(aLSS, bLSS *LabelSetStorage, aLsID, bLsID uint32, dropMetricNameA, dropMetricNameB bool) int {
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

	if aLSS.Pointer() == bLSS.Pointer() && aLsID == bLsID && dropMetricNameA == dropMetricNameB {
		return 0
	}

	return int(primitivesLabelSetCompare(
		aLSS.Pointer(), bLSS.Pointer(),
		aLsID, bLsID,
		dropMetricNameA, dropMetricNameB,
	))
}

func (ls *LabelsCpp) ID() uint32 {
	return ls.id
}
