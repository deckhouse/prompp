package keeper

import (
	"container/heap"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/relabeler/querier"
	"github.com/prometheus/prometheus/pp/go/storage/logger"
	"github.com/prometheus/prometheus/pp/go/util"
)

// type Block interface {
// 	TimeBounds() (minT, maxT int64)
// 	// ChunkIterator(minT, maxT int64) ChunkIterator
// 	// IndexWriter() IndexWriter
// }

const (
	writeRetryTimeout        = 5 * time.Minute
	maxAddIter        uint32 = 5

	MinHeadConvertingQueueSize = 2

	// BlockWrite name of task.
	BlockWrite = "block_write"
)

var (
	ErrorHeadConvertingQueueIsFull error = errors.New("head converting queue is full")
	ErrorNoHeadForConvert          error = errors.New("no head for convert")
)

// HeadBlockWriter writes block on disk from [Head].
type HeadBlockWriter[TBlock any] interface {
	Write(block TBlock) error
}

type WriteNotifier interface {
	NotifyWritten()
}

// GenericTask the minimum required task [Generic] implementation.
type GenericTask interface {
	// Wait for the task to complete on all shards.
	Wait() error
}

//
// DataStorage
//

// DataStorage the minimum required [DataStorage] implementation.
type DataStorage interface {
	// TODO
}

//
// LSS
//

// LSS the minimum required [LSS] implementation.
type LSS interface {
	// TODO
}

//
// Shard
//

// Shard the minimum required head [Shard] implementation.
type Shard[TDataStorage DataStorage, TLSS LSS] interface {
	// DataStorage returns shard [DataStorage].
	DataStorage() TDataStorage

	// LSS returns shard labelset storage [LSS].
	LSS() TLSS
}

//
// Head
//

// Head the minimum required [Head] implementation.
type Head[
	TGenericTask GenericTask,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
] interface {
	// CreateTask create a task for operations on the [Head] shards.
	CreateTask(taskName string, shardFn func(shard TShard) error) TGenericTask

	// Enqueue the task to be executed on shards [Head].
	Enqueue(t TGenericTask)

	// ID returns id [Head].
	ID() string

	// String serialize as string.
	String() string

	// IsReadOnly returns true if the [Head] has switched to read-only.
	IsReadOnly() bool
}

type HeadBlockBuilder[TBlock any] func() TBlock

type HeadForConverting[THead any] struct {
	head      THead
	createdAt time.Duration
}

type HeadConvertingSlice[THead any] []HeadForConverting[THead]

func (q *HeadConvertingSlice[THead]) Len() int {
	return len(*q)
}

func (q *HeadConvertingSlice[THead]) Less(i, j int) bool {
	return (*q)[i].createdAt < (*q)[j].createdAt
}

func (q *HeadConvertingSlice[THead]) Swap(i, j int) {
	(*q)[i], (*q)[j] = (*q)[j], (*q)[i]
}

func (q *HeadConvertingSlice[THead]) Push(head any) {
	*q = append(*q, head.(HeadForConverting[THead]))
}

func (q *HeadConvertingSlice[THead]) Pop() any {
	n := len(*q)
	item := (*q)[n-1]
	*q = (*q)[0 : n-1]
	return item
}

type HeadConvertingQueue[THead any] struct {
	heads HeadConvertingSlice[THead]
}

func NewHeadConvertingQueue[THead any](size int) HeadConvertingQueue[THead] {
	queue := HeadConvertingQueue[THead]{}
	queue.heads = make(HeadConvertingSlice[THead], 0, max(size, MinHeadConvertingQueueSize))
	heap.Init(&queue.heads)
	return queue
}

func (h *HeadConvertingQueue[THead]) Heads() HeadConvertingSlice[THead] {
	return h.heads
}

func (h *HeadConvertingQueue[THead]) Push(head THead, createdAt time.Duration) error {
	if len(h.heads) < cap(h.heads) {
		heap.Push(&h.heads, HeadForConverting[THead]{head: head, createdAt: createdAt})
		return nil
	}

	if h.heads[0].createdAt < createdAt {
		h.heads[0].head = head
		h.heads[0].createdAt = createdAt
		heap.Fix(&h.heads, 0)
		return nil
	}

	return ErrorHeadConvertingQueueIsFull
}

func (h *HeadConvertingQueue[THead]) Pop() THead {
	return heap.Pop(&h.heads).(HeadForConverting[THead]).head
}

type Keeper[
	TGenericTask GenericTask,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TGenericTask, TDataStorage, TLSS, TShard],
	// TBlock any,
] struct {
	heads HeadConvertingQueue[THead]

	headRetentionTimeout time.Duration
}

func (k *Keeper[TGenericTask, TDataStorage, TLSS, TShard, THead]) Add(head THead, createdAt time.Duration) error {
	return k.heads.Push(head, createdAt)
}

