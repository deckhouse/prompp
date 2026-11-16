package remote_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	config_util "github.com/prometheus/common/config"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/model/labels"
	ppremote "github.com/prometheus/prometheus/pp-pkg/storage/remote"
	"github.com/prometheus/prometheus/prompb"
	"github.com/prometheus/prometheus/storage"
	"github.com/prometheus/prometheus/storage/remote"
	"github.com/prometheus/prometheus/tsdb/chunkenc"
)

// remoteReadTestMetric identifies the series served by the streamed test endpoint.
const remoteReadTestMetric = "remote_read_test"

// RemoteReadSuite checks configured frame limits through the public remote-read query path.
type RemoteReadSuite struct {
	suite.Suite
	remoteRead *ppremote.RemoteRead
	readConfig config.RemoteReadConfig
}

func TestRemoteReadSuite(t *testing.T) {
	suite.Run(t, new(RemoteReadSuite))
}

func (s *RemoteReadSuite) SetupTest() {
	chunk := chunkenc.NewXORChunk()
	appender, err := chunk.Appender()
	s.Require().NoError(err)
	appender.Append(1000, 42)

	response := prompb.ChunkedReadResponse{ChunkedSeries: []*prompb.ChunkedSeries{{
		Labels: []prompb.Label{{Name: labels.MetricName, Value: remoteReadTestMetric}},
		Chunks: []prompb.Chunk{{
			MinTimeMs: 1000,
			MaxTimeMs: 1000,
			Type:      prompb.Chunk_XOR,
			Data:      chunk.Bytes(),
		}},
	}}}
	frame, err := response.Marshal()
	s.Require().NoError(err)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-streamed-protobuf; proto=prometheus.ChunkedReadResponse")
		_, writeErr := remote.NewChunkedWriter(w, w.(http.Flusher)).Write(frame)
		s.NoError(writeErr)
	}))
	s.T().Cleanup(server.Close)

	serverURL, err := url.Parse(server.URL)
	s.Require().NoError(err)

	s.readConfig = config.DefaultRemoteReadConfig
	s.readConfig.URL = &config_util.URL{URL: serverURL}
	s.readConfig.ReadRecent = true
	s.readConfig.ChunkedReadLimit = uint64(len(frame))
	s.remoteRead = ppremote.NewRemoteRead(nil, nil)
	s.T().Cleanup(func() { s.Require().NoError(s.remoteRead.Close()) })
}

func (s *RemoteReadSuite) TestReadsStreamedFrameWithinConfiguredLimit() {
	// Arrange
	seriesSet := s.selectSeries()

	// Act
	advanced := seriesSet.Next()

	// Assert
	s.Require().True(advanced, "warnings: %v", seriesSet.Warnings())
	s.Equal(labels.FromStrings(labels.MetricName, remoteReadTestMetric), seriesSet.At().Labels())
	iterator := seriesSet.At().Iterator(nil)
	s.Require().Equal(chunkenc.ValFloat, iterator.Next())
	timestamp, value := iterator.At()
	s.Equal(int64(1000), timestamp)
	s.Equal(float64(42), value)
	s.Equal(chunkenc.ValNone, iterator.Next())
	s.Require().NoError(iterator.Err())
	s.False(seriesSet.Next())
	s.Require().NoError(seriesSet.Err())
	s.Empty(seriesSet.Warnings())
}

func (s *RemoteReadSuite) TestRejectsStreamedFrameOverConfiguredLimit() {
	// Arrange
	s.readConfig.ChunkedReadLimit = 1
	seriesSet := s.selectSeries()

	// Act
	advanced := seriesSet.Next()

	// Assert
	s.False(advanced)
	s.Require().NoError(seriesSet.Err())
	warnings := seriesSet.Warnings().AsErrors()
	s.Require().Len(warnings, 1)
	s.ErrorContains(warnings[0], "message size exceeded the limit 1 bytes")
}

func (s *RemoteReadSuite) selectSeries() storage.SeriesSet {
	s.T().Helper()
	s.Require().NoError(s.remoteRead.ApplyConfig(&config.Config{
		RemoteReadConfigs: []*config.RemoteReadConfig{&s.readConfig},
	}))
	querier, err := s.remoteRead.Querier(0, 2000)
	s.Require().NoError(err)
	s.T().Cleanup(func() { s.Require().NoError(querier.Close()) })

	return querier.Select(s.T().Context(), false, nil,
		labels.MustNewMatcher(labels.MatchEqual, labels.MetricName, remoteReadTestMetric))
}
