package model_test

import (
	"bytes"
	"net/http"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp-pkg/handler/model"
)

type RefillProcessingStatusSuite struct {
	suite.Suite
}

func TestRefillProcessingStatusSuite(t *testing.T) {
	suite.Run(t, new(RefillProcessingStatusSuite))
}

func (s *RefillProcessingStatusSuite) TestWrite() {
	status := model.RefillProcessingStatus{
		Code:    200,
		Message: "ok",
	}

	rw := &responseWriter{}

	err := status.Write(rw)
	s.Require().NoError(err)

	s.Equal(status.Code, rw.statusCode)
	s.Equal(status.Message, rw.buf.String())
}

// responseWriter implementation http.ResponseWriter.
type responseWriter struct {
	header     http.Header
	buf        bytes.Buffer
	statusCode int
}

// Header implementation http.ResponseWriter.
func (rw *responseWriter) Header() http.Header {
	return rw.header
}

// Write implementation http.ResponseWriter.
func (rw *responseWriter) Write(b []byte) (int, error) {
	return rw.buf.Write(b)
}

// WriteHeader implementation http.ResponseWriter.
func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
}
