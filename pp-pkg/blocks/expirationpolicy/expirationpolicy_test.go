package expirationpolicy_test

import (
	"testing"

	"github.com/oklog/ulid"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp-pkg/blocks/expirationpolicy"
	"github.com/prometheus/prometheus/pp/go/storage/catalog"
)

type ExpirationPolicySuite struct {
	suite.Suite
}

func TestExpirationPolicySuite(t *testing.T) {
	suite.Run(t, new(ExpirationPolicySuite))
}

func (s *ExpirationPolicySuite) TestHappyPath() {
	dir := s.T().TempDir()
	c := &testCatalog{
		onDiskSize: 100,
		list:       []*catalog.Record{},
	}
	opts := &expirationpolicy.Options{
		RetentionDuration: 100,
		DownsamplingMS:    100,
		MaxBytes:          100,
	}
	ep := expirationpolicy.NewExpirationPolicy[*testBlock](
		dir,
		c,
		opts,
		nil,
	)
	ep.BlocksToDelete(nil)
	s.Require().Empty(ep.BlocksToDelete(nil))
}

//
// testBlock
//

// testBlock is the test implementation of the [expirationpolicy.Block].
type testBlock struct {
	deletable           bool
	isDownsamplingBlock bool
	maxTime             int64
	size                int64
	ulid                ulid.ULID
}

// Deletable returns true if the block is deletable.
func (b *testBlock) Deletable() bool {
	return b.deletable
}

// IsDownsamplingBlock returns true if the block is a downsampling block.
func (b *testBlock) IsDownsamplingBlock() bool {
	return b.isDownsamplingBlock
}

// MaxTime returns the maximum time of the block.
func (b *testBlock) MaxTime() int64 {
	return b.maxTime
}

// Size returns the size of the block.
func (b *testBlock) Size() int64 {
	return b.size
}

// ULID returns the ULID of the block.
func (b *testBlock) ULID() ulid.ULID {
	return b.ulid
}

//
// testCatalog
//

// testCatalog is the test implementation of the [expirationpolicy.Catalog].
type testCatalog struct {
	onDiskSize int64
	list       []*catalog.Record
}

// OnDiskSize returns the on-disk size of the catalog.
func (c *testCatalog) OnDiskSize() int64 {
	return c.onDiskSize
}

// List returns the list of heads in the catalog.
func (c *testCatalog) List(
	func(record *catalog.Record) bool,
	func(lhs *catalog.Record, rhs *catalog.Record) bool,
) []*catalog.Record {
	return c.list
}
