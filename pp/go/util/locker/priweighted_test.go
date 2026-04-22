package locker_test

import (
	"context"
	"math/rand/v2"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/util/locker"
)

type WeightedSuite struct {
	suite.Suite
}

func TestWeightedSuite(t *testing.T) {
	suite.Run(t, new(WeightedSuite))
}

func (s *WeightedSuite) TestClose() {
	ctx := s.T().Context()
	l := locker.NewWeighted(1)
	s.Require().NoError(l.Close())

	_, err := l.Lock(ctx)
	s.Require().ErrorIs(err, locker.ErrSemaphoreClosed)

	_, err = l.RLock(ctx)
	s.Require().ErrorIs(err, locker.ErrSemaphoreClosed)

	_, err = l.LockWithPriority(ctx)
	s.Require().ErrorIs(err, locker.ErrSemaphoreClosed)

	_, err = l.RLockWithPriority(ctx)
	s.Require().ErrorIs(err, locker.ErrSemaphoreClosed)
}

func (s *WeightedSuite) TestCloseLocked() {
	synctest.Test(s.T(), func(t *testing.T) {
		ctx := context.Background()
		l := locker.NewWeighted(2)
		var counter uint32
		var closed atomic.Bool

		// Hold the whole semaphore exclusively so RLock waiters cannot take a slot
		// before Close acquires the priority writer lock.
		unlockHold, err := l.Lock(ctx)
		s.Require().NoError(err)

		for range 10 {
			go func() {
				unlockG, errG := l.RLock(ctx)
				if errG != nil {
					require.ErrorIs(t, errG, locker.ErrSemaphoreClosed)
					return
				}
				atomic.AddUint32(&counter, 1)
				unlockG()
			}()
			synctest.Wait()
		}

		go func() {
			t.Log("close")
			require.NoError(t, l.Close())
			t.Log("closed")
			closed.Store(true)
		}()
		synctest.Wait()

		s.Require().False(closed.Load())
		t.Log("unlock hold")
		unlockHold()
		t.Log("unlocked")
		synctest.Wait()

		s.Require().True(closed.Load())
		s.Require().Equal(uint32(0), atomic.LoadUint32(&counter))
	})
}

// TestPriorityCancelTailUpdatesLastPri ensures that when the lastPri priority
// waiter is cancelled, the next priority waiter enqueued via the slow path
// (while all slots are still held) is correctly inserted after the remaining
// priority prefix. Without the fix in acquireWithInserter, lastPri would keep
// dangling at the removed element and InsertAfter would return nil, silently
// dropping the new waiter and deadlocking the goroutine.
func (s *WeightedSuite) TestPriorityCancelTailUpdatesLastPri() {
	synctest.Test(s.T(), func(t *testing.T) {
		ctxBase := context.Background()
		l := locker.NewWeighted(2)

		unlockHold1, err := l.RLock(ctxBase)
		s.Require().NoError(err)
		unlockHold2, err := l.RLock(ctxBase)
		s.Require().NoError(err)

		ctxTail, cancelTail := context.WithCancel(ctxBase)
		t.Cleanup(cancelTail)

		unlocks := make(chan func(), 2)

		// p1 — first priority waiter, becomes initial lastPri.
		go func() {
			unlockRP, errP := l.RLockWithPriority(ctxBase)
			require.NoError(t, errP)
			unlocks <- unlockRP
		}()
		synctest.Wait()

		// p2 — tail priority waiter, becomes lastPri, will be cancelled.
		go func() {
			_, errP := l.RLockWithPriority(ctxTail)
			require.ErrorIs(t, errP, context.Canceled)
		}()
		synctest.Wait()

		// Cancel the tail. lastPri must be advanced back to p1; otherwise it
		// will dangle at the removed element.
		cancelTail()
		synctest.Wait()

		// p3 — enqueued while both slots are still held, so this forces the
		// slow-path InsertAfter(lastPri). If lastPri still pointed to the
		// removed p2, InsertAfter would return nil and this goroutine would
		// hang forever.
		go func() {
			unlockRP, errP := l.RLockWithPriority(ctxBase)
			require.NoError(t, errP)
			unlocks <- unlockRP
		}()
		synctest.Wait()

		unlockHold1()
		unlockHold2()
		synctest.Wait()

		(<-unlocks)()
		(<-unlocks)()
	})
}

