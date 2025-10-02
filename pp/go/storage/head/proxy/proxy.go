package proxy

// TODO DELETE
// import (
// 	"context"
// 	"errors"
// 	"time"
// )

// // Head the minimum required [Head] implementation for a proxy.
// type Head interface {
// 	// Close closes wals, query semaphore for the inability to get query and clear metrics.
// 	Close() error
// }

// //
// // ActiveHeadContainer
// //

// // ActiveHeadContainer container for active [Head], the minimum required [ActiveHeadContainer] implementation.
// type ActiveHeadContainer[THead Head] interface {
// 	// Close closes [ActiveHeadContainer] for the inability work with [Head].
// 	Close() error

// 	// Get the active [Head].
// 	Get() THead

// 	// Replace the active [Head] with a new [Head].
// 	Replace(ctx context.Context, newHead THead) error

// 	// With calls fn(h Head).
// 	With(ctx context.Context, fn func(h THead) error) error
// }

// //
// // Keeper
// //

// // Keeper holds outdated heads until conversion.
// type Keeper[THead Head] interface {
// 	// Add the [Head] to the [Keeper] if there is a free slot.
// 	Add(head THead, createdAt time.Duration) error

// 	// AddWithReplace the [Head] to the [Keeper] with replace if the createdAt is earlier.
// 	AddWithReplace(head THead, createdAt time.Duration) error

// 	// Close closes for the inability work with [Head].
// 	Close() error

// 	// HasSlot returns the tru if there is a slot in the [Keeper].
// 	HasSlot() bool

// 	// Heads returns a slice of the [Head]s stored in the [Keeper].
// 	Heads() []THead

// 	// Remove removes [Head]s from the [Keeper].
// 	Remove(headsForRemove []THead)
// }

// //
// // Proxy
// //

// // Proxy it proxies requests to the active [Head] and the keeper of old [Head]s.
// type Proxy[THead Head] struct {
// 	activeHeadContainer ActiveHeadContainer[THead]
// 	keeper              Keeper[THead]
// 	onClose             func(h THead) error
// }

// // NewProxy init new [Proxy].
// func NewProxy[THead Head](
// 	activeHeadContainer ActiveHeadContainer[THead],
// 	keeper Keeper[THead],
// 	onClose func(h THead) error,
// ) *Proxy[THead] {
// 	return &Proxy[THead]{
// 		activeHeadContainer: activeHeadContainer,
// 		keeper:              keeper,
// 		onClose:             onClose,
// 	}
// }

// // Add the [Head] to the [Keeper] if there is a free slot.
// func (p *Proxy[THead]) Add(head THead, createdAt time.Duration) error {
// 	return p.keeper.Add(head, createdAt)
// }

// // AddWithReplace the [Head] to the [Keeper] with replace if the createdAt is earlier.
// func (p *Proxy[THead]) AddWithReplace(head THead, createdAt time.Duration) error {
// 	return p.keeper.AddWithReplace(head, createdAt)
// }

// // Close closes [ActiveHeadContainer] and [Keeper] for the inability work with [Head].
// func (p *Proxy[THead]) Close() error {
// 	ahErr := p.activeHeadContainer.Close()

// 	h := p.activeHeadContainer.Get()
// 	onCloseErr := p.onClose(h)
// 	headCloseErr := h.Close()

// 	keeperErr := p.keeper.Close()

// 	return errors.Join(ahErr, onCloseErr, headCloseErr, keeperErr)
// }

// // Get the active [Head].
// func (p *Proxy[THead]) Get() THead {
// 	return p.activeHeadContainer.Get()
// }

// // HasSlot returns the tru if there is a slot in the [Keeper].
// func (p *Proxy[THead]) HasSlot() bool {
// 	return p.keeper.HasSlot()
// }

// // Heads returns a slice of the [Head]s stored in the [Keeper].
// func (p *Proxy[THead]) Heads() []THead {
// 	return p.keeper.Heads()
// }

// // Remove removes [Head]s from the [Keeper].
// func (p *Proxy[THead]) Remove(headsForRemove []THead) {
// 	p.keeper.Remove(headsForRemove)
// }

// // Replace the active [Head] with a new [Head].
// func (p *Proxy[THead]) Replace(ctx context.Context, newHead THead) error {
// 	return p.activeHeadContainer.Replace(ctx, newHead)
// }

// // With calls fn(h Head) on active [Head].
// func (p *Proxy[THead]) With(ctx context.Context, fn func(h THead) error) error {
// 	return p.activeHeadContainer.With(ctx, fn)
// }
