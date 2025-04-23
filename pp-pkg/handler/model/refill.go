package model

import (
	"net/http"
)

//
// RefillProcessingStatus
//

// RefillProcessingStatus status of processing refill.
type RefillProcessingStatus struct {
	Code    int
	Message string
}

// Write to writer RefillProcessingStatus.
func (s *RefillProcessingStatus) Write(writer http.ResponseWriter) error {
	writer.WriteHeader(s.Code)
	_, err := writer.Write([]byte(s.Message))
	return err
}
