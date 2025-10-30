package mediator_test

import (
	"context"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/prometheus/prometheus/pp/go/storage/mediator"
	"github.com/stretchr/testify/suite"
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	counter := 0
	done := make(chan struct{})
	start := sync.WaitGroup{}
	start.Add(1)
	go func() {
		start.Done()

		for range m.C() {
			counter++
			break
		}

		close(done)
	}()

	start.Wait()
	s.T().Log("mediator close")
	m.Close()

	select {
	case <-done:
	case <-ctx.Done():
		m.Trigger()
	}
	cancel()

	<-done

	s.Equal(0, counter)
	s.Equal(1, stopCounter)
}

func (s *MediatorSuite) TestTrigger() {
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	counter := 0
	done := make(chan struct{})
	start := sync.WaitGroup{}
	start.Add(1)
	go func() {
		start.Done()
		select {
		case <-m.C():
			counter++
			close(done)
		case <-ctx.Done():
		}
	}()

	start.Wait()
	s.T().Log("trigger")
	m.Trigger()

	select {
	case <-done:
	case <-ctx.Done():
	}
	cancel()

	s.Equal(1, counter)
	s.Empty(timer.ResetCalls())
}

func (s *MediatorSuite) TestTriggerWithResetTimer() {
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	counter := 0
	done := make(chan struct{})
	start := sync.WaitGroup{}
	start.Add(1)
	go func() {
		start.Done()
		select {
		case <-m.C():
			counter++
			close(done)
		case <-ctx.Done():
		}
	}()

	start.Wait()
	s.T().Log("trigger with reset timer")
	m.TriggerWithResetTimer()

	select {
	case <-done:
	case <-ctx.Done():
	}
	cancel()

	s.Equal(1, counter)
	s.Len(timer.ResetCalls(), 1)
}

func (s *MediatorSuite) TestTriggerSkip() {
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

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	counter := 0
	done := make(chan struct{})
	start := sync.WaitGroup{}
	start.Add(1)
	go func() {
		start.Wait()
		select {
		case <-m.C():
			counter++
			close(done)
		case <-ctx.Done():
		}
	}()

	s.T().Log("trigger")
	m.Trigger()
	start.Done()

	select {
	case <-done:
	case <-ctx.Done():
	}
	cancel()

	s.Equal(0, counter)
}
