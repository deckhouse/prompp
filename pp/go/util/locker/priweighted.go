package locker // based "golang.org/x/sync/semaphore"

import (
	"container/list"
	"context"
	"errors"
	"sync"
)

// ErrSemaphoreClosed error when the semaphore was closed.
var ErrSemaphoreClosed = errors.New("semaphore was closed")

type waiter struct {
	n     int64
	ready chan<- struct{} // Closed when semaphore acquired.
}

// NewWeighted creates a new weighted semaphore with the given
// maximum combined weight for concurrent access.
func NewWeighted(n int64) *Weighted {
	w := &Weighted{size: n}
	return w
}

// Weighted provides a way to bound concurrent access to a resource.
// The callers can request access with a given weight.
type Weighted struct {
	size      int64
	cur       int64
	mu        sync.Mutex
	waiters   list.List
	lastPri   *list.Element
	exclusive bool
	closed    bool
}

// Close sets the flag that the semaphore is closed under the priority lock
// and after unlocking all those waiting will receive the error [ErrSemaphoreClosed].
func (s *Weighted) Close() error {
	unlock, err := s.LockWithPriority(context.Background())
	if err != nil {
		return err
	}

	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()

	unlock()

	return nil
}

// Lock locks for exclusive operation with weight of full size.
func (s *Weighted) Lock(ctx context.Context) (unlock func(), err error) {
	return s.unlock, s.acquireWithInserter(ctx, 0, func(w waiter) *list.Element {
		return s.waiters.PushBack(w)
	})
}

// LockWithPriority locks for exclusive operation with weight of full size
// and push waiter to front or after priority waiter.
func (s *Weighted) LockWithPriority(ctx context.Context) (unlock func(), err error) {
	return s.unlock, s.acquireWithInserter(ctx, 0, func(w waiter) *list.Element {
		var elem *list.Element
		if s.lastPri == nil {
			elem = s.waiters.PushFront(w)
		} else {
			elem = s.waiters.InsertAfter(w, s.lastPri)
		}
		s.lastPri = elem
		return elem
	})
}

// RLock locks for non-exclusive operation with weight of 1.
func (s *Weighted) RLock(ctx context.Context) (runlock func(), err error) {
	return s.runlock, s.acquireWithInserter(ctx, 1, func(w waiter) *list.Element {
		return s.waiters.PushBack(w)
	})
}

// RLockWithPriority locks for non-exclusive operation with weight of 1
// and push waiter to front or after priority waiter.
func (s *Weighted) RLockWithPriority(ctx context.Context) (runlock func(), err error) {
	return s.runlock, s.acquireWithInserter(ctx, 1, func(w waiter) *list.Element {
		var elem *list.Element
		if s.lastPri == nil {
			elem = s.waiters.PushFront(w)
		} else {
			elem = s.waiters.InsertAfter(w, s.lastPri)
		}
		s.lastPri = elem
		return elem
	})
}

// Resize [Weighted] on n.
func (s *Weighted) Resize(n int64) {
	s.mu.Lock()

	if s.size == n {
		s.mu.Unlock()
		return
	}

	s.size = n
	if s.exclusive {
		s.cur = s.size
	}

	s.mu.Unlock()
}

//revive:disable-next-line:function-length from base.
//revive:disable-next-line:cyclomatic from base.
func (s *Weighted) acquireWithInserter(ctx context.Context, n int64, inserter func(waiter) *list.Element) error {
	done := ctx.Done()

	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return ErrSemaphoreClosed
	}

	select {
	case <-done:
		// ctx becoming done has "happened before" acquiring the semaphore,
		// whether it became done before the call began or while we were
		// waiting for the mutex. We prefer to fail even if we could acquire
		// the mutex without blocking.
		s.mu.Unlock()
		return ctx.Err()
	default:
	}

	if ws := s.weightSize(n); s.size-s.cur >= ws && s.waiters.Len() == 0 {
		// Since we hold s.mu and haven't synchronized since checking done, if
		// ctx becomes done before we return here, it becoming done must have
		// "happened concurrently" with this call - it cannot "happen before"
		// we return in this branch. So, we're ok to always acquire here.
		s.cur += ws
		if n == 0 {
			s.exclusive = true
		}
		s.mu.Unlock()

		return nil
	}

	ready := make(chan struct{})
	elem := inserter(waiter{n: n, ready: ready})
	s.mu.Unlock()

	select {
	case <-done:
		s.mu.Lock()
		select {
		case <-ready:
			// Acquired the semaphore after we were canceled.
			// Pretend we didn't and put the tokens back.
			s.cur -= s.weightSize(n)
			s.notifyWaiters()
		default:
			isFront := s.waiters.Front() == elem
			s.waiters.Remove(elem)
			// If we're at the front and there're extra tokens left, notify other waiters.
			if isFront && s.size > s.cur {
				s.notifyWaiters()
			}
		}
		s.mu.Unlock()
		return ctx.Err()

	case <-ready:
		// Acquired the semaphore. Check that ctx isn't already done.
		// We check the done channel instead of calling ctx.Err because we
		// already have the channel, and ctx.Err is O(n) with the nesting
		// depth of ctx.
		select {
		case <-done:
			s.release(n)
			return ctx.Err()
		default:
		}

		if s.closed {
			s.release(n)
			return ErrSemaphoreClosed
		}

		return nil
	}
}

func (s *Weighted) notifyWaiters() {
	for {
		next := s.waiters.Front()
		if next == nil {
			break // No more waiters blocked.
		}

		w := next.Value.(waiter)
		ws := s.weightSize(w.n)
		if s.size-s.cur < ws {
			// Not enough tokens for the next waiter.  We could keep going (to try to
			// find a waiter with a smaller request), but under load that could cause
			// starvation for large requests; instead, we leave all remaining waiters
			// blocked.
			//
			// Consider a semaphore used as a read-write lock, with N tokens, N
			// readers, and one writer.  Each reader can Acquire(1) to obtain a read
			// lock.  The writer can Acquire(N) to obtain a write lock, excluding all
			// of the readers.  If we allow the readers to jump ahead in the queue,
			// the writer will starve — there is always one token available for every
			// reader.
			break
		}

		s.cur += ws
		s.waiters.Remove(next)
		if next == s.lastPri {
			s.lastPri = nil
		}
		if w.n == 0 {
			s.exclusive = true
		}
		close(w.ready)
	}
}

// release releases the semaphore with a weight of n.
func (s *Weighted) release(n int64) {
	s.mu.Lock()

	s.cur -= s.weightSize(n)
	if s.cur < 0 {
		s.mu.Unlock()
		panic("semaphore: released more than held")
	}
	if n == 0 {
		s.exclusive = false
	}
	s.notifyWaiters()

	s.mu.Unlock()
}

// runlock unlocks from non-exclusive operation with weight of 1.
func (s *Weighted) runlock() {
	s.release(1)
}

// unlock unlocks from exclusive operation with weight of full size.
func (s *Weighted) unlock() {
	s.release(0)
}

// weightSize return weight of n, if n == 0, return full size.
func (s *Weighted) weightSize(n int64) int64 {
	if n == 0 {
		return s.size
	}

	return n
}
