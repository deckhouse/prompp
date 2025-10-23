package util

import (
	"errors"
	"io"
	"sync"
)

type Closer struct {
	close      chan struct{}
	closeOnce  sync.Once
	closed     chan struct{}
	closedOnce sync.Once
}

func NewCloser() *Closer {
	return &Closer{
		close:      make(chan struct{}),
		closeOnce:  sync.Once{},
		closed:     make(chan struct{}),
		closedOnce: sync.Once{},
	}
}

func (c *Closer) Done() {
	c.closedOnce.Do(func() {
		close(c.closed)
	})
}

func (c *Closer) Signal() <-chan struct{} {
	return c.close
}

func (c *Closer) Close() error {
	c.closeOnce.Do(func() {
		close(c.close)
	})
	<-c.closed
	return nil
}

func CloseAll(closers ...io.Closer) error {
	var errs error
	for _, closer := range closers {
		errs = errors.Join(errs, closer.Close())
	}
	return errs
}
