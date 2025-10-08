package storage

import (
	"context"
	"errors"
	"time"

	"github.com/prometheus/prometheus/pp/go/storage/head/container"
	"github.com/prometheus/prometheus/pp/go/storage/head/keeper"
)

//
// Proxy
//

// Proxy it proxies requests to the active [Head] and the keeper of old [Head]s.
type Proxy struct {
	activeHeadContainer *container.Weighted[Head, *Head]
	keeper              *keeper.Keeper[Head, *Head]
	onClose             func(h *Head) error
}

// NewProxy init new [Proxy].
func NewProxy(
	activeHeadContainer *container.Weighted[Head, *Head],
	hKeeper *keeper.Keeper[Head, *Head],
	onClose func(h *Head) error,
) *Proxy {
	return &Proxy{
		activeHeadContainer: activeHeadContainer,
		keeper:              hKeeper,
		onClose:             onClose,
	}
}

// Add the [Head] to the [Keeper] if there is a free slot.
func (p *Proxy) Add(head *Head, createdAt time.Duration) error {
	return p.keeper.Add(head, createdAt)
}

// AddWithReplace the [Head] to the [Keeper] with replace if the createdAt is earlier.
func (p *Proxy) AddWithReplace(head *Head, createdAt time.Duration) error {
	return p.keeper.AddWithReplace(head, createdAt)
}

// Close closes [ActiveHeadContainer] and [Keeper] for the inability work with [Head].
func (p *Proxy) Close() error {
	ahErr := p.activeHeadContainer.Close()

	h := p.activeHeadContainer.Get()
	onCloseErr := p.onClose(h)
	headCloseErr := h.Close()

	keeperErr := p.keeper.Close()

	return errors.Join(ahErr, onCloseErr, headCloseErr, keeperErr)
}

// Get the active [Head].
func (p *Proxy) Get() *Head {
	return p.activeHeadContainer.Get()
}

// HasSlot returns the tru if there is a slot in the [Keeper].
func (p *Proxy) HasSlot() bool {
	return p.keeper.HasSlot()
}

// Heads returns a slice of the [Head]s stored in the [Keeper].
func (p *Proxy) Heads() []*Head {
	return p.keeper.Heads()
}

// Remove removes [Head]s from the [Keeper].
func (p *Proxy) Remove(headsForRemove []*Head) {
	p.keeper.Remove(headsForRemove)
}

// Replace the active [Head] with a new [Head].
func (p *Proxy) Replace(ctx context.Context, newHead *Head) error {
	return p.activeHeadContainer.Replace(ctx, newHead)
}

// With calls fn(h Head) on active [Head].
func (p *Proxy) With(ctx context.Context, fn func(h *Head) error) error {
	return p.activeHeadContainer.With(ctx, fn)
}
