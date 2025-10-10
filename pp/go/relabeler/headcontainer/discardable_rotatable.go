package headcontainer

import (
	"errors"

	"github.com/prometheus/prometheus/pp/go/relabeler"
)

//
// DiscardableRotatable
//

type DiscardableRotatable struct {
	onRotate   func(id string, err error) error
	onDiscard  func(id string) error
	afterClose func(id string) error
	relabeler.Head
}

func NewDiscardableRotatable(
	head relabeler.Head,
	onRotate func(id string, err error) error,
	onDiscard func(id string) error,
	afterClose func(id string) error,
) *DiscardableRotatable {
	return &DiscardableRotatable{
		onRotate:   onRotate,
		onDiscard:  onDiscard,
		afterClose: afterClose,
		Head:       head,
	}
}

func (h *DiscardableRotatable) Rotate() error {
	err := h.Head.Rotate()
	if h.onRotate != nil {
		err = errors.Join(err, h.onRotate(h.ID(), err))
		h.onRotate = nil
	}

	return err
}

func (h *DiscardableRotatable) Close() error {
	err := h.Head.Close()
	if h.afterClose != nil {
		err = errors.Join(err, h.afterClose(h.ID()))
	}

	return err
}

func (h *DiscardableRotatable) Discard() (err error) {
	err = h.Head.Discard()
	if h.onDiscard != nil {
		err = errors.Join(err, h.onDiscard(h.ID()))
		h.onDiscard = nil
	}

	return err
}
