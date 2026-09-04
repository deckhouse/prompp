package mediator_test

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/storage/mediator"
)

type MediatorSuite struct {
	suite.Suite
}

func TestMediatorSuite(t *testing.T) {
	suite.Run(t, new(MediatorSuite))
}

func (s *MediatorSuite) TestC() {
	synctest.Test(s.T(), func(t *testing.T) {
		chTimer := make(chan time.Time, 1)

		timer := &TimerMock{
			ChanFunc: func() <-chan time.Time {
				return chTimer
			},
			ResetFunc: func() {},
			StopFunc:  func() {},
		}

		m := mediator.NewMediator(timer)

		counter := 0

		go func() {
			t.Log("service run")
			<-m.C()
			counter++
		}()
		synctest.Wait()

		go func() {
			t.Log("timer tick")
			chTimer <- time.Time{}
		}()
		synctest.Wait()

		s.Equal(1, counter)
		m.Close()
	})
}

func (s *MediatorSuite) TestClose() {
	synctest.Test(s.T(), func(t *testing.T) {
		chTimer := make(chan time.Time, 1)
		stopCounter := 0

		timer := &TimerMock{
			ChanFunc: func() <-chan time.Time {
				return chTimer
			},
			ResetFunc: func() {},
			StopFunc:  func() { stopCounter++ },
		}

		m := mediator.NewMediator(timer)

		counter := 0
		go func() {
			_, ok := <-m.C()
			if ok {
				counter++
			}
		}()

		synctest.Wait()
		m.Close()
		synctest.Wait()
		require.Equal(t, 0, counter)
		require.Equal(t, 1, stopCounter)
	})
}

func (s *MediatorSuite) TestTrigger() {
	synctest.Test(s.T(), func(t *testing.T) {
		chTimer := make(chan time.Time, 1)

		timer := &TimerMock{
			ChanFunc: func() <-chan time.Time {
				return chTimer
			},
			ResetFunc: func() {},
			StopFunc:  func() {},
		}

		m := mediator.NewMediator(timer)
		defer m.Close()

		counter := 0
		go func() {
			<-m.C()
			counter++
		}()

		synctest.Wait()
		m.Trigger()
		synctest.Wait()

		require.Equal(t, 1, counter)
		require.Empty(t, timer.ResetCalls())
	})
}

func (s *MediatorSuite) TestTriggerWithResetTimer() {
	synctest.Test(s.T(), func(t *testing.T) {
		chTimer := make(chan time.Time, 1)

		timer := &TimerMock{
			ChanFunc: func() <-chan time.Time {
				return chTimer
			},
			ResetFunc: func() {},
			StopFunc:  func() {},
		}

		m := mediator.NewMediator(timer)
		defer m.Close()

		counter := 0
		go func() {
			<-m.C()
			counter++
		}()

		synctest.Wait()
		m.TriggerWithResetTimer()
		synctest.Wait()

		require.Equal(t, 1, counter)
		require.Len(t, timer.ResetCalls(), 1)
	})
}

// TestCloseRacesWithTrigger reproduces the scenario where Close() closes the
// event channel while another goroutine concurrently calls Trigger*() (e.g.
// Rotator.rotate() racing with shutdown). Before the fix this either raced on
// m.c or panicked with "send on closed channel"; now Trigger*() must observe
// the closed state and become a no-op instead.
func (s *MediatorSuite) TestCloseRacesWithTrigger() {
	chTimer := make(chan time.Time, 1)

	timer := &TimerMock{
		ChanFunc:  func() <-chan time.Time { return chTimer },
		ResetFunc: func() {},
		StopFunc:  func() {},
	}

	m := mediator.NewMediator(timer)

	s.Require().NotPanics(func() {
		var wg sync.WaitGroup
		wg.Add(3)

		go func() {
			defer wg.Done()

			for range 1000 {
				m.Trigger()
			}
		}()

		go func() {
			defer wg.Done()

			for range 1000 {
				m.TriggerWithResetTimer()
			}
		}()

		go func() {
			defer wg.Done()
			m.Close()
		}()

		wg.Wait()
	})
}

func (s *MediatorSuite) TestTriggerSkip() {
	synctest.Test(s.T(), func(t *testing.T) {
		chTimer := make(chan time.Time, 1)

		timer := &TimerMock{
			ChanFunc: func() <-chan time.Time {
				return chTimer
			},
			ResetFunc: func() {},
			StopFunc:  func() {},
		}

		m := mediator.NewMediator(timer)
		defer m.Close()

		m.Trigger()

		counter := 0

		go func() {
			<-m.C()
			counter++
		}()

		synctest.Wait()

		require.Equal(t, 0, counter)
	})
}
