package keeper

import (
	"sync"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/prometheus/pp/go/relabeler"
	"github.com/prometheus/prometheus/pp/go/relabeler/querier"
	"github.com/prometheus/prometheus/pp/go/storage/logger"
	"github.com/prometheus/prometheus/pp/go/util"
)

// type Block interface {
// 	TimeBounds() (minT, maxT int64)
// 	// ChunkIterator(minT, maxT int64) ChunkIterator
// 	// IndexWriter() IndexWriter
// }

// HeadBlockWriter writes block on disk from [Head].
type HeadBlockWriter[TBlock any] interface {
	Write(block TBlock) error
}

type WriteNotifier interface {
	NotifyWritten()
}

type HeadBlockBuilder[TBlock any] func() TBlock

type Keeper[TBlock any] struct {
	hbWriter         HeadBlockWriter[TBlock]
	headBlockBuilder HeadBlockBuilder[TBlock]

	writeNotifier        WriteNotifier
	mtx                  sync.Mutex
	heads                []relabeler.Head
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

func (k *Keeper[TBlock]) write() bool {
	k.mtx.Lock()
	lenHeads := len(k.heads)
	if lenHeads == 0 {
		// quick exit
		k.mtx.Unlock()
		return true
	}
	heads := make([]relabeler.Head, lenHeads)
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
		if err := head.Flush(); err != nil {
			logger.Errorf("QUERYABLE STORAGE: failed to flush head %s: %s", head.String(), err.Error())
			successful = false
			continue
		}
		if err := head.Rotate(); err != nil {
			logger.Errorf("QUERYABLE STORAGE: failed to rotate head %s: %s", head.String(), err.Error())
			successful = false
			continue
		}

		tBlockWrite := head.CreateTask(
			relabeler.BlockWrite,
			func(shard relabeler.Shard) error {
				shard.LSSLock()
				defer shard.LSSUnlock()

				bl := k.headBlockBuilder() // relabeler.NewBlock(shard.LSS().Raw(), shard.DataStorage().Raw())
				return k.hbWriter.Write(bl)
			},
			relabeler.ForLSSTask,
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

func (k *Keeper[TBlock]) headIsOutdated(head relabeler.Head) bool {
	headMaxTimestampMs := head.Status(1).HeadStats.MaxTime
	return k.clock.Now().Sub(time.Unix(headMaxTimestampMs/1000, 0)) > k.maxRetentionDuration
}

func (k *Keeper[TBlock]) shrink(persisted ...string) {
	k.mtx.Lock()
	defer k.mtx.Unlock()

	persistedMap := make(map[string]struct{})
	for _, headID := range persisted {
		persistedMap[headID] = struct{}{}
	}

	var heads []relabeler.Head
	for _, head := range k.heads {
		if _, ok := persistedMap[head.ID()]; ok {
			_ = head.Close()
			_ = head.Discard()
			logger.Infof("QUERYABLE STORAGE: head %s persisted, closed and discarded", head.String())
			continue
		}
		heads = append(heads, head)
	}
	k.heads = heads
}
