package localstorageobserver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/log"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/suite"
)

type LocalStorageObserverSuite struct {
	suite.Suite
}

func TestLocalStorageObserverSuite(t *testing.T) {
	suite.Run(t, new(LocalStorageObserverSuite))
}

func (s *LocalStorageObserverSuite) TestHappyPath() {
	reg := prometheus.NewRegistry()
	dir := s.T().TempDir()
	size := 1024
	observer := NewLocalStorageObserver(
		dir,
		nil,
		log.NewNopLogger(),
		reg,
	)

	err := os.WriteFile(filepath.Join(dir, "some-file"), make([]byte, size), 0o644)
	s.Require().NoError(err)

	observer.Observe(s.T().Context())

	mfs, err := reg.Gather()
	s.Require().NoError(err)

	var totalSize int
	for _, mf := range mfs {
		if mf.GetName() == "prompp_localstorage_unknown_bytes" {
			for _, m := range mf.GetMetric() {
				totalSize += int(m.GetGauge().GetValue())
			}
		}
	}

	s.Require().Equal(totalSize, size)
}
