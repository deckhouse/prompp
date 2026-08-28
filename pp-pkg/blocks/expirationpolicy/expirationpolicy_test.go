package expirationpolicy_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/oklog/ulid"
	"github.com/prometheus/client_golang/prometheus"
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
	ep := s.newEP(&expirationpolicy.Options{
		RetentionDuration: 100,
		MaxBytes:          100,
	}, s.newCatalog(100), nil)

	ep.BlocksToDelete(nil)
	s.Require().Empty(ep.BlocksToDelete(nil))
}

func (s *ExpirationPolicySuite) TestBlocksToDelete_EmptyInput() {
	ep := s.newEP(&expirationpolicy.Options{MaxBytes: 100}, s.newCatalog(0), nil)

	s.Nil(ep.BlocksToDelete(nil))
	s.Nil(ep.BlocksToDelete([]*testBlock{}))
}

func (s *ExpirationPolicySuite) TestBlocksToDelete_DeletableBlocks() {
	b1 := s.newDeletableBlock(1, 1000, 10)
	b2 := s.newBlock(2, 900, 10)

	ep := s.newEP(&expirationpolicy.Options{
		RetentionDuration: 0,
		MaxBytes:          0,
	}, s.newCatalog(0), nil)

	actual := ep.BlocksToDelete([]*testBlock{b1, b2})
	s.assertDeletable(actual, b1.ULID())
}

func (s *ExpirationPolicySuite) TestBlocksToDelete_NoRetentionTriggered() {
	b1 := s.newBlock(1, 2000, 10)
	b2 := s.newBlock(2, 1500, 10)
	b3 := s.newBlock(3, 900, 10)

	ep := s.newEP(&expirationpolicy.Options{
		RetentionDuration: 10_000,
		MaxBytes:          1_000_000,
	}, s.newCatalog(0), nil)

	actual := ep.BlocksToDelete([]*testBlock{b1, b2, b3})
	s.Empty(actual)
}

func (s *ExpirationPolicySuite) TestBlocksToDelete_TimeRetention() {
	for _, tc := range []struct {
		name      string
		blocks    []*testBlock
		retention int64
		expected  []ulid.ULID
	}{
		{
			name: "delta greater than retention duration",
			blocks: []*testBlock{
				s.newBlock(1, 900, 10),
				s.newBlock(2, 1500, 10),
				s.newBlock(3, 2000, 10),
			},
			retention: 1000,
			expected:  []ulid.ULID{s.newBlock(1, 0, 0).ULID()},
		},
		{
			name: "delta equal to retention duration",
			blocks: []*testBlock{
				s.newBlock(1, 900, 10),
				s.newBlock(2, 1500, 10),
				s.newBlock(3, 2000, 10),
			},
			retention: 500,
			expected:  []ulid.ULID{s.newBlock(1, 0, 0).ULID(), s.newBlock(2, 0, 0).ULID()},
		},
		{
			name: "retention duration disabled",
			blocks: []*testBlock{
				s.newBlock(1, 900, 10),
				s.newBlock(2, 1500, 10),
				s.newBlock(3, 2000, 10),
			},
			retention: 0,
			expected:  nil,
		},
	} {
		s.Run(tc.name, func() {
			reg := prometheus.NewRegistry()
			ep := s.newEP(&expirationpolicy.Options{
				RetentionDuration: tc.retention,
				MaxBytes:          1_000_000,
			}, s.newCatalog(0), reg)

			actual := ep.BlocksToDelete(tc.blocks)
			if len(tc.expected) == 0 {
				s.Empty(actual)
				s.Equal(0.0, s.metricValue(reg, "prometheus_tsdb_time_retentions_total"))
				return
			}

			s.assertDeletable(actual, tc.expected...)
			s.Equal(1.0, s.metricValue(reg, "prometheus_tsdb_time_retentions_total"))
		})
	}
}

func (s *ExpirationPolicySuite) TestBlocksToDelete_DownsampledExemptFromTimeRetention() {
	oldDownsampled := s.newDownsamplingBlock(1, 100, 10)
	rawNewest := s.newBlock(2, 2000, 10)
	rawMiddle := s.newBlock(3, 1500, 10)

	ep := s.newEP(&expirationpolicy.Options{
		RetentionDuration: 1000,
		MaxBytes:          1_000_000,
	}, s.newCatalog(0), nil)

	actual := ep.BlocksToDelete([]*testBlock{oldDownsampled, rawNewest, rawMiddle})
	s.Empty(actual)
}

