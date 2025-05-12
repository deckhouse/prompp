package model

import (
	"encoding/binary"
	"io"
)

//
// StreamSegmentProcessingStatus
//

// StreamSegmentProcessingStatus status of processing segment.
type StreamSegmentProcessingStatus struct {
	SegmentID uint32
	Code      uint16
	Message   string
	Timestamp int64
}

// Encode status to slice byte.
func (s *StreamSegmentProcessingStatus) Encode() []byte {
	buf := make([]byte, 8+4+2+4+len(s.Message))
	binary.LittleEndian.PutUint64(buf, uint64(s.Timestamp))
	binary.LittleEndian.PutUint32(buf[8:], s.SegmentID)
	binary.LittleEndian.PutUint16(buf[12:], s.Code)
	binary.LittleEndian.PutUint32(buf[14:], uint32(len(s.Message)))
	copy(buf[18:], s.Message)
	return buf
}

// EncodeTo to writer RefillProcessingStatus.
func (s *StreamSegmentProcessingStatus) EncodeTo(writer io.Writer) error {
	_, err := writer.Write(s.Encode())
	return err
}

// DecodeFrom read from reader and decode.
func (s *StreamSegmentProcessingStatus) DecodeFrom(reader io.Reader) error {
	header := make([]byte, 8+4+2+4)
	if _, err := io.ReadFull(reader, header); err != nil {
		return err
	}

	s.Timestamp = int64(binary.LittleEndian.Uint64(header[:8]))
	s.SegmentID = binary.LittleEndian.Uint32(header[8:12])
	s.Code = binary.LittleEndian.Uint16(header[12:14])
	messageLen := binary.LittleEndian.Uint32(header[14:18])
	if messageLen == 0 {
		return nil
	}

	message := make([]byte, messageLen)
	if _, err := io.ReadFull(reader, message); err != nil {
		return err
	}

	s.Message = string(message)

	return nil
}
