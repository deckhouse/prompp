package keeper

import (
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
)

const (
	// BlockWrite name of task.
	BlockWrite = "block_write"
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
}

type HeadBlockBuilder[TBlock any] func() TBlock

type Keeper[
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

func (k *Keeper[TGenericTask, TDataStorage, TLSS, TShard, THead, TBlock]) Add(head THead) {
	k.mtx.Lock()
	k.heads = append(k.heads, head)
	logger.Infof("QUERYABLE STORAGE: head %s added", head.String())
	k.mtx.Unlock()

	if atomic.AddUint32(&k.addCount, 1) < maxAddIter {
		k.writeTimer.Reset(k.writeTimeout)
	}
}

func (k *Keeper[TGenericTask, TDataStorage, TLSS, TShard, THead, TBlock]) write() bool {
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
		// if err := head.Flush(); err != nil {
		// 	logger.Errorf("QUERYABLE STORAGE: failed to flush head %s: %s", head.String(), err.Error())
		// 	successful = false
		// 	continue
		// }
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

func (k *Keeper[TGenericTask, TDataStorage, TLSS, TShard, THead, TBlock]) headIsOutdated(head THead) bool {
	// TODO
	// headMaxTimestampMs := head.Status(1).HeadStats.MaxTime
	// return k.clock.Now().Sub(time.Unix(headMaxTimestampMs/1000, 0)) > k.maxRetentionDuration

	return false
}

func (k *Keeper[TGenericTask, TDataStorage, TLSS, TShard, THead, TBlock]) shrink(persisted ...string) {
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
