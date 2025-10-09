package headcontainer

import (
	"github.com/prometheus/prometheus/pp/go/relabeler"
)

//
// HeapProfileWritable
//

type HeapProfileWriter interface {
	WriteHeapProfile() error
}

type HeapProfileWritable struct {
	heapProfileWriter HeapProfileWriter
	relabeler.Head
}

func NewHeapProfileWritable(head relabeler.Head, heapProfileWriter HeapProfileWriter) *HeapProfileWritable {
	return &HeapProfileWritable{Head: head, heapProfileWriter: heapProfileWriter}
}

func (h *HeapProfileWritable) Rotate() error {
	if err := h.Head.Rotate(); err != nil {
		return err
	}

	return h.heapProfileWriter.WriteHeapProfile()
}
