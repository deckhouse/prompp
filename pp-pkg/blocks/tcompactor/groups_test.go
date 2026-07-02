package tcompactor_test

import (
	"testing"

	"github.com/go-kit/log"
	"github.com/stretchr/testify/suite"
	"github.com/thanos-io/thanos/pkg/block/metadata"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/pp-pkg/blocks/block"
	"github.com/prometheus/prometheus/pp-pkg/blocks/tcompactor"
)

type GrouperSuite struct {
	suite.Suite
}

func TestGrouperSuite(t *testing.T) {
	suite.Run(t, new(GrouperSuite))
}

func (s *GrouperSuite) TestOneGroup() {
	blks := []*block.Block{{}, {}}
	ls := map[string]string{"foo": "bar"}
	blks[0].Metadata().Thanos.Labels = ls
	blks[1].Metadata().Thanos.Labels = ls

	groups, err := tcompactor.NewDefaultGrouper(log.NewNopLogger(), nil, false).Groups(blks)
	s.Require().NoError(err)
	s.Len(groups, 1)
}

func (s *GrouperSuite) TestTwoGroupsByLabels() {
	blks := []*block.Block{{}, {}}
	ls1 := map[string]string{"foo": "bar"}
	ls2 := map[string]string{"foo": "baz"}
	blks[0].Metadata().Thanos.Labels = ls1
	blks[1].Metadata().Thanos.Labels = ls2

	groups, err := tcompactor.NewDefaultGrouper(log.NewNopLogger(), nil, false).Groups(blks)
	s.Require().NoError(err)
	s.Len(groups, 2)
}

func (s *GrouperSuite) TestTwoGroupsByResolution() {
	blks := []*block.Block{{}, {}}
	ls := map[string]string{"foo": "bar"}
	blks[0].Metadata().Thanos.Labels = ls
	blks[1].Metadata().Thanos.Labels = ls
	blks[0].Metadata().Thanos.Downsample.Resolution = 10
	blks[1].Metadata().Thanos.Downsample.Resolution = 20

	groups, err := tcompactor.NewDefaultGrouper(log.NewNopLogger(), nil, false).Groups(blks)
	s.Require().NoError(err)
	s.Len(groups, 2)
}

//
// GroupSuite
//

type GroupSuite struct {
	suite.Suite
}

func TestGroupSuite(t *testing.T) {
	suite.Run(t, new(GroupSuite))
}

func (s *GroupSuite) OverlappingBlocks() {
	noopCounter := promauto.With(nil).NewCounter(prometheus.CounterOpts{Name: "noop", Help: "noop"})

	group := tcompactor.NewGroup(
		log.NewNopLogger(),
		s.T().Name(),
		labels.FromStrings("foo", "bar"),
		0,
		noopCounter, noopCounter, noopCounter, noopCounter, noopCounter,
		false,
	)
	err := group.AppendMeta(&metadata.Meta{
		Thanos: metadata.Thanos{
			Labels: map[string]string{"foo": "bar"},
		},
	})
	s.Require().NoError(err)
}
