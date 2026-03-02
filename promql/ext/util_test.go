package promqlext_test

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/timestamp"
	"github.com/prometheus/prometheus/promql"
	"github.com/prometheus/prometheus/util/teststorage"

	"github.com/stretchr/testify/suite"
)

type BaseSuite struct {
	suite.Suite

	engine  *promql.Engine
	storage *teststorage.TestStorage
}

func (s *BaseSuite) SetupSuite() {
	opts := promql.EngineOpts{
		Logger:     nil,
		Reg:        nil,
		MaxSamples: 10000,
		Timeout:    10 * time.Second,
		// По умолчанию этот интервал равен 5m
		LookbackDelta: 59 * time.Second,
	}
	s.engine = promql.NewEngine(opts)
}

func (s *BaseSuite) SetupTest() {
	s.storage = teststorage.New(s.T())
}

func (s *BaseSuite) TearDownTest() {
	_ = s.storage.Close()
}

// Добавляет в s.storage записи из строк в формате
//
// timestamp(minutes)  value  labels
// 12                  42.678 __name__=foo,server=carrot
func (s *BaseSuite) fillStorageByStrings(records ...string) {
	a := s.storage.Appender(context.Background())
	var (
		ts       int64
		value    float64
		labelSet string
	)
	for i := range records {
		_, err := fmt.Sscanf(records[i], "%d %f %s", &ts, &value, &labelSet)
		s.Require().NoError(err)
		var labelPairs []string
		for _, pair := range strings.Split(labelSet, ",") {
			labelPairs = append(labelPairs, strings.Split(pair, "=")...)
		}
		_, _ = a.Append(0, labels.FromStrings(labelPairs...), ts*60*1000, value)
	}
	s.Require().NoError(a.Commit())
}

func (s *BaseSuite) time(duration string) time.Time {
	d, err := time.ParseDuration(duration)
	s.Require().NoError(err)
	return timestamp.Time(int64(d / time.Millisecond))
}

func (s *BaseSuite) checkMatrix(res *promql.Result, expected map[string][]float64) {
	actual, err := res.Matrix()
	s.Require().NoError(err)
	for _, series := range actual {
		values := make([]float64, len(series.Floats))
		for i := range values {
			values[i] = series.Floats[i].F
		}
		key := series.Metric.String()
		s.Equalf(expected[key], values, "series %s mismatch", key)
		delete(expected, key)
	}
	s.Empty(expected)
}
