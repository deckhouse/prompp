package keeper

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type HeadConvertingQueueSuite struct {
	suite.Suite
	queue HeadConvertingQueue[string]
}

func TestHeadConvertingQueueSuite(t *testing.T) {
	suite.Run(t, new(HeadConvertingQueueSuite))
}

func (s *HeadConvertingQueueSuite) popItems() []string {
	items := make([]string, 0, len(s.queue.Heads()))
	for i := len(s.queue.heads); i > 0; i-- {
		items = append(items, s.queue.Pop())
	}
	return items
}

func (s *HeadConvertingQueueSuite) TestPush() {
	// Arrange
	s.queue = NewHeadConvertingQueue[string](4)

	// Act
	_ = s.queue.Push("d", 4)
	_ = s.queue.Push("c", 3)
	_ = s.queue.Push("b", 2)

	// Assert
	s.Equal([]string{"b", "c", "d"}, s.popItems())
}

func (s *HeadConvertingQueueSuite) TestPushFullFill() {
	// Arrange
	s.queue = NewHeadConvertingQueue[string](4)

	// Act
	_ = s.queue.Push("d", 4)
	_ = s.queue.Push("c", 3)
	_ = s.queue.Push("b", 2)
	_ = s.queue.Push("a", 1)

	// Assert
	s.Equal([]string{"a", "b", "c", "d"}, s.popItems())
}

func (s *HeadConvertingQueueSuite) TestOverrideAtPush() {
	// Arrange
	s.queue = NewHeadConvertingQueue[string](3)

	// Act
	_ = s.queue.Push("12", 12)
	_ = s.queue.Push("10", 10)
	_ = s.queue.Push("1", 1)
	_ = s.queue.Push("11", 11)

	// Assert
	s.Equal([]string{"10", "11", "12"}, s.popItems())
}

func (s *HeadConvertingQueueSuite) TestPushError() {
	// Arrange
	s.queue = NewHeadConvertingQueue[string](3)

	// Act
	_ = s.queue.Push("a", 1)
	_ = s.queue.Push("b", 2)
	_ = s.queue.Push("c", 3)
	err := s.queue.Push("d", 1)

	// Assert
	s.Equal([]string{"a", "b", "c"}, s.popItems())
	s.Equal(ErrorHeadConvertingQueueIsFull, err)
}
