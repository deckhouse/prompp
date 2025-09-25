package keeper

import (
	"container/heap"
	"errors"
	"sync"
	"time"

	"github.com/prometheus/prometheus/pp/go/logger"
)

type addPolicy = uint8

const (
	// MinHeadConvertingQueueSize the minimum value of the [Keeper]'s queue.
	MinHeadConvertingQueueSize = 2

	add            addPolicy = 0
	addWithReplace addPolicy = 1
)

// ErrorNoSlots error when keeper has no slots.
var ErrorNoSlots = errors.New("keeper has no slots")

type sortableHead[THead any] struct {
	head      THead
	createdAt time.Duration
}

type headSortedSlice[THead any] []sortableHead[THead]

func (q *headSortedSlice[THead]) Len() int {
	return len(*q)
}

func (q *headSortedSlice[THead]) Less(i, j int) bool {
	return (*q)[i].createdAt < (*q)[j].createdAt
}

func (q *headSortedSlice[THead]) Swap(i, j int) {
	(*q)[i], (*q)[j] = (*q)[j], (*q)[i]
}

func (q *headSortedSlice[THead]) Push(head any) {
	*q = append(*q, head.(sortableHead[THead]))
}

func (q *headSortedSlice[THead]) Pop() any {
	n := len(*q)
	item := (*q)[n-1]
	*q = (*q)[0 : n-1]
	return item
}

// Head the minimum required [Head] implementation for a [Keeper].
type Head[T any] interface {
	// ID returns id [Head].
	ID() string

	// Close closes wals, query semaphore for the inability to get query and clear metrics.
	Close() error

	// for use as a pointer
	*T
}

// Keeper holds outdated heads until conversion.
type Keeper[T any, THead Head[T]] struct {
	heads headSortedSlice[THead]
	lock  sync.RWMutex
}

// NewKeeper init new [Keeper].
func NewKeeper[T any, THead Head[T]](queueSize int) *Keeper[T, THead] {
	return &Keeper[T, THead]{
		heads: make(headSortedSlice[THead], 0, max(queueSize, MinHeadConvertingQueueSize)),
	}
}

// Add the [Head] to the [Keeper] if there is a free slot.
func (k *Keeper[T, THead]) Add(head THead, createdAt time.Duration) error {
	k.lock.Lock()
	result := k.addHead(head, createdAt, add)
	k.lock.Unlock()

	return result
}

// AddWithReplace the [Head] to the [Keeper] with replace if the createdAt is earlier.
func (k *Keeper[T, THead]) AddWithReplace(head THead, createdAt time.Duration) error {
	k.lock.Lock()
	result := k.addHead(head, createdAt, addWithReplace)
	k.lock.Unlock()

	return result
}

// Close closes for the inability work with [Head].
func (k *Keeper[T, THead]) Close() error {
	k.lock.Lock()
	if len(k.heads) == 0 {
		k.lock.Unlock()
		return nil
	}

	errs := make([]error, 0, len(k.heads))
	for _, head := range k.heads {
		errs = append(errs, head.head.Close())
	}
	k.lock.Unlock()

	return errors.Join(errs...)
}

// HasSlot returns the tru if there is a slot in the [Keeper].
func (k *Keeper[T, THead]) HasSlot() bool {
	k.lock.RLock()
	result := cap(k.heads) > len(k.heads)
	k.lock.RUnlock()
	return result
}

// Heads returns a slice of the [Head]s stored in the [Keeper].
func (k *Keeper[T, THead]) Heads() []THead {
	k.lock.RLock()

	if len(k.heads) == 0 {
		k.lock.RUnlock()
		return nil
	}

	headsCopy := make([]THead, 0, len(k.heads))
	for _, head := range k.heads {
		headsCopy = append(headsCopy, head.head)
	}

	k.lock.RUnlock()

	return headsCopy
}

// Remove removes [Head]s from the [Keeper].
func (k *Keeper[T, THead]) Remove(headsForRemove []THead) {
	if len(headsForRemove) == 0 {
		return
	}

	headsMap := make(map[string]THead, len(headsForRemove))
	for _, head := range headsForRemove {
		headsMap[head.ID()] = nil
	}

	k.lock.Lock()
	newHeads := make([]sortableHead[THead], 0, cap(k.heads))
	for _, head := range k.heads {
		if _, ok := headsMap[head.head.ID()]; ok {
			headsMap[head.head.ID()] = head.head
		} else {
			newHeads = append(newHeads, head)
		}
	}
	k.setHeads(newHeads)
	k.lock.Unlock()

	for _, head := range headsMap {
		if head != nil {
			_ = head.Close()
			logger.Infof("[Keeper]: head %s persisted, closed and removed", head.ID())
		}
	}
}

func (k *Keeper[T, THead]) addHead(head THead, createdAt time.Duration, policy addPolicy) error {
	if len(k.heads) < cap(k.heads) {
		heap.Push(&k.heads, sortableHead[THead]{head: head, createdAt: createdAt})
		return nil
	}

	if policy == addWithReplace && k.heads[0].createdAt < createdAt {
		_ = k.heads[0].head.Close()
		k.heads[0].head = head
		k.heads[0].createdAt = createdAt
		heap.Fix(&k.heads, 0)
		return nil
	}

	return ErrorNoSlots
}

func (k *Keeper[T, THead]) setHeads(heads headSortedSlice[THead]) {
	k.heads = heads
	heap.Init(&k.heads)
}