func (s *ExpirationPolicySuite) TestBlocksToDelete_TimePrecedenceOverSize() {
	b1 := s.newBlock(1, 900, 100)
	b2 := s.newBlock(2, 1500, 100)
	b3 := s.newBlock(3, 2000, 100)

	reg := prometheus.NewRegistry()
	ep := s.newEP(&expirationpolicy.Options{
		RetentionDuration: 1000,
		MaxBytes:          100,
	}, s.newCatalog(0), reg)

	actual := ep.BlocksToDelete([]*testBlock{b1, b2, b3})
	s.assertDeletable(actual, b1.ULID(), b2.ULID())
	s.Equal(1.0, s.metricValue(reg, "prometheus_tsdb_time_retentions_total"))
	s.Equal(1.0, s.metricValue(reg, "prometheus_tsdb_size_retentions_total"))
}

func (s *ExpirationPolicySuite) TestBlocksToDelete_SizeRetention() {
	for _, tc := range []struct {
		name           string
		opts           *expirationpolicy.Options
		catalog        *testCatalog
		blocks         []*testBlock
		expected       []ulid.ULID
		expectedMetric float64
	}{
		{
			name:    "max bytes disabled",
			opts:    &expirationpolicy.Options{MaxBytes: 0},
			catalog: s.newCatalog(0),
			blocks: []*testBlock{
				s.newBlock(1, 2000, 100),
				s.newBlock(2, 1000, 100),
			},
			expected:       nil,
			expectedMetric: 0,
		},
		{
			name:    "catalog size exceeds limit",
			opts:    &expirationpolicy.Options{MaxBytes: 100},
			catalog: s.newCatalog(150),
			blocks: []*testBlock{
				s.newBlock(1, 2000, 0),
				s.newBlock(2, 1000, 0),
			},
			expected:       []ulid.ULID{s.newBlock(1, 0, 0).ULID(), s.newBlock(2, 0, 0).ULID()},
			expectedMetric: 1,
		},
		{
			name:    "raw blocks exceed limit",
			opts:    &expirationpolicy.Options{MaxBytes: 100},
			catalog: s.newCatalog(80),
			blocks: []*testBlock{
				s.newBlock(1, 1000, 10),
				s.newBlock(2, 2000, 15),
			},
			expected:       []ulid.ULID{s.newBlock(1, 0, 0).ULID()},
			expectedMetric: 1,
		},
		{
			name:    "downsampled blocks exceed limit after raw",
			opts:    &expirationpolicy.Options{MaxBytes: 100},
			catalog: s.newCatalog(80),
			blocks: []*testBlock{
				s.newDownsamplingBlock(1, 1000, 5),
				s.newBlock(2, 2000, 15),
				s.newDownsamplingBlock(3, 3000, 3),
			},
			expected:       []ulid.ULID{s.newBlock(1, 0, 0).ULID()},
			expectedMetric: 1,
		},
	} {
		s.Run(tc.name, func() {
			reg := prometheus.NewRegistry()
			ep := s.newEP(tc.opts, tc.catalog, reg)

			actual := ep.BlocksToDelete(tc.blocks)
			s.assertDeletable(actual, tc.expected...)
			s.Equal(tc.expectedMetric, s.metricValue(reg, "prometheus_tsdb_size_retentions_total"))
		})
	}
}

func (s *ExpirationPolicySuite) TestBlocksToDelete_UnsortedInputUsesNewestFirst() {
	// Input is intentionally out of order; retention should still delete the oldest raw block.
	bOldest := s.newBlock(1, 900, 10)
	bMiddle := s.newBlock(2, 1500, 10)
	bNewest := s.newBlock(3, 2000, 10)

	reg := prometheus.NewRegistry()
	ep := s.newEP(&expirationpolicy.Options{
		RetentionDuration: 1000,
		MaxBytes:          1_000_000,
	}, s.newCatalog(0), reg)

	actual := ep.BlocksToDelete([]*testBlock{bMiddle, bOldest, bNewest})
	s.assertDeletable(actual, bOldest.ULID())
	s.Equal(1.0, s.metricValue(reg, "prometheus_tsdb_time_retentions_total"))
}

func (s *ExpirationPolicySuite) TestBlocksToDelete_ExtraSizeTakesPrecedence() {
	b1 := s.newBlock(1, 2000, 0)
	b2 := s.newBlock(2, 1000, 0)

	ep := expirationpolicy.NewExpirationPolicy[*testBlock](
		s.T().TempDir(),
		s.newCatalog(0),
		&expirationpolicy.Options{
			MaxBytes:  100,
			ExtraSize: func() int64 { return 150 },
		},
		nil,
	)

	actual := ep.BlocksToDelete([]*testBlock{b1, b2})
	s.assertDeletable(actual, b1.ULID(), b2.ULID())
}

