package remotewriter

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	config_util "github.com/prometheus/common/config"
	"github.com/prometheus/common/model"
	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/config"
)

type HTTP2Suite struct {
	suite.Suite

	server *httptest.Server
	proto  string
}

func TestHTTP2Suite(t *testing.T) {
	suite.Run(t, new(HTTP2Suite))
}

// SetupTest starts a TLS server able to serve both HTTP/2 and HTTP/1.1, so the protocol is
// negotiated by the client alone.
func (s *HTTP2Suite) SetupTest() {
	s.proto = ""
	s.server = httptest.NewUnstartedServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		s.proto = r.Proto
	}))
	s.server.EnableHTTP2 = true
	s.server.StartTLS()
}

func (s *HTTP2Suite) TearDownTest() {
	s.server.Close()
	HTTP2Enabled = true
}

func (s *HTTP2Suite) TestHTTP2IsNegotiatedByDefault() {
	s.Require().NoError(s.send())
	s.Equal("HTTP/2.0", s.proto)
}

func (s *HTTP2Suite) TestDisabledFlagKeepsHTTP11() {
	HTTP2Enabled = false

	s.Require().NoError(s.send())
	s.Equal("HTTP/1.1", s.proto)
}

// send delivers a message to the test server through a client built by [createClient].
func (s *HTTP2Suite) send() error {
	serverURL, err := url.Parse(s.server.URL)
	s.Require().NoError(err)

	client, err := createClient(DestinationConfig{
		RemoteWriteConfig: config.RemoteWriteConfig{
			Name:          "dst",
			URL:           &config_util.URL{URL: serverURL},
			RemoteTimeout: model.Duration(10 * time.Second),
			HTTPClientConfig: config_util.HTTPClientConfig{
				// the destination config asks for HTTP/2, as it does by default
				EnableHTTP2: true,
				TLSConfig:   config_util.TLSConfig{InsecureSkipVerify: true},
			},
		},
	})
	s.Require().NoError(err)

	return newProtobufWriter(client).Write(s.T().Context(), []byte("message"))
}
