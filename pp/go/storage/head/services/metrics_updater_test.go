package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
)

type MetricsUpdaterSuite struct {
	suite.Suite

	baseCtx context.Context
}

func TestMetricsUpdaterSuite(t *testing.T) {
	suite.Run(t, new(MetricsUpdaterSuite))
}

func (s *MetricsUpdaterSuite) SetupSuite() {
	s.baseCtx = context.Background()
}

func (s *MetricsUpdaterSuite) TestHappyPath() {
	s.T().Log("MetricsUpdaterSuite TestHappyPath TODO")
}