func (s *WeightedSuite) TestAllocCancelDoesntStarve() {
	synctest.Test(s.T(), func(t *testing.T) {
		l := locker.NewWeighted(10)

		unlock1, err := l.RLock(context.Background())
		s.Require().NoError(err)

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		go func() {
			_, errW := l.Lock(ctx)
			require.ErrorIs(t, errW, context.Canceled)
		}()
		synctest.Wait()

		cancel()

		go func() {
			unlockW, errW := l.Lock(context.Background())
			require.NoError(t, errW)
			unlockW()
		}()
		synctest.Wait()

		unlock1()
		unlock2, err := l.RLock(context.Background())
		s.Require().NoError(err)

		unlock2()
	})
}

func (s *WeightedSuite) TestResizeNoOpSameNUnderExclusive() {
	ctx := context.Background()
	l := locker.NewWeighted(5)

	unlock, err := l.Lock(ctx)
	s.Require().NoError(err)

	l.Resize(5)
	l.Resize(5)

	unlock()

	unlock2, err := l.RLock(ctx)
	s.Require().NoError(err)
	unlock2()
}

func (s *WeightedSuite) TestResizeUnderExclusiveExpandsSlots() {
	ctx := context.Background()
	l := locker.NewWeighted(2)

	unlock, err := l.Lock(ctx)
	s.Require().NoError(err)

	l.Resize(5)
	unlock()

	var wg sync.WaitGroup
	for range 5 {
		wg.Go(func() {
			unlockR, errR := l.RLock(ctx)
			s.Require().NoError(errR)
			unlockR()
		})
	}
	wg.Wait()
}

func (s *WeightedSuite) TestResizeUnderExclusiveShrinkSetsCurToSize() {
	ctx := context.Background()
	l := locker.NewWeighted(5)

	unlock, err := l.Lock(ctx)
	s.Require().NoError(err)

	l.Resize(2)
	unlock()

	// After shrink under exclusive, at most two concurrent readers fit.
	var wg sync.WaitGroup
	started := make(chan struct{}, 2)
	for range 2 {
		wg.Go(func() {
			unlockR, errR := l.RLock(ctx)
			s.Require().NoError(errR)
			started <- struct{}{}
			unlockR()
		})
	}

	for range 2 {
		<-started
	}

	wg.Wait()
}

func (s *WeightedSuite) TestResizeWithoutExclusiveDoesNotResetCur() {
	ctx := context.Background()
	l := locker.NewWeighted(2)
	unlock1, err := l.RLock(ctx)
	s.Require().NoError(err)
	unlock2, err := l.RLock(ctx)
	s.Require().NoError(err)

	l.Resize(1)

	acquired := make(chan struct{})
	go func() {
		unlock3, errR := l.RLock(ctx)
		s.Require().NoError(errR)
		close(acquired)
		unlock3()
	}()

	// With size=1 and cur=2, a new reader needs both slots released.
	unlock1()
	unlock2()

	<-acquired
}

func (s *WeightedSuite) TestAcquireAlreadyCanceledContext() {
	ctx := context.Background()
	l := locker.NewWeighted(2)
	canceled, cancel := context.WithCancel(ctx)
	cancel()

	_, err := l.RLock(canceled)
	s.Require().ErrorIs(err, context.Canceled)

	_, err = l.Lock(canceled)
	s.Require().ErrorIs(err, context.Canceled)

	_, err = l.RLockWithPriority(canceled)
	s.Require().ErrorIs(err, context.Canceled)

	_, err = l.LockWithPriority(canceled)
	s.Require().ErrorIs(err, context.Canceled)
}

func (s *WeightedSuite) TestAcquireAlreadyCanceledDoesNotConsumeSlot() {
	ctx := context.Background()
	l := locker.NewWeighted(1)
	canceled, cancel := context.WithCancel(ctx)
	cancel()

	_, err := l.RLock(canceled)
	s.Require().ErrorIs(err, context.Canceled)

	unlock, err := l.RLock(ctx)
	s.Require().NoError(err)
	unlock()
}

func (s *WeightedSuite) TestCloseTwice() {
	l := locker.NewWeighted(1)
	s.Require().NoError(l.Close())
	s.Require().ErrorIs(l.Close(), locker.ErrSemaphoreClosed)
}

