package querier

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

type ChunksSeriesSetTestSuite struct {
	suite.Suite
}

func TestChunksSeriesSetTestSuite(t *testing.T) {
	suite.Run(t, new(ChunksSeriesSetTestSuite))
}
