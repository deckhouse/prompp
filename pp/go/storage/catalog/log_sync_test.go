package catalog

import (
	"io"
	"os"
)

// closeAfterEncode closes the snapshot file after encoding to make the final sync fail.
type closeAfterEncode struct {
	Encoder
}

func (e closeAfterEncode) EncodeTo(writer io.Writer, record *SerializedRecord) error {
	if err := e.Encoder.EncodeTo(writer, record); err != nil {
		return err
	}

	return writer.(*FileHandler).Close()
}

func (s *FileLogReplaceTestSuite) TestRewrite_FileSyncFailurePreservesSource() {
	before, err := os.ReadFile(s.filePath)
	s.Require().NoError(err)
	oldFile := s.fl.file
	encoder := s.fl.encoder
	s.fl.encoder = closeAfterEncode{Encoder: encoder}

	err = s.fl.ReWrite(&s.initial.SerializedRecord)
	s.fl.encoder = encoder

	s.Require().ErrorIs(err, os.ErrClosed)
	s.Same(oldFile, s.fl.file)
	after, err := os.ReadFile(s.filePath)
	s.Require().NoError(err)
	s.Equal(before, after)
	_, err = os.Stat(s.filePath + ".compacted")
	s.Require().ErrorIs(err, os.ErrNotExist)
	appended := newAppendTestRecord(3, 300)
	s.Require().NoError(s.fl.Write(&appended.SerializedRecord))
	s.reopen(LogFileVersionV2)
	s.requireRecords(s.initial, appended)
}
