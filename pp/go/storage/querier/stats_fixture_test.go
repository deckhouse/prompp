package querier_test

import (
	"context"
	"sync/atomic"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/model"
	"github.com/prometheus/prometheus/pp/go/storage/head/head"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard"
	"github.com/prometheus/prometheus/pp/go/storage/head/task"
	"github.com/prometheus/prometheus/pp/go/storage/querier"
)

type statsTask = task.Generic[*statsShard]

// statsHead injects acquisition and enqueue failures and observes task and semaphore cleanup.
type statsHead struct {
	*head.Head[*statsShard, *statsShard]
	acquireErr error
	onEnqueue  func(*statsTask)
	onPutTask  func(*statsTask)
	onRelease  func()
}

func (h *statsHead) AcquireQuery(ctx context.Context) (func(), error) {
	if h.acquireErr != nil {
		return nil, h.acquireErr
	}

	release, err := h.Head.AcquireQuery(ctx)
	if err != nil {
		return nil, err
	}

	return func() {
		release()

		if h.onRelease != nil {
			h.onRelease()
		}
	}, nil
}

// Enqueue invokes the test hook before dispatching the task to shards.
func (h *statsHead) Enqueue(t *statsTask) {
	if h.onEnqueue != nil {
		h.onEnqueue(t)
	}

	h.Head.Enqueue(t)
}

// PutTask invokes the test hook before returning the completed task to its pool.
func (h *statsHead) PutTask(t *statsTask) {
	if h.onPutTask != nil {
		h.onPutTask(t)
	}

	h.Head.PutTask(t)
}

type statsShard struct {
	*shard.Shard
	ds        *statsDataStorage
	lss       *statsLSS
	loadErr   error
	loadGate  <-chan struct{}
	loadCalls atomic.Int64
}

func newStatsShard(sh *shard.Shard) *statsShard {
	return &statsShard{
		Shard: sh,
		ds:    &statsDataStorage{DataStorage: sh.DataStorage()},
		lss:   &statsLSS{LSS: sh.LSS()},
	}
}

// Close leaves the underlying shard owned by the original test head.
func (*statsShard) Close() error {
	return nil
}

func (s *statsShard) DataStorage() querier.DataStorage {
	return s.ds
}

func (s *statsShard) LoadAndQuerySeriesData() error {
	s.loadCalls.Add(1)

	if s.loadGate != nil {
		<-s.loadGate
	}

	s.LoadAndQuerySeriesDataTask().Release(func([]uintptr) {})

	return s.loadErr
}

func (s *statsShard) LSS() querier.LSS {
	return s.lss
}

type statsDataStorage struct {
	querier.DataStorage
	needLoad bool
}

func (d *statsDataStorage) InstantQuery(maxt int64, ids []uint32, samples uintptr) cppbridge.DataStorageQueryResult {
	result := d.DataStorage.InstantQuery(maxt, ids, samples)

	if d.needLoad {
		result.Status = cppbridge.DataStorageQueryStatusNeedDataLoad
	}

	return result
}

func (d *statsDataStorage) Query(query cppbridge.DataStorageQuery) cppbridge.DataStorageQueryResult {
	result := d.DataStorage.Query(query)

	if d.needLoad {
		result.Status = cppbridge.DataStorageQueryStatusNeedDataLoad
	}

	return result
}

type statsLSS struct {
	querier.LSS
	selectorErr error
	metadataErr error
	cancel      context.CancelFunc
}

func (l *statsLSS) QueryLabelNames(
	id uint16, matchers []model.LabelMatcher,
	add func(uint16, *cppbridge.LabelSetSnapshot, []string),
) error {
	if l.cancel != nil {
		l.cancel()
	}

	if l.metadataErr != nil {
		return l.metadataErr
	}

	return l.LSS.QueryLabelNames(id, matchers, add)
}

func (l *statsLSS) QueryLabelValues(
	id uint16, name string, matchers []model.LabelMatcher,
	add func(uint16, *cppbridge.LabelSetSnapshot, []string),
) error {
	if l.cancel != nil {
		l.cancel()
	}

	if l.metadataErr != nil {
		return l.metadataErr
	}

	return l.LSS.QueryLabelValues(id, name, matchers, add)
}

func (l *statsLSS) QuerySelector(
	id uint16, matchers []model.LabelMatcher,
) (uintptr, *cppbridge.LabelSetSnapshot, error) {
	if l.selectorErr != nil {
		return 0, nil, l.selectorErr
	}

	return l.LSS.QuerySelector(id, matchers)
}
