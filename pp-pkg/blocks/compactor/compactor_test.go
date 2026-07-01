package compactor_test

import (
	"sync"
	"testing"

	"github.com/go-kit/log"
	"github.com/oklog/ulid"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp-pkg/blocks/block"
	"github.com/prometheus/prometheus/pp-pkg/blocks/compactor"
)

type CompactorSuite struct {
	suite.Suite
}

func TestCompactorSuite(t *testing.T) {
	suite.Run(t, new(CompactorSuite))
}

func (s *CompactorSuite) TestCompactorCompactUsesPlanAndSource() {
	wantUID := ulid.MustNew(1, nil)
	fake := &fakeCompactor{
		plan:   []string{"01AAA", "01BBB"},
		result: []ulid.ULID{wantUID},
	}

	c := compactor.NewCompactorWithLeveledCompactor("/tmp/data", fake, log.NewNopLogger(), nil)

	uids, err := c.Compact([]*block.Block{nil, nil})
	s.Require().NoError(err)
	s.Require().Len(uids, 1)
	s.Require().Equal([]ulid.ULID{wantUID}, uids)
	s.Require().True(fake.compactCalled)
	s.Require().Equal("/tmp/data", fake.compactDest)
	s.Require().Equal([]string{"01AAA", "01BBB"}, fake.compactDirs)
	s.Require().Len(fake.compactOpen, 2)
}

func (s *CompactorSuite) TestCompactorCompactNoPlanIsNoop() {
	fake := &fakeCompactor{plan: nil}
	c := compactor.NewCompactorWithLeveledCompactor("/tmp/data", fake, log.NewNopLogger(), nil)

	uids, err := c.Compact([]*block.Block{})
	s.Require().NoError(err)
	s.Require().Empty(uids)
	s.Require().Equal(1, fake.planCalls)
	s.Require().False(fake.compactCalled)
}

//
// fakeCompactor
//

// fakeCompactor is a fake implementation of the [LeveledCompactor] interface.
type fakeCompactor struct {
	mu sync.Mutex

	plan   []string
	result []ulid.ULID

	planCalls int

	compactCalls  int
	compactCalled bool
	compactDest   string
	compactDirs   []string
	compactOpen   []*block.Block
}

// Compact compacts the blocks in the given directories into the given destination directory.
func (f *fakeCompactor) Compact(dest string, dirs []string, open []*block.Block) ([]ulid.ULID, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.compactCalls++
	f.compactCalled = true
	f.compactDest = dest
	f.compactDirs = append([]string(nil), dirs...)
	f.compactOpen = append([]*block.Block(nil), open...)
	return append([]ulid.ULID(nil), f.result...), nil
}

// Plan plans a compaction of the blocks in the given directory.
func (f *fakeCompactor) Plan(string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	f.planCalls++
	return append([]string(nil), f.plan...), nil
}