func (s *WeightedSuite) TestRLockWakesToErrSemaphoreClosedAfterClose() {
	synctest.Test(s.T(), func(*testing.T) {
		ctx := context.Background()
		l := locker.NewWeighted(1)

		unlockHold, err := l.RLock(ctx)
		s.Require().NoError(err)

		waitAcquire := make(chan error, 1)
		go func() {
			_, errA := l.RLock(ctx)
			waitAcquire <- errA
		}()
		synctest.Wait()

		closeErr := make(chan error, 1)
		go func() {
			closeErr <- l.Close()
		}()
		synctest.Wait()

		unlockHold()
		synctest.Wait()

		s.Require().NoError(<-closeErr)
		s.Require().ErrorIs(<-waitAcquire, locker.ErrSemaphoreClosed)

		_, err = l.RLock(ctx)
		s.Require().ErrorIs(err, locker.ErrSemaphoreClosed)
	})
}


func (s *WeightedSuite) TestTwoRLockWithPriorityOrder() {
	synctest.Test(s.T(), func(t *testing.T) {
		ctx := context.Background()
		l := locker.NewWeighted(2)

		unlock1, err := l.RLock(ctx)
		require.NoError(t, err)

		unlock2, err := l.RLock(ctx)
		require.NoError(t, err)

		got := make(chan int, 2)

		go func() {
			unlockRP1, errP := l.RLockWithPriority(ctx)
			require.NoError(t, errP)
			got <- 1
			unlockRP1()
		}()
		synctest.Wait()

		go func() {
			unlockRP2, errP := l.RLockWithPriority(ctx)
			require.NoError(t, errP)
			got <- 2
			unlockRP2()
		}()
		synctest.Wait()

		unlock1()
		synctest.Wait()
		s.Require().Equal(1, <-got)

		unlock2()
		synctest.Wait()
		s.Require().Equal(2, <-got)
	})
}

func (s *WeightedSuite) TestLockWithPriorityBeforeOrdinaryLockWhenBothWait() {
	synctest.Test(s.T(), func(t *testing.T) {
		ctx := context.Background()
		l := locker.NewWeighted(2)

		unlock1, err := l.RLock(ctx)
		s.Require().NoError(err)

		unlock2, err := l.RLock(ctx)
		s.Require().NoError(err)

		got := make(chan string, 2)

		go func() {
			unlockOrdinary, errL := l.Lock(ctx)
			require.NoError(t, errL)
			got <- "ordinary"
			unlockOrdinary()
		}()
		synctest.Wait()

		go func() {
			unlockPriority, errL := l.LockWithPriority(ctx)
			require.NoError(t, errL)
			got <- "priority"
			unlockPriority()
		}()
		synctest.Wait()

		unlock1()
		unlock2()
		synctest.Wait()
		s.Require().Equal("priority", <-got)

		synctest.Wait()
		s.Require().Equal("ordinary", <-got)
	})
}

func (s *WeightedSuite) TestPriorityCancelFrontClearsLastPri() {
	synctest.Test(s.T(), func(t *testing.T) {
		ctxBase := context.Background()
		l := locker.NewWeighted(2)

		unlockHold1, err := l.RLock(ctxBase)
		s.Require().NoError(err)

		unlockHold2, err := l.RLock(ctxBase)
		s.Require().NoError(err)

		ctxPri, cancelPri := context.WithCancel(ctxBase)
		t.Cleanup(cancelPri)

		go func() {
			_, errP := l.RLockWithPriority(ctxPri)
			require.ErrorIs(t, errP, context.Canceled)
		}()
		synctest.Wait()

		cancelPri()
		synctest.Wait()

		unlocks := make(chan func(), 1)
		go func() {
			unlockRP, errP := l.RLockWithPriority(ctxBase)
			require.NoError(t, errP)
			unlocks <- unlockRP
		}()
		synctest.Wait()

		unlockHold1()
		synctest.Wait()

		unlockHold2()
		synctest.Wait()

		(<-unlocks)()
	})
}

// TestPriorityCancelTailWithPrevPriority verifies that when both the front and
// the tail priority waiters get cancelled while a middle priority waiter
// remains, lastPri is correctly repositioned to the remaining middle waiter
// (via elem.Prev()) rather than being cleared.
func (s *WeightedSuite) TestPriorityCancelTailWithPrevPriority() {
	synctest.Test(s.T(), func(t *testing.T) {
		ctxBase := context.Background()
		l := locker.NewWeighted(2)

		unlockHold1, err := l.RLock(ctxBase)
		s.Require().NoError(err)

		unlockHold2, err := l.RLock(ctxBase)
		s.Require().NoError(err)

		ctxA, cancelA := context.WithCancel(ctxBase)
		ctxB, cancelB := context.WithCancel(ctxBase)
		t.Cleanup(cancelA)
		t.Cleanup(cancelB)

		go func() {
			_, errP := l.RLockWithPriority(ctxA)
			require.ErrorIs(t, errP, context.Canceled)
		}()
		synctest.Wait()

		go func() {
			unlockRP, errP := l.RLockWithPriority(ctxBase)
			require.NoError(t, errP)
			unlockRP()
		}()
		synctest.Wait()

		go func() {
			_, errP := l.RLockWithPriority(ctxB)
			require.ErrorIs(t, errP, context.Canceled)
		}()
		synctest.Wait()

		cancelA()
		synctest.Wait()

		cancelB()
		synctest.Wait()

		unlockHold1()
		synctest.Wait()

		unlockHold2()
		synctest.Wait()
	})
}

