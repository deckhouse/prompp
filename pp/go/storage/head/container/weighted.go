package container

import (
	"context"
	"fmt"
	"sync/atomic"
	"unsafe"

	"github.com/prometheus/prometheus/pp/go/util/locker"
)

// DefaultBackPressure is a coefficient that allows enough concurrent tasks
// to pass through so that goroutines don’t stay idle.
const DefaultBackPressure int64 = 2

// Head the minimum required Head implementation for a container.
type Head[T any] interface {
	// Concurrency return current head workers concurrency.
	Concurrency() int64

	// for use as a pointer
	*T
}

// Weighted container for [Head] with weighted locker.
type Weighted[T any, THead Head[T]] struct {
	wlocker      *locker.Weighted
	head         *T
	backPressure int64
}

// NewWeighted init new [Weighted].
func NewWeighted[T any, THead Head[T]](head THead, backPressure int64) *Weighted[T, THead] {
	if backPressure == 0 {
		backPressure = DefaultBackPressure
	}

	return &Weighted[T, THead]{
		wlocker:      locker.NewWeighted(backPressure * head.Concurrency()),
		head:         head,
		backPressure: backPressure,
	}
}

// Close closes wlocker semaphore for the inability work with [Head].
func (c *Weighted[T, THead]) Close() error {
	return c.wlocker.Close()
}

// Get the active head [Head] without lock and return.
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
	c.wlocker = locker.NewWeighted(c.backPressure * newHead.Concurrency())

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
