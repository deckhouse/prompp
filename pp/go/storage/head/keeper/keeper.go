package keeper

import (
	"container/heap"
	"errors"
	"sync"
	"time"
)

const (
	MinHeadConvertingQueueSize = 2

	Add            = 0
	AddWithReplace = 1
)

var (
	ErrorNoSlots error = errors.New("keeper has no slots")
)

type AddPolicy = uint8

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

type Head interface {
	// ID returns id [Head].
	ID() string

	// Close closes wals, query semaphore for the inability to get query and clear metrics.
	Close() error
}

type Keeper[THead Head] struct {
	heads headSortedSlice[THead]
	lock  sync.Mutex
}

func NewKeeper[THead Head](queueSize int) *Keeper[THead] {
	return &Keeper[THead]{
		heads: make(headSortedSlice[THead], 0, max(queueSize, MinHeadConvertingQueueSize)),
	}
}

func (k *Keeper[THead]) Add(head THead, createdAt time.Duration, policy AddPolicy) error {
	k.lock.Lock()
	result := k.addHead(head, createdAt, policy)
	k.lock.Unlock()
	return result
}

func (k *Keeper[THead]) addHead(head THead, createdAt time.Duration, policy AddPolicy) error {
	if len(k.heads) < cap(k.heads) {
		heap.Push(&k.heads, sortableHead[THead]{head: head, createdAt: createdAt})
		return nil
	}

	if policy == AddWithReplace && k.heads[0].createdAt < createdAt {
		k.heads[0].head = head
		k.heads[0].createdAt = createdAt
		heap.Fix(&k.heads, 0)
		return nil
	}

	return ErrorNoSlots
}

func (k *Keeper[THead]) setHeads(heads headSortedSlice[THead]) {
	k.heads = heads
	heap.Init(&k.heads)
}

func (k *Keeper[THead]) Heads() []THead {
	k.lock.Lock()
	headsCopy := make([]THead, 0, len(k.heads))
	for _, head := range k.heads {
		headsCopy = append(headsCopy, head.head)
	}
	k.lock.Unlock()

	return headsCopy
}

func (k *Keeper[THead]) Remove(headsForRemove []THead) {
	if len(headsForRemove) == 0 {
		return
	}

	headsMap := make(map[string]*THead, len(headsForRemove))
	for _, head := range headsForRemove {
		headsMap[head.ID()] = nil
	}

	k.lock.Lock()
	newHeads := make([]sortableHead[THead], 0, cap(k.heads))
	for _, head := range k.heads {
		if _, ok := headsMap[head.head.ID()]; ok {
			headsMap[head.head.ID()] = &head.head
		} else {
			newHeads = append(newHeads, head)
		}
	}
	k.setHeads(newHeads)
	k.lock.Unlock()

	for _, head := range headsMap {
		if head != nil {
			_ = (*head).Close()
		}
	}
}

func (k *Keeper[THead]) HasSlot() bool {
	k.lock.Lock()
	result := cap(k.heads) > len(k.heads)
	k.lock.Unlock()
	return result
}
