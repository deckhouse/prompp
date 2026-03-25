package rules

import (
	"sync"
)

// ConcurrencyExecuter executes eval rules in parallel in pre-launched goroutines.
type ConcurrencyExecuter interface {
	// Execute eval rules in parallel in pre-launched goroutines via queue.
	Execute(fn func())

	// Run worker goroutines.
	Run()

	// Stop send signal for stop launched goroutines and waits until all goroutines stop.
	Stop()
}

// ConcurrentRuleEvalExecuter executes eval rules in parallel in pre-launched goroutines,
// if there are no free goroutines, then it is executed on the calling goroutine.
type ConcurrentRuleEvalExecuter struct {
	queue          chan func()
	stop           chan struct{}
	wg             sync.WaitGroup
	maxConcurrency int
}

// NewConcurrentRuleEvalExecuter init new [ConcurrentRuleEvalExecuter].
func NewConcurrentRuleEvalExecuter(maxConcurrency int) *ConcurrentRuleEvalExecuter {
	return &ConcurrentRuleEvalExecuter{
		queue:          make(chan func()),
		stop:           make(chan struct{}),
		wg:             sync.WaitGroup{},
		maxConcurrency: maxConcurrency,
	}
}

// Execute eval rules in parallel in pre-launched goroutines via queue.
func (e *ConcurrentRuleEvalExecuter) Execute(fn func()) {
	select {
	case e.queue <- fn:
	default:
		fn()
	}
}

// Run worker goroutines.
func (e *ConcurrentRuleEvalExecuter) Run() {
	if e.isStopped() {
		return
	}

	e.wg.Add(e.maxConcurrency)
	for range e.maxConcurrency {
		go e.workerLoop()
	}
}

// Stop send signal for stop launched goroutines and waits until all goroutines stop.
func (e *ConcurrentRuleEvalExecuter) Stop() {
	close(e.stop)
	e.wg.Wait()
}

// isStopped check goroutines is stopped.
func (e *ConcurrentRuleEvalExecuter) isStopped() bool {
	select {
	case <-e.stop:
		return true

	default:
		return false
	}
}

// workerLoop main workers goroutines.
func (e *ConcurrentRuleEvalExecuter) workerLoop() {
	defer e.wg.Done()

	for {
		select {
		case <-e.stop:
			return

		case fn := <-e.queue:
			fn()
		}
	}
}
