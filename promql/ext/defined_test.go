package promqlext_test

import (
	"context"
	"testing"
	"time"

	promqlext "github.com/prometheus/prometheus/promql/ext"
	"github.com/stretchr/testify/suite"
)

func TestDefined(t *testing.T) {
	suite.Run(t, new(DefinedSuite))
}

type DefinedSuite struct {
	BaseSuite
}

func (s *DefinedSuite) SetupSuite() {
	promqlext.RegisterOPDefined()
	s.BaseSuite.SetupSuite()
}

func (s *DefinedSuite) TestSingle() {
	ctx := context.Background()

	s.fillStorageByStrings(
		"1 5 __name__=foo,server=foo",
	)
	expected := `{server="foo"} => 0 @[180000]`

	query, err := s.engine.NewInstantQuery(
		ctx,
		s.storage,
		nil,
		"op_defined(foo[3m])",
		s.time("3m"),
	)
	s.Require().NoError(err)

	result := query.Exec(context.Background())
	s.Require().NoError(result.Err)
	s.Equal(expected, result.String())
}

func (s *DefinedSuite) TestMultiple() {
	ctx := context.Background()

	s.fillStorageByStrings(
		"0 5 __name__=foo,server=apple",
		"0 5 __name__=foo,server=orange",
		// "1 5 __name__=foo,server=apple",
		"1 5 __name__=foo,server=orange",
		// "2 5 __name__=foo,server=apple",
		"2 5 __name__=foo,server=orange",
		// "3 5 __name__=foo,server=apple",
		"3 5 __name__=foo,server=orange",
		"4 5 __name__=foo,server=apple",
	)
	query, err := s.engine.NewRangeQuery(
		ctx,
		s.storage,
		nil,
		"op_defined(foo[10m])",
		s.time("2m"),
		s.time("6m"),
		time.Duration(time.Minute),
	)
	s.Require().NoError(err)

	result := query.Exec(context.Background())
	s.Require().NoError(result.Err)
	s.checkMatrix(result, map[string][]float64{
		// Minutest           2  3  4  5  6
		`{server="apple"}`:  {0, 0, 1, 1, 0},
		`{server="orange"}`: {1, 1, 1, 0, 0},
	})
}

func (s *DefinedSuite) TestBackwardCompatibility() {
	ctx := context.Background()

	s.fillStorageByStrings(
		"0 5 __name__=foo,server=apple",
		"0 5 __name__=foo,server=orange",
		"1 5 __name__=foo,server=orange",
		"2 5 __name__=foo,server=orange",
		"3 5 __name__=foo,server=orange",
		"4 5 __name__=foo,server=apple",
	)

	opQuery, opErr := s.engine.NewRangeQuery(
		ctx,
		s.storage,
		nil,
		"op_defined(foo[10m])",
		s.time("2m"),
		s.time("6m"),
		time.Minute,
	)
	okQuery, okErr := s.engine.NewRangeQuery(
		ctx,
		s.storage,
		nil,
		"ok_defined(foo[10m])",
		s.time("2m"),
		s.time("6m"),
		time.Minute,
	)
	opResult := opQuery.Exec(context.Background())
	okResult := okQuery.Exec(context.Background())

	s.Require().NoError(opErr)
	s.Require().NoError(okErr)
	s.Require().NoError(opResult.Err)
	s.Require().NoError(okResult.Err)
	s.Equal(opResult, okResult)
}
