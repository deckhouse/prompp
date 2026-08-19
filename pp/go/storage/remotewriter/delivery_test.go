package remotewriter

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/jonboulle/clockwork"
	"github.com/prometheus/client_golang/prometheus/testutil"
	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/storage/remote"

	"github.com/prometheus/prometheus/config"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/logger"
	"github.com/prometheus/prometheus/pp/go/storage/remotewriter/mock"
)

// roundTripperFunc is a [http.RoundTripper] over a func.
type roundTripperFunc func(req *http.Request) (*http.Response, error)

// RoundTrip implementation [http.RoundTripper].
func (fn roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

// writeClientStub is a [remote.WriteClient] recording the arguments of the last Store call.
type writeClientStub struct {
	attempt int
	data    []byte
}

// Store implementation [remote.WriteClient].
func (c *writeClientStub) Store(_ context.Context, req []byte, attempt int) (remote.WriteResponseStats, error) {
	c.data = req
	c.attempt = attempt
	return remote.WriteResponseStats{}, nil
}

// Name implementation [remote.WriteClient].
func (*writeClientStub) Name() string { return "stub" }

// Endpoint implementation [remote.WriteClient].
func (*writeClientStub) Endpoint() string { return "http://test.com" }

type DeliverySuite struct {
	suite.Suite
}

func TestDeliverySuite(t *testing.T) {
	suite.Run(t, new(DeliverySuite))
}

// newRoundTripper builds a [deliveryRoundTripper] with its own metrics over the underlying round tripper.
func (*DeliverySuite) newRoundTripper(underlyingRT http.RoundTripper) *deliveryRoundTripper {
	return newDeliveryRoundTripper(underlyingRT, newDestinationMetrics("test", "http://test.com"))
}

func (s *DeliverySuite) TestRoundTripperSetsHeaderFromContext() {
	var actualKey string
	rt := s.newRoundTripper(roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		actualKey = req.Header.Get(IdempotencyKeyHeader)
		return &http.Response{StatusCode: http.StatusOK}, nil
	}))

	expectedKey := newDeliveryTarget("dst", "http://test.com", "head-42").messageIdempotencyKey(7, 1)
	req, err := http.NewRequestWithContext(
		withDeliveryMarks(s.T().Context(), expectedKey, 0),
		http.MethodPost,
		"http://test.com",
		http.NoBody,
	)
	s.Require().NoError(err)

	_, err = rt.RoundTrip(req)
	s.Require().NoError(err)
	s.Equal(expectedKey, actualKey)
	s.Equal("dst/head-42/7/1", actualKey)
}

func (s *DeliverySuite) TestRoundTripperKeepsHeaderUnsetWithoutKey() {
	headerIsSet := true
	rt := s.newRoundTripper(roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		_, headerIsSet = req.Header[IdempotencyKeyHeader]
		return &http.Response{StatusCode: http.StatusOK}, nil
	}))

	req, err := http.NewRequestWithContext(s.T().Context(), http.MethodPost, "http://test.com", http.NoBody)
	s.Require().NoError(err)

	_, err = rt.RoundTrip(req)
	s.Require().NoError(err)
	s.False(headerIsSet)
}

func (s *DeliverySuite) TestProtobufWriterPassesRetryAttempt() {
	client := &writeClientStub{}
	writer := newProtobufWriter(client)

	s.Require().NoError(writer.Write(withDeliveryMarks(s.T().Context(), "dst/head-42/7/0", 3), []byte("message")))
	s.Equal(3, client.attempt)

	s.Require().NoError(writer.Write(s.T().Context(), []byte("message")))
	s.Equal(0, client.attempt)
}

// TestClientSendsDeliveryHeaders checks that both marks reach the wire through the client built by createClient.
func (s *DeliverySuite) TestClientSendsDeliveryHeaders() {
	var actualKey, actualAttempt string
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		actualKey = r.Header.Get(IdempotencyKeyHeader)
		actualAttempt = r.Header.Get(RetryAttemptHeader)
	}))
	defer server.Close()

	client, _, err := s.newClient(server.URL)
	s.Require().NoError(err)

	err = newProtobufWriter(client).Write(
		withDeliveryMarks(s.T().Context(), "dst/head-42/7/1", 2),
		[]byte("message"),
	)
	s.Require().NoError(err)
	s.Equal("dst/head-42/7/1", actualKey)
	s.Equal("2", actualAttempt)
}

