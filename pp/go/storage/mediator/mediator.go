package mediator

import (
	"sync"
	"time"

	"github.com/prometheus/prometheus/pp/go/util"
)

//go:generate -command moq go run github.com/matryer/moq --rm --skip-ensure --pkg mediator_test --out

//
// Timer
//

// Timer implementation timer.
//
//go:generate moq mediator_moq_test.go . Timer
type Timer interface {
	// Chan returns chan with ticker time.
	Chan() <-chan time.Time

	// Reset changes the timer to expire after duration Block and clearing channels.
	Reset()

	// Stop prevents the Timer from firing.
	Stop()
}

//
// Mediator
//

// Mediator notifies about events via the channel.
type Mediator struct {
	timer     Timer
	c         chan struct{}
	closeOnce sync.Once
	closer    *util.Closer
}

// NewMediator init new Mediator.
func NewMediator(timer Timer) *Mediator {
	m := &Mediator{
		timer:     timer,
		c:         make(chan struct{}),
		closeOnce: sync.Once{},
		closer:    util.NewCloser(),
	}

	go m.loop()

	return m
}

// C returns channel with events.
func (m *Mediator) C() <-chan struct{} {
	return m.c
}

// Close stops the internal timer and clears the channel.
func (m *Mediator) Close() {
	_ = m.closer.Close()
	m.timer.Stop()
	m.closeOnce.Do(func() {
		close(m.c)
	})
}

// Trigger send notify to channel.
func (m *Mediator) Trigger() {
	select {
	case m.c <- struct{}{}:
	default:
	}
}

// loop by timer.
func (m *Mediator) loop() {
	defer m.closer.Done()

	for {
		select {
		case <-m.timer.Chan():
			m.Trigger()
			m.timer.Reset()
		case <-m.closer.Signal():
			return
		}
	}
}
