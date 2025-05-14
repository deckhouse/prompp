//go:build !slicelabels && !dedupelabels && !stringlabels

package labels_test

import (
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/model/labels"
)

type LabelsSuite struct {
	suite.Suite
}

func TestLabelsSuite(t *testing.T) {
	suite.Run(t, new(LabelsSuite))
}

func (s *LabelsSuite) TestBytes() {
	expected := []byte{
		254, 95, 95, 110, 97, 109, 101, 95, 95, 255, 117, 98,
		101, 114, 110, 97, 109, 101, 255, 99, 104, 101, 255, 98,
		117, 114, 101, 99, 107, 255, 108, 111, 108, 255, 107, 101,
		107, 255, 122, 105, 109, 121, 97, 255, 114, 101, 99, 107,
	}

	lsA := labels.FromMap(map[string]string{
		"__name__": "ubername",
		"lol":      "kek",
		"che":      "bureck",
		"zimya":    "reck",
	})

	s.Equal(expected, lsA.Bytes(nil))
}
