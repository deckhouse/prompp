package proxy

import (
	"context"
	"errors"
)

// Head the minimum required [Head] implementation for a proxy.
type Head interface {
	// Close closes wals, query semaphore for the inability to get query and clear metrics.
	Close() error

	// ID returns id [Head].
	ID() string
}

//
// ActiveHeadContainer
//

// ActiveHeadContainer container for active [Head], the minimum required [ActiveHeadContainer] implementation.
type ActiveHeadContainer[THead Head] interface {
	// Close closes [ActiveHeadContainer] for the inability work with [Head].
	Close() error

	// Get the active [Head].
	Get() THead

	// Replace the active [Head] with a new [Head].
	Replace(ctx context.Context, newHead THead) error

	// With calls fn(h Head).
	With(ctx context.Context, fn func(h THead) error) error
}

//
// Keeper
//

// TODO Description
type Keeper[THead Head] interface {
	Add(head THead)
	Close() error
	RangeQueriableHeads(mint, maxt int64) func(func(THead) bool)
}

//
// Proxy
//

// Proxy it proxies requests to the active [Head] and the keeper of old [Head]s.
type Proxy[THead Head] struct {
	activeHeadContainer ActiveHeadContainer[THead]
	keeper              Keeper[THead]
	onClose             func(h THead) error
}

// NewProxy init new [Proxy].
func NewProxy[THead Head](
	activeHeadContainer ActiveHeadContainer[THead],
	keeper Keeper[THead],
	onClose func(h THead) error,
) *Proxy[THead] {
	return &Proxy[THead]{
		activeHeadContainer: activeHeadContainer,
		keeper:              keeper,
		onClose:             onClose,
	}
}

// Add the [Head] to the [Keeper].
func (p *Proxy[THead]) Add(head THead) {
	p.keeper.Add(head)
}

// Close closes [ActiveHeadContainer] and [Keeper] for the inability work with [Head].
func (p *Proxy[THead]) Close() error {
	ahErr := p.activeHeadContainer.Close()

	h := p.activeHeadContainer.Get()
	onCloseErr := p.onClose(h)
	headCloseErr := h.Close()

	keeperErr := p.keeper.Close()

	return errors.Join(ahErr, onCloseErr, headCloseErr, keeperErr)
}

// Get the active [Head].
func (p *Proxy[THead]) Get() THead {
	return p.activeHeadContainer.Get()
}

// RangeQueriableHeadsWithActive returns the iterator to queriable [Head]s:
// the active [Head] and the [Head]s from the [Keeper].
func (p *Proxy[THead]) RangeQueriableHeadsWithActive(mint, maxt int64) func(func(THead) bool) {
	return func(yield func(h THead) bool) {
		ahead := p.activeHeadContainer.Get()
		if !yield(ahead) {
			return
		}

		for head := range p.keeper.RangeQueriableHeads(mint, maxt) {
			if ahead.ID() == head.ID() {
				continue
			}

			if !yield(head) {
				return
			}
		}
	}
}

// RangeQueriableHeads returns the iterator to queriable [Head]s - the [Head]s only from the [Keeper].
func (p *Proxy[THead]) RangeQueriableHeads(mint, maxt int64) func(func(THead) bool) {
	return p.keeper.RangeQueriableHeads(mint, maxt)
}

// Replace the active [Head] with a new [Head].
func (p *Proxy[THead]) Replace(ctx context.Context, newHead THead) error {
	return p.activeHeadContainer.Replace(ctx, newHead)
}

// With calls fn(h Head) on active [Head].
func (p *Proxy[THead]) With(ctx context.Context, fn func(h THead) error) error {
	return p.activeHeadContainer.With(ctx, fn)
}
