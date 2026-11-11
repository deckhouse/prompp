package cppbridge

import "runtime"

// LabelSetIDs carries label set ids together with the object that owns the memory they
// live in.
//
// Ids produced by the C++ side live in C-allocated memory released by the owner's
// finalizer, so handing a bare []uint32 to a bridge call lets the GC free the buffer
// mid-call. Passing the owner along keeps it reachable for the duration.
type LabelSetIDs struct {
	ids   []uint32
	owner any
}

// GoLabelSetIDs wraps ids that live in Go memory and therefore need no owner.
func GoLabelSetIDs(ids []uint32) LabelSetIDs {
	return LabelSetIDs{ids: ids}
}

// slice returns the ids. It is unexported on purpose: handing the raw slice out again
// would reopen the hole this type exists to close.
func (l LabelSetIDs) slice() []uint32 {
	return l.ids
}

// keepAlive keeps the owner reachable. Defer it around the bridge call that reads the ids.
func (l LabelSetIDs) keepAlive() {
	runtime.KeepAlive(l.owner)
}
