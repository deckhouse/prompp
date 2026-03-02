package promqlext_test

import (
	"context"
	"testing"
	"time"

	promqlext "github.com/prometheus/prometheus/promql/ext"
	"github.com/stretchr/testify/suite"
)

func TestReplaceNaN(t *testing.T) {
	suite.Run(t, new(ReplaceNaNSuite))
}

type ReplaceNaNSuite struct {
	BaseSuite
}

func (s *ReplaceNaNSuite) SetupSuite() {
	promqlext.RegisterOPReplaceNaN()
	s.BaseSuite.SetupSuite()
}

func (s *ReplaceNaNSuite) TestSingle() {
	ctx := context.Background()

	s.fillStorageByStrings(
		"1 5 __name__=foo,server=foo",
	)
	expected := `{server="foo"} => 17 @[180000]`

	query, err := s.engine.NewInstantQuery(
		ctx,
		s.storage,
		nil,
		"op_replace_nan(foo[3m], 17)",
		s.time("3m"),
	)
	s.Require().NoError(err)

	result := query.Exec(context.Background())
	s.Require().NoError(result.Err)
	s.Equal(expected, result.String())
}

func (s *ReplaceNaNSuite) TestMinimal() {
	ctx := context.Background()

	s.fillStorageByStrings(
		"1 5 __name__=foo,server=foo",
	)

	query, err := s.engine.NewInstantQuery(
		ctx,
		s.storage,
		nil,
		"op_replace_nan(foo[3m], 17, 60001)",
		s.time("3m"),
	)
	s.Require().NoError(err)

	result := query.Exec(context.Background())
	s.Require().NoError(result.Err)
	s.Zero(result.String())
}

func (s *ReplaceNaNSuite) TestMultiple() {
	ctx := context.Background()

	s.fillStorageByStrings(
		"0 5 __name__=foo,server=apple",
		"0 5 __name__=foo,server=orange",
		// "1 5 __name__=foo,server=apple",
		"1 0 __name__=foo,server=orange",
		// "2 5 __name__=foo,server=apple",
		"2 -42 __name__=foo,server=orange",
		// "3 5 __name__=foo,server=apple",
		"3 5 __name__=foo,server=orange",
		"4 3 __name__=foo,server=apple",
	)
	query, err := s.engine.NewRangeQuery(
		ctx,
		s.storage,
		nil,
		"op_replace_nan(foo[10m], 17)",
		s.time("2m"),
		s.time("6m"),
		time.Duration(time.Minute),
	)
	s.Require().NoError(err)

	result := query.Exec(context.Background())
	s.Require().NoError(result.Err)
	s.checkMatrix(result, map[string][]float64{
		// Minutes             2   3  4  5  6
		`{server="apple"}`:  {17, 17, 3, 3, 17},
		`{server="orange"}`: {-42, 5, 5, 17, 17},
	})
}

func (s *ReplaceNaNSuite) TestBackwardCompatibility() {
	ctx := context.Background()

	s.fillStorageByStrings(
		"0 5 __name__=foo,server=apple",
		"0 5 __name__=foo,server=orange",
		"1 0 __name__=foo,server=orange",
		"2 -42 __name__=foo,server=orange",
		"3 5 __name__=foo,server=orange",
		"4 3 __name__=foo,server=apple",
	)

	opQuery, opErr := s.engine.NewRangeQuery(
		ctx,
		s.storage,
		nil,
		"op_replace_nan(foo[10m], 17)",
		s.time("2m"),
		s.time("6m"),
		time.Minute,
	)
	okQuery, okErr := s.engine.NewRangeQuery(
		ctx,
		s.storage,
		nil,
		"ok_replace_nan(foo[10m], 17)",
		s.time("2m"),
		s.time("6m"),
		time.Duration(time.Minute),
	)
	opResult := opQuery.Exec(context.Background())
	okResult := okQuery.Exec(context.Background())

	s.Require().NoError(opErr)
	s.Require().NoError(okErr)
	s.Require().NoError(opResult.Err)
	s.Require().NoError(okResult.Err)
	s.Equal(opResult, okResult)
}