// TestAcquireCancellationRace stresses the race between ctx cancellation and
// notifyWaiters closing the ready channel. The scheduler-dependent timing is
// driven by runtime.Gosched and a random ordering of the two events, so the
// test may hit different branches (ctx wins, ready wins, or both fire before
// the waiter wakes up) on different runs. The goal is not to assert that a
// specific rare branch executes, but that the semaphore never deadlocks and
// remains usable for subsequent acquires regardless of which branch wins.
func (s *WeightedSuite) TestAcquireCancellationRace() {
	if testing.Short() {
		s.T().Skip("timing-dependent cancellation path")
	}

	ctx := context.Background()
	const iterations = 800
	for range iterations {
		l := locker.NewWeighted(1)

		unlockHold, err := l.RLock(ctx)
		s.Require().NoError(err)

		ctxW, cancelW := context.WithCancel(ctx)
		errCh := make(chan error, 1)
		go func() {
			unlockR, errW := l.RLock(ctxW)
			if errW != nil {
				errCh <- errW
				return
			}
			unlockR()
			errCh <- nil
		}()

		for range 20 {
			runtime.Gosched()
		}

		if rand.IntN(2) == 0 {
			cancelW()
			unlockHold()
		} else {
			unlockHold()
			cancelW()
		}

		for range 20 {
			runtime.Gosched()
		}

		var errW error
		select {
		case errW = <-errCh:
		case <-time.After(time.Second):
			s.FailNow("deadlock waiting for RLock waiter")
		}

		if errW != nil {
			s.Require().ErrorIs(errW, context.Canceled)
		}

		// Regardless of which branch fired, the semaphore must remain usable.
		unlock, err := l.RLock(ctx)
		s.Require().NoError(err)
		unlock()
	}
}

// TestWeighted is a stress test kept outside WeightedSuite because testify v1
// does not support parallel execution for suite methods.
func TestWeighted(t *testing.T) {
	t.Parallel()

	ctx := t.Context()
	n := runtime.GOMAXPROCS(0)
	loops := 1000 / n
	l := locker.NewWeighted(int64(n))
	var wg sync.WaitGroup
	wg.Add(n)
	for i := range n {
		rw := i%2 == 0
		go func() {
			defer wg.Done()

			var unlock func()
			for range loops {
				if rw {
					unlock, _ = l.RLock(ctx)
				} else {
					unlock, _ = l.Lock(ctx)
				}

				time.Sleep(time.Duration(rand.Int64N(int64(1*time.Millisecond/time.Nanosecond))) * time.Nanosecond)
				unlock()
			}
		}()
	}
	wg.Wait()
}

// TestLargeAcquireDoesntStarve is a stress test kept outside WeightedSuite
// because testify v1 does not support parallel execution for suite methods.
func TestLargeAcquireDoesntStarve(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	n := int64(runtime.GOMAXPROCS(0))
	l := locker.NewWeighted(n)
	var running atomic.Bool
	running.Store(true)

	var wg sync.WaitGroup
	wg.Add(int(n))

	var runWG sync.WaitGroup
	runWG.Add(int(n))

	for i := n; i > 0; i-- {
		loopUnlock, err := l.RLock(ctx)
		require.NoError(t, err)

		go func(lUnlock func()) {
			runWG.Done()
			defer func() {
				wg.Done()
			}()
			lUnlock()

			for running.Load() {
				unlock, err := l.RLock(ctx)
				require.NoError(t, err)
				time.Sleep(1 * time.Millisecond)
				unlock()
			}
		}(loopUnlock)
	}
	runWG.Wait()

	unlock, err := l.Lock(ctx)
	require.NoError(t, err)
	running.Store(false)
	unlock()
	wg.Wait()
}