// TestClientCountsConnections checks that an established connection and its reuse are told apart.
func (s *DeliverySuite) TestClientCountsConnections() {
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer server.Close()

	client, metrics, err := s.newClient(server.URL)
	s.Require().NoError(err)
	writer := newProtobufWriter(client)

	s.Require().NoError(writer.Write(s.T().Context(), []byte("message")))
	s.Equal(float64(1), testutil.ToFloat64(metrics.connectionsTotal.WithLabelValues(connectionNew)))
	s.Equal(float64(0), testutil.ToFloat64(metrics.connectionsTotal.WithLabelValues(connectionReused)))

	// the client drains and closes the response body, so the connection is back in the idle pool
	s.Require().NoError(writer.Write(s.T().Context(), []byte("message")))
	s.Equal(float64(1), testutil.ToFloat64(metrics.connectionsTotal.WithLabelValues(connectionNew)))
	s.Equal(float64(1), testutil.ToFloat64(metrics.connectionsTotal.WithLabelValues(connectionReused)))
}

// newClient builds a write client for the endpoint together with the metrics it reports to.
func (*DeliverySuite) newClient(endpoint string) (remote.WriteClient, *DestinationMetrics, error) {
	endpointURL, err := url.Parse(endpoint)
	if err != nil {
		return nil, nil, err
	}

	metrics := newDestinationMetrics("test", endpoint)
	client, err := createClient(DestinationConfig{
		RemoteWriteConfig: config.RemoteWriteConfig{
			Name:          "dst",
			URL:           &config_util.URL{URL: endpointURL},
			RemoteTimeout: model.Duration(10 * time.Second),
		},
	}, metrics)

	return client, metrics, err
}

// TestSendMessageRepeatsKeyOnRetry checks that a replayed message carries the key of its first delivery
// attempt and the number of the attempt it is, and that the failed attempt is reported with its details.
//
//revive:disable-next-line:function-length // long but readable
func (s *DeliverySuite) TestSendMessageRepeatsKeyOnRetry() {
	clock := clockwork.NewRealClock()
	queueConfig := config.QueueConfig{
		MinShards:         1,
		MaxShards:         2,
		MaxSamplesPerSend: 10,
		SampleAgeLimit:    model.Duration(time.Minute),
	}

	var mtx sync.Mutex
	actualMarks := make(map[deliveryMarks]int)
	var errorLogs []string
	firstAttemptFailed := false

	logger.Errorf = func(format string, args ...any) {
		mtx.Lock()
		defer mtx.Unlock()

		errorLogs = append(errorLogs, fmt.Sprintf(format, args...))
	}
	s.T().Cleanup(logger.Unset)

	protobufWriter := &mock.ProtobufWriterMock{
		WriteFunc: func(ctx context.Context, data []byte) error {
			mtx.Lock()
			defer mtx.Unlock()

			actualMarks[deliveryMarksFromContext(ctx)]++

			// fail the first delivery of the first message to force a replay of exactly that message
			if string(data) == "message-0" && !firstAttemptFailed {
				firstAttemptFailed = true
				return remote.RecoverableError{}
			}

			return nil
		},
	}

	it, err := newIterator(
		clock,
		queueConfig,
		nil,
		&mock.TargetSegmentIDSetCloserMock{
			SetTargetSegmentIDFunc: func(uint32) error { return nil },
			CloseFunc:              func() error { return nil },
		},
		0,
		10*time.Second,
		protobufWriter,
		newDestinationMetrics("test", "http://remote.test/api/v1/write"),
		newDeliveryTarget("dst", "http://remote.test/api/v1/write", "head-42"),
	)
	s.Require().NoError(err)

	maxTimestamp := clock.Now().UnixMilli()
	msg := &cppbridge.RWMessageList{
		MaxTimestamp:    maxTimestamp,
		TargetSegmentID: 7,
		Messages: []cppbridge.RWMessage{
			{Buffer: []byte("message-0"), MaxTimestamp: maxTimestamp, SampleCount: 1},
			{Buffer: []byte("message-1"), MaxTimestamp: maxTimestamp, SampleCount: 1},
		},
	}

	s.Require().NoError(it.SendMessage(s.T().Context(), msg))
	s.True(firstAttemptFailed)
	s.Equal(
		map[deliveryMarks]int{
			{idempotencyKey: "dst/head-42/7/0", retryAttempt: 0}: 1,
			{idempotencyKey: "dst/head-42/7/1", retryAttempt: 0}: 1,
			// the same key, the next attempt: the replay of the failed message
			{idempotencyKey: "dst/head-42/7/0", retryAttempt: 1}: 1,
		},
		actualMarks,
	)

	s.Require().Len(errorLogs, 1)
	s.Contains(errorLogs[0], "url=http://remote.test/api/v1/write")
	s.Contains(errorLogs[0], "idempotency_key=dst/head-42/7/0")
	s.Contains(errorLogs[0], "attempt=0")
	s.Contains(errorLogs[0], "bytes=9") // len("message-0")
	s.Contains(errorLogs[0], "samples=1")
	s.Regexp(`duration=\d`, errorLogs[0])
}
