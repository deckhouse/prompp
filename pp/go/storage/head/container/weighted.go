package container

import (
	"context"
	"fmt"
	"sync/atomic"
	"unsafe"

	"github.com/prometheus/prometheus/pp/go/util/locker"
)

// Head the minimum required Head implementation for a container.
type Head[T any] interface {
	Concurrency() int64
	*T
}

// Weighted container for [Head] with weighted locker.
type Weighted[T any, THead Head[T]] struct {
	wlocker *locker.Weighted
	head    *T
}

// NewWeighted init new [Weighted].
func NewWeighted[T any, THead Head[T]](head THead) *Weighted[T, THead] {
	return &Weighted[T, THead]{
		wlocker: locker.NewWeighted(2 * head.Concurrency()), // x2 for back pressure
		head:    head,
	}
}

// Get the active head [Head] under the non-exlusive lock and return.
func (c *Weighted[T, THead]) Get() THead {
	return (*T)(atomic.LoadPointer(
		(*unsafe.Pointer)(unsafe.Pointer(&c.head))), // #nosec G103 // it's meant to be that way
	)
}

// Replace the active head [Head] with a new head under the exlusive priority lock.
func (c *Weighted[T, THead]) Replace(ctx context.Context, newHead THead) error {
	unlock, err := c.wlocker.LockWithPriority(ctx)
	if err != nil {
		return fmt.Errorf("weighted lock with priority: %w", err)
	}

	atomic.StorePointer(
		(*unsafe.Pointer)(unsafe.Pointer(&c.head)), // #nosec G103 // it's meant to be that way
		unsafe.Pointer(newHead),                    // #nosec G103 // it's meant to be that way
	)

	unlock()

	return nil
}

// With calls fn(h Head) under the non-exlusive lock.
func (c *Weighted[T, THead]) With(ctx context.Context, fn func(h THead) error) error {
	runlock, err := c.wlocker.RLock(ctx)
	if err != nil {
		return fmt.Errorf("weighted rlock: %w", err)
	}
	defer runlock()

	return fn(c.head)
}