func (k *Keeper[TGenericTask, TDataStorage, TLSS, TShard, THead]) Range() func(func(head THead) bool) {
	return func(yield func(head THead) bool) {
		heads := k.heads.Heads()
		for i := range heads {
			if !yield(heads[i].head) {
				return
			}
		}
	}
}

type CustomKeeper[
	TGenericTask GenericTask,
	TDataStorage DataStorage,
	TLSS LSS,
	TShard Shard[TDataStorage, TLSS],
	THead Head[TGenericTask, TDataStorage, TLSS, TShard],
	TBlock any,
] struct {
	hbWriter         HeadBlockWriter[TBlock]
	headBlockBuilder HeadBlockBuilder[TBlock]

	writeNotifier        WriteNotifier
	mtx                  sync.Mutex
	heads                []THead
	headRetentionTimeout time.Duration

	writeTimer   clockwork.Timer
	writeTimeout time.Duration
	addCount     uint32
	closer       *util.Closer

	clock                   clockwork.Clock
	maxRetentionDuration    time.Duration
	headPersistenceDuration prometheus.Histogram
	querierMetrics          *querier.Metrics
}

func (k *CustomKeeper[TGenericTask, TDataStorage, TLSS, TShard, THead, TBlock]) Add(head THead) {
	k.mtx.Lock()
	k.heads = append(k.heads, head)
	logger.Infof("QUERYABLE STORAGE: head %s added", head.String())
	k.mtx.Unlock()

	if atomic.AddUint32(&k.addCount, 1) < maxAddIter {
		k.writeTimer.Reset(k.writeTimeout)
	}
}

func (k *CustomKeeper[TGenericTask, TDataStorage, TLSS, TShard, THead, TBlock]) write() bool {
	k.mtx.Lock()
	lenHeads := len(k.heads)
	if lenHeads == 0 {
		// quick exit
		k.mtx.Unlock()
		return true
	}
	heads := make([]THead, lenHeads)
	copy(heads, k.heads)
	k.mtx.Unlock()

	successful := true
	shouldNotify := false
	persisted := make([]string, 0, lenHeads)
	for _, head := range heads {
		start := k.clock.Now()
		if k.headIsOutdated(head) {
			persisted = append(persisted, head.ID())
			shouldNotify = true
			continue
		}
		// TODO
		//if err := head.Flush(); err != nil {
		//	logger.Errorf("QUERYABLE STORAGE: failed to flush head %s: %s", head.String(), err.Error())
		//	successful = false
		//	continue
		//}
		// if err := head.Rotate(); err != nil {
		// 	logger.Errorf("QUERYABLE STORAGE: failed to rotate head %s: %s", head.String(), err.Error())
		// 	successful = false
		// 	continue
		// }

		tBlockWrite := head.CreateTask(
			BlockWrite,
			func(shard TShard) error {
				// shard.LSSLock()
				// defer shard.LSSUnlock()

				bl := k.headBlockBuilder() // relabeler.NewBlock(shard.LSS().Raw(), shard.DataStorage().Raw())
				return k.hbWriter.Write(bl)
			},
		)
		head.Enqueue(tBlockWrite)
		if err := tBlockWrite.Wait(); err != nil {
			logger.Errorf("QUERYABLE STORAGE: failed to write head %s: %s", head.String(), err.Error())
			successful = false
			continue
		}

		k.headPersistenceDuration.Observe(float64(k.clock.Since(start).Milliseconds()))
		persisted = append(persisted, head.ID())
		shouldNotify = true
		logger.Infof("QUERYABLE STORAGE: head %s persisted, duration: %v", head.String(), k.clock.Since(start))
	}

	if shouldNotify {
		k.writeNotifier.NotifyWritten()
	}

	time.AfterFunc(k.headRetentionTimeout, func() {
		select {
		case <-k.closer.Signal():
			return
		default:
			k.shrink(persisted...)
		}
	})

	return successful
}

func (k *CustomKeeper[TGenericTask, TDataStorage, TLSS, TShard, THead, TBlock]) headIsOutdated(head THead) bool {
	// TODO
	// headMaxTimestampMs := head.Status(1).HeadStats.MaxTime
	// return k.clock.Now().Sub(time.Unix(headMaxTimestampMs/1000, 0)) > k.maxRetentionDuration

	return false
}

func (k *CustomKeeper[TGenericTask, TDataStorage, TLSS, TShard, THead, TBlock]) shrink(persisted ...string) {
	k.mtx.Lock()
	defer k.mtx.Unlock()

	persistedMap := make(map[string]struct{})
	for _, headID := range persisted {
		persistedMap[headID] = struct{}{}
	}

	heads := make([]THead, len(k.heads))
	for _, head := range k.heads {
		if _, ok := persistedMap[head.ID()]; ok {
			// TODO
			// _ = head.Close()
			// _ = head.Discard()
			logger.Infof("QUERYABLE STORAGE: head %s persisted, closed and discarded", head.String())
			continue
		}
		heads = append(heads, head)
	}
	k.heads = heads
}
