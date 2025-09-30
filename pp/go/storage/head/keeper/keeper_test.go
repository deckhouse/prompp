package keeper

import (
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

//
// testRemovedHeadNotifier
//

// testRemovedHeadNotifier implementation [RemovedHeadNotifier].
type testRemovedHeadNotifier struct {
	count int
}

// Notify implementation [RemovedHeadNotifier].
func (n *testRemovedHeadNotifier) Notify() {
	n.count++
}

type sortedSlice = headSortedSlice[*headForTest]

type KeeperSuite struct {
	suite.Suite
	keeper *Keeper[headForTest, *headForTest]
}

func TestKeeperSuite(t *testing.T) {
	suite.Run(t, new(KeeperSuite))
}

func (s *KeeperSuite) TestAdd() {
	// Arrange
	count := 0
	addTrigger := func() { count++ }
	removedHeadNotifier := &testRemovedHeadNotifier{}
	s.keeper = NewKeeper[headForTest](2, addTrigger, removedHeadNotifier)

	// Act
	_ = s.keeper.Add(newHeadForTest("d"), 4)
	_ = s.keeper.Add(newHeadForTest("c"), 3)
	err := s.keeper.Add(newHeadForTest("b"), 2)

	// Assert
	s.Equal(sortedSlice{
		{head: newHeadForTest("c"), createdAt: 3},
		{head: newHeadForTest("d"), createdAt: 4},
	}, s.keeper.heads)
	s.Equal(2, count)
	s.Equal(err, ErrorNoSlots)
}

func (s *KeeperSuite) TestAddWithReplaceNoReplace() {
	// Arrange
	count := 0
	addTrigger := func() { count++ }
	removedHeadNotifier := &testRemovedHeadNotifier{}
	s.keeper = NewKeeper[headForTest](2, addTrigger, removedHeadNotifier)

	// Act
	_ = s.keeper.Add(newHeadForTest("d"), 4)
	_ = s.keeper.Add(newHeadForTest("c"), 3)
	err := s.keeper.AddWithReplace(newHeadForTest("b"), 3)

	// Assert
	s.Equal(sortedSlice{
		{head: newHeadForTest("c"), createdAt: 3},
		{head: newHeadForTest("d"), createdAt: 4},
	}, s.keeper.heads)
	s.Equal(2, count)
	s.Equal(err, ErrorNoSlots)
}

func (s *KeeperSuite) TestAddWithReplace() {
	// Arrange
	count := 0
	addTrigger := func() { count++ }
	removedHeadNotifier := &testRemovedHeadNotifier{}
	s.keeper = NewKeeper[headForTest](2, addTrigger, removedHeadNotifier)

	// Act
	_ = s.keeper.Add(newHeadForTest("d"), 4)
	_ = s.keeper.Add(newHeadForTest("c"), 3)
	err := s.keeper.AddWithReplace(newHeadForTest("b"), 4)

	// Assert
	s.Equal(sortedSlice{
		{head: newHeadForTest("b"), createdAt: 4},
		{head: newHeadForTest("d"), createdAt: 4},
	}, s.keeper.heads)
	s.Equal(3, count)
	s.NoError(err)
}

func (s *KeeperSuite) TestRemove() {
	// Arrange
	const Slots = 5

	count := 0
	addTrigger := func() { count++ }
	removedHeadNotifier := &testRemovedHeadNotifier{}
	s.keeper = NewKeeper[headForTest](Slots, addTrigger, removedHeadNotifier)
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
	s.Equal(5, count)
	s.Equal(Slots, cap(s.keeper.heads))
	s.Equal(1, removedHeadNotifier.count)
}
