package wal_test

import (
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/go-faker/faker/v4"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	"github.com/stretchr/testify/suite"
)

type SegmentWalReaderSuite struct {
	suite.Suite
}

func TestSegmentWalReaderSuite(t *testing.T) {
	suite.Run(t, new(SegmentWalReaderSuite))
}

func (s *SegmentWalReaderSuite) TestHappyPath() {
	buf := &bytes.Buffer{}
	data := []byte(faker.Paragraph())
	data = data[:(len(data)/10)*10]
	_, err := buf.Write(data)
	s.Require().NoError(err)

	swr := wal.NewSegmentWalReader(buf, newTestSegment)
	limiter := 0
	actual := make([]byte, 0, len(data))
	err = swr.ForEachSegment(func(rsm *testSegment) error {
		actual = append(actual, rsm.Bytes()...)

		// protect from infinite loop
		limiter++
		if limiter == 1000 {
			return errors.New("limiter")
		}

		return nil
	})
	s.Require().NoError(err)

	s.Equal(data, actual)
}

func (s *SegmentWalReaderSuite) TestForEachSegmentError() {
	buf := &bytes.Buffer{}
	data := []byte(faker.Paragraph())
	data = data[:(len(data)/10)*10]
	_, err := buf.Write(data)
	s.Require().NoError(err)

	swr := wal.NewSegmentWalReader(buf, newTestSegment)
	limiter := 0
	actual := make([]byte, 0, len(data))
	expectedError := errors.New("test error")
	err = swr.ForEachSegment(func(rsm *testSegment) error {
		actual = append(actual, rsm.Bytes()...)

		// protect from infinite loop
		limiter++
		if limiter == 1 {
			return expectedError
		}

		return nil
	})
	s.Require().ErrorIs(err, expectedError)
}

func (s *SegmentWalReaderSuite) TestForEachSegmentReadError() {
	buf := &bytes.Buffer{}
	data := []byte(faker.Paragraph())
	data = data[:(len(data)/10)*10]
	_, err := buf.Write(data)
	s.Require().NoError(err)

	expectedError := errors.New("test error")
	swr := wal.NewSegmentWalReader(buf, newTestSegmentWithError(expectedError))
	limiter := 0
	actual := make([]byte, 0, len(data))
	err = swr.ForEachSegment(func(rsm *testSegment) error {
		actual = append(actual, rsm.Bytes()...)

		// protect from infinite loop
		limiter++
		if limiter == 1 {
			return errors.New("another error")
		}

		return nil
	})
	s.Require().ErrorIs(err, expectedError)
}

//
// testSegment
//

// testSegment implements [ReadSegment].
type testSegment struct {
	buf []byte
	*ReadSegmentMock
}

// newTestSegment init new [testSegment].
func newTestSegment() *testSegment {
	s := &testSegment{
		buf: make([]byte, 10),
	}

	s.ReadSegmentMock = &ReadSegmentMock{
		ReadFromFunc: func(r io.Reader) (int64, error) {
			n, err := io.ReadFull(r, s.buf)
			return int64(n), err
		},
		ResetFunc: func() {
			for i := range s.buf {
				s.buf[i] = 0
			}
		},
	}

	return s
}

// newTestSegmentWithError init new [testSegment] with error.
func newTestSegmentWithError(err error) func() *testSegment {
	return func() *testSegment {
		s := &testSegment{
			buf: make([]byte, 10),
		}

		s.ReadSegmentMock = &ReadSegmentMock{
			ReadFromFunc: func(r io.Reader) (int64, error) {
				n, errRead := io.ReadFull(r, s.buf)
				if errRead != nil {
					return int64(n), errRead
				}

				return int64(n), err
			},
			ResetFunc: func() {
				for i := range s.buf {
					s.buf[i] = 0
				}
			},
		}

		return s
	}
}

// Bytes returns data.
func (s *testSegment) Bytes() []byte {
	return s.buf
}
