package services_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/suite"
)

type RotatorSuite struct {
	suite.Suite

	baseCtx context.Context
}

func TestRotatorSuite(t *testing.T) {
	suite.Run(t, new(RotatorSuite))
}

func (s *RotatorSuite) SetupSuite() {
	s.baseCtx = context.Background()
}

func (s *RotatorSuite) TestHappyPath() {
	s.T().Log("RotatorSuite TestHappyPath TODO")
}
