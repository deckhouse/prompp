package keeper

import (
	"container/heap"
	"testing"

	"github.com/stretchr/testify/suite"
)

type headForTest struct {
	id string
}

func newHeadForTest(id string) *headForTest {
	return &headForTest{id: id}
}

func (h *headForTest) ID() string {
	return h.id
}

func (*headForTest) Close() error {
	return nil
}

type sortedSlice = headSortedSlice[*headForTest]

type KeeperSuite struct {
	suite.Suite
	keeper *Keeper[headForTest, *headForTest]
}

func TestKeeperSuite(t *testing.T) {
	suite.Run(t, new(KeeperSuite))
}

func (s *KeeperSuite) getHeads() []*headForTest {
	heads := make([]*headForTest, 0, len(s.keeper.heads))
	for _, head := range s.keeper.heads {
		heads = append(heads, head.head)
	}
	return heads
}

func (s *KeeperSuite) TestAdd() {
	// Arrange
	s.keeper = NewKeeper[headForTest](2)

	// Act
	_ = s.keeper.Add(newHeadForTest("d"), 4)
	_ = s.keeper.Add(newHeadForTest("c"), 3)
	err := s.keeper.Add(newHeadForTest("b"), 2)

	// Assert
	s.Equal(sortedSlice{
		{head: newHeadForTest("c"), createdAt: 3},
		{head: newHeadForTest("d"), createdAt: 4},
	}, s.keeper.heads)
	s.Equal(err, ErrorNoSlots)
}

func (s *KeeperSuite) TestAddWithReplaceNoReplace() {
	// Arrange
	s.keeper = NewKeeper[headForTest](2)

	// Act
	_ = s.keeper.Add(newHeadForTest("d"), 4)
	_ = s.keeper.Add(newHeadForTest("c"), 3)
	err := s.keeper.AddWithReplace(newHeadForTest("b"), 3)

	// Assert
	s.Equal(sortedSlice{
		{head: newHeadForTest("c"), createdAt: 3},
		{head: newHeadForTest("d"), createdAt: 4},
	}, s.keeper.heads)
	s.Equal(err, ErrorNoSlots)
}

func (s *KeeperSuite) TestAddWithReplace() {
	// Arrange
	s.keeper = NewKeeper[headForTest](2)

	// Act
	_ = s.keeper.Add(newHeadForTest("d"), 4)
	_ = s.keeper.Add(newHeadForTest("c"), 3)
	err := s.keeper.AddWithReplace(newHeadForTest("b"), 4)

	// Assert
	s.Equal(sortedSlice{
		{head: newHeadForTest("b"), createdAt: 4},
		{head: newHeadForTest("d"), createdAt: 4},
	}, s.keeper.heads)
	s.NoError(err)
}

func (s *KeeperSuite) TestRemove() {
	// Arrange
	const Slots = 5

	s.keeper = NewKeeper[headForTest](Slots)
	_ = s.keeper.Add(newHeadForTest("a"), 1)
	_ = s.keeper.Add(newHeadForTest("b"), 2)
	_ = s.keeper.Add(newHeadForTest("c"), 3)
	_ = s.keeper.Add(newHeadForTest("d"), 4)
	_ = s.keeper.Add(newHeadForTest("e"), 5)

	// Act
	s.keeper.Remove([]*headForTest{newHeadForTest("a"), newHeadForTest("c"), newHeadForTest("e")})

	// Assert
	s.Equal(sortedSlice{
		{head: newHeadForTest("b"), createdAt: 2},
		{head: newHeadForTest("d"), createdAt: 4},
	}, s.keeper.heads)
	s.Equal(Slots, cap(s.keeper.heads))
}

func TestXxx(t *testing.T) {
	ss := sortedSlice{
		{head: newHeadForTest("b"), createdAt: 2},
		{head: newHeadForTest("d"), createdAt: 4},
	}

	t.Log(ss)

	ss[0].head = newHeadForTest("b")
	ss[0].createdAt = 5

	t.Log(ss)

	heap.Fix(&ss, 0)

	t.Log(ss)
}
