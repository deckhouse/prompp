package container_test

import (
	"context"
	"fmt"
	"testing"
	"unsafe"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/storage/head/container"
	"github.com/prometheus/prometheus/pp/go/util/locker"
)

type WeightedSuite struct {
	suite.Suite
}

func TestWeightedSuite(t *testing.T) {
	suite.Run(t, new(WeightedSuite))
}

func (s *WeightedSuite) TestGet() {
	expectedHead := &testHead{c: 2}
	c := container.NewWeighted(expectedHead)

	actualHead := c.Get()

	s.Equal(expectedHead, actualHead)
}

func (s *WeightedSuite) TestReplace() {
	baseCtx := context.Background()
	expectedHead := &testHead{c: 2}
	newHead := &testHead{c: 3}
	c := container.NewWeighted(expectedHead)

	err := c.Replace(baseCtx, newHead)
	s.Require().NoError(err)

	actualHead := c.Get()

	s.NotEqual(expectedHead, actualHead)
	s.NotEqual(unsafe.Pointer(expectedHead), unsafe.Pointer(actualHead))
	s.Equal(newHead, actualHead)
	s.Equal(unsafe.Pointer(newHead), unsafe.Pointer(actualHead))
}

func (s *WeightedSuite) TestReplaceError() {
	expectedHead := &testHead{c: 2}
	newHead := &testHead{c: 3}
	c := container.NewWeighted(expectedHead)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := c.Replace(ctx, newHead)
	s.Error(err)
}

func (s *WeightedSuite) TestWith() {
	baseCtx := context.Background()
	expectedHead := &testHead{c: 2}
	c := container.NewWeighted(expectedHead)

	err := c.With(baseCtx, func(h *testHead) error {
		if expectedHead.c != h.c {
			return fmt.Errorf("expectedHead(%d) not equal actual(%d)", expectedHead.c, h.c)
		}

		return nil
	})

	s.NoError(err)
}

func (s *WeightedSuite) TestWithError() {
	baseCtx := context.Background()
	expectedHead := &testHead{c: 1}
	c := container.NewWeighted(expectedHead)
	step1 := make(chan struct{})
	step2 := make(chan struct{})
	ctx, cancel := context.WithCancel(baseCtx)

	go c.With(baseCtx, func(_ *testHead) error {
		close(step1)
		cancel()
		<-step2
		return nil
	})

	<-step1
	err := c.With(ctx, func(_ *testHead) error {
		return nil
	})
	close(step2)

	s.Error(err)
	s.Require().ErrorIs(err, context.Canceled)
}

func (s *WeightedSuite) TestClose() {
	baseCtx := context.Background()
	expectedHead := &testHead{c: 2}
	c := container.NewWeighted(expectedHead)

	err := c.Close()
	s.Require().NoError(err)

	actualHead := c.Get()
	s.Require().NotNil(actualHead)
	s.Equal(expectedHead.c, actualHead.c)

	err = c.Replace(baseCtx, &testHead{c: 3})
	s.Require().ErrorIs(err, locker.ErrSemaphoreClosed)

	err = c.With(baseCtx, func(h *testHead) error {
		if expectedHead.c != h.c {
			return fmt.Errorf("expectedHead(%d) not equal actual(%d)", expectedHead.c, h.c)
		}

		return nil
	})
	s.Require().ErrorIs(err, locker.ErrSemaphoreClosed)
}

//
// testHead
//

// testHead implementation [container.Head].
type testHead struct {
	c int64
}

// Concurrency implementation [container.Head].
func (h *testHead) Concurrency() int64 {
	return h.c
}