func (s *ExpirationPolicySuite) TestBlocksToDelete_WithoutCatalog() {
	b1 := s.newBlock(1, 2000, 60)
	b2 := s.newBlock(2, 1000, 60)

	ep := expirationpolicy.NewExpirationPolicy[*testBlock](
		s.T().TempDir(),
		nil,
		&expirationpolicy.Options{
			MaxBytes:  100,
			ExtraSize: func() int64 { return 0 },
		},
		nil,
	)

	actual := ep.BlocksToDelete([]*testBlock{b1, b2})
	s.assertDeletable(actual, b2.ULID())
}

func (s *ExpirationPolicySuite) TestCatalogHeadsSize() {
	dir := s.T().TempDir()

	id1 := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	id2 := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	s.createHeadDir(dir, id1, 30)
	s.createHeadDir(dir, id2, 20)

	record1 := catalog.NewRecordWithData(id1, 1, 0, 0, 0, false, 0, catalog.StatusActive, nil)
	record2 := catalog.NewRecordWithData(id2, 1, 0, 0, 0, false, 0, catalog.StatusActive, nil)

	s.Run("no catalog", func() {
		ep := expirationpolicy.NewExpirationPolicy[*testBlock](
			dir,
			nil,
			&expirationpolicy.Options{},
			nil,
		)
		s.Zero(ep.CatalogHeadsSize())
	})

	s.Run("catalog only", func() {
		ep := expirationpolicy.NewExpirationPolicy[*testBlock](
			dir,
			s.newCatalog(50),
			&expirationpolicy.Options{},
			nil,
		)
		s.Equal(int64(50), ep.CatalogHeadsSize())
	})

	s.Run("catalog and one head", func() {
		ep := expirationpolicy.NewExpirationPolicy[*testBlock](
			dir,
			s.newCatalog(50, record1),
			&expirationpolicy.Options{},
			nil,
		)
		s.Equal(int64(80), ep.CatalogHeadsSize())
	})

	s.Run("catalog and two heads", func() {
		ep := expirationpolicy.NewExpirationPolicy[*testBlock](
			dir,
			s.newCatalog(50, record1, record2),
			&expirationpolicy.Options{},
			nil,
		)
		s.Equal(int64(100), ep.CatalogHeadsSize())
	})
}

//
// helpers
//

func (s *ExpirationPolicySuite) newEP(
	opts *expirationpolicy.Options,
	c *testCatalog,
	reg prometheus.Registerer,
) *expirationpolicy.ExpirationPolicy[*testBlock] {
	return expirationpolicy.NewExpirationPolicy[*testBlock](
		s.T().TempDir(),
		c,
		opts,
		reg,
	)
}

func (*ExpirationPolicySuite) newBlock(
	id uint64,
	maxTime, size int64,
) *testBlock {
	return &testBlock{
		maxTime: maxTime,
		size:    size,
		ulid:    ulid.MustNew(id, nil),
	}
}

func (*ExpirationPolicySuite) newDeletableBlock(
	id uint64,
	maxTime, size int64,
) *testBlock {
	return &testBlock{
		maxTime:   maxTime,
		size:      size,
		ulid:      ulid.MustNew(id, nil),
		deletable: true,
	}
}

func (*ExpirationPolicySuite) newDownsamplingBlock(
	id uint64,
	maxTime, size int64,
) *testBlock {
	return &testBlock{
		maxTime:             maxTime,
		size:                size,
		ulid:                ulid.MustNew(id, nil),
		isDownsamplingBlock: true,
	}
}

func (*ExpirationPolicySuite) newCatalog(onDiskSize int64, records ...*catalog.Record) *testCatalog {
	return &testCatalog{
		onDiskSize: onDiskSize,
		list:       records,
	}
}

func (s *ExpirationPolicySuite) createHeadDir(dir string, id uuid.UUID, size int) {
	s.T().Helper()

	headDir := filepath.Join(dir, id.String())
	s.Require().NoError(os.MkdirAll(headDir, 0o755))
	s.Require().NoError(os.WriteFile(filepath.Join(headDir, "data"), make([]byte, size), 0o600))
}

func (s *ExpirationPolicySuite) assertDeletable(actual map[ulid.ULID]struct{}, expected ...ulid.ULID) {
	s.T().Helper()

	if len(expected) == 0 {
		s.Empty(actual)
		return
	}

	s.Require().Len(actual, len(expected))

	for _, id := range expected {
		_, ok := actual[id]
		s.True(ok, "expected block %s to be deletable", id.String())
	}
}

func (s *ExpirationPolicySuite) metricValue(reg *prometheus.Registry, name string) float64 {
	s.T().Helper()

	mfs, err := reg.Gather()
	s.Require().NoError(err)

	for _, mf := range mfs {
		if mf.GetName() == name {
			return mf.GetMetric()[0].GetCounter().GetValue()
		}
	}

	s.FailNow("metric not found: " + name)

	return 0
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
	func(lhs, rhs *catalog.Record) bool,
) []*catalog.Record {
	return c.list
}
