package ready

import (
	"sync"
	"sync/atomic"
)

//
// Notifier
//

// Notifier the sender notifies about readiness for work.
type Notifier interface {
	// NotifyReady the sender notifies about readiness for work.
	NotifyReady()
}

//
// Notifiable
//

// Notifiable notifies the recipient that it is ready to work.
type Notifiable interface {
	// ReadyChan notifies the recipient that it is ready to work.
	ReadyChan() <-chan struct{}
}

//
// Builder
//

// Builder for creating [MultiNotifiable].
type Builder struct {
	input []Notifiable
}

// NewMultiNotifiableBuilder init new [Builder].
func NewMultiNotifiableBuilder() *Builder {
	return &Builder{}
}

// Add [Notifiable] to list.
func (b *Builder) Add(notifiable Notifiable) *Builder {
	b.input = append(b.input, notifiable)
	return b
}

// Build creating [MultiNotifiable] from [Notifiable]s.
func (b *Builder) Build() *MultiNotifiable {
	mn := &MultiNotifiable{
		ready:  make(chan struct{}),
		closed: make(chan struct{}),
	}

	mn.counter.Add(int64(len(b.input)))
	for _, notifiable := range b.input {
		go func(notifiable Notifiable) {
			select {
			case <-notifiable.ReadyChan():
				if mn.counter.Add(-1) == 0 {
					mn.setReady()
				}
			case <-mn.closed:
			}
		}(notifiable)
	}

	return mn
}

//
// MultiNotifiable
//

// MultiNotifiable aggregates multiple [Notifiable]s.
type MultiNotifiable struct {
	readyOnce  sync.Once
	ready      chan struct{}
	closedOnce sync.Once
	closed     chan struct{}
	counter    atomic.Int64
}

// Close stop [MultiNotifiable].
func (mn *MultiNotifiable) Close() error {
	mn.setClosed()
	return nil
}

// ReadyChan notifies the recipient that it is ready to work.
func (mn *MultiNotifiable) ReadyChan() <-chan struct{} {
	return mn.ready
}

// setClosed set once [MultiNotifiable] is closed.
func (mn *MultiNotifiable) setClosed() {
	mn.closedOnce.Do(func() {
		close(mn.closed)
	})
}

// setReady set once [MultiNotifiable] is ready.
func (mn *MultiNotifiable) setReady() {
	mn.readyOnce.Do(func() {
		close(mn.ready)
	})
}

//
// NotifiableNotifier
//

// NotifiableNotifier the sender notifies about readiness for work, notifies the recipient that it is ready to work.
type NotifiableNotifier struct {
	once sync.Once
	c    chan struct{}
}

// NewNotifiableNotifier init new [NotifiableNotifier].
func NewNotifiableNotifier() *NotifiableNotifier {
	return &NotifiableNotifier{
		c: make(chan struct{}),
	}
}

// NotifyReady the sender notifies about readiness for work.
func (nn *NotifiableNotifier) NotifyReady() {
	nn.once.Do(func() {
		close(nn.c)
	})
}

// ReadyChan notifies the recipient that it is ready to work.
func (nn *NotifiableNotifier) ReadyChan() <-chan struct{} {
	return nn.c
}

//
// NoOpNotifier
//

// NoOpNotifier do nothing notifier.
type NoOpNotifier struct{}

// NotifyReady implementation [Notifier], do nothing.
func (NoOpNotifier) NotifyReady() {}
