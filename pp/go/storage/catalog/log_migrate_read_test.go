package catalog

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"github.com/jonboulle/clockwork"
)

const (
	// migrationDamageHeaders identifies damage to both v3 file header slots.
	migrationDamageHeaders = "headers"
	// migrationDamageLong identifies a v3 file exceeding its fixed size.
	migrationDamageLong = "long"
	// migrationDamageShort identifies a truncated v3 file.
	migrationDamageShort = "short"
	// migrationSourceUnsupported identifies a log with an unsupported version.
	migrationSourceUnsupported = "unsupported"
)

func (s *V3FileTestSuite) TestMigration_CompactedFallbackForDamagedV3() {
	for _, target := range []uint64{LogFileVersionV2, LogFileVersionV3} {
		for _, source := range []uint64{LogFileVersionV2, LogFileVersionV3} {
			for _, damage := range []string{migrationDamageHeaders, migrationDamageShort, migrationDamageLong} {
				s.Run(fmt.Sprintf("target%d/source%d/%s", target, source, damage), func() {
					path := filepath.Join(s.T().TempDir(), "head.log")
					s.writeDamagedV3Log(path, damage)
					initial := newAppendTestRecord(1, 100)
					s.writeRecoveryLog(path+".compacted", source, initial)

					fl, err := NewFileLog(path, target)

					s.Require().NoError(err)
					s.T().Cleanup(func() { _ = fl.Close() })
					c, err := New(clockwork.NewFakeClock(), fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil)
					s.Require().NoError(err)
					got, err := c.Get(initial.ID())
					s.Require().NoError(err)
					s.Equal(initial.NumberOfShards(), got.NumberOfShards())
					s.Equal(initial.CreatedAt(), got.CreatedAt())
					s.Len(c.List(nil, nil), 1)
					_, err = os.Stat(path + ".compacted")
					s.Require().ErrorIs(err, os.ErrNotExist)
					appended, err := c.Create(2)
					s.Require().NoError(err)
					s.Require().NoError(fl.Close())
					fl, err = NewFileLog(path, target)
					s.Require().NoError(err)
					c, err = New(clockwork.NewFakeClock(), fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil)
					s.Require().NoError(err)
					_, err = c.Get(initial.ID())
					s.Require().NoError(err)
					_, err = c.Get(appended.ID())
					s.Require().NoError(err)
				})
			}
		}
	}
}

func (s *V3FileTestSuite) TestMigration_CompactedFallbackIsRenamed() {
	initial := newAppendTestRecord(1, 100)
	s.Require().NoError(s.fl.Write(&initial.SerializedRecord))
	s.Require().NoError(s.fl.Close())
	s.Require().NoError(os.Rename(s.filePath, s.filePath+".compacted"))
	s.Require().NoError(os.WriteFile(s.filePath, nil, logFilePerm))

	var err error
	s.fl, err = NewFileLogV3(s.filePath)
	s.Require().NoError(err)
	s.requireRecords(initial)

	_, err = os.Stat(s.filePath + ".compacted")
	s.Require().ErrorIs(err, os.ErrNotExist)
	s.Require().NoError(s.fl.Write(&newAppendTestRecord(2, 200).SerializedRecord))
}

func (s *V3FileTestSuite) TestMigration_DamagedFallbackPreservesBothFiles() {
	for _, target := range []uint64{LogFileVersionV2, LogFileVersionV3} {
		s.Run(fmt.Sprintf("target%d", target), func() {
			path := filepath.Join(s.T().TempDir(), "head.log")
			before := s.writeDamagedV3Log(path, migrationDamageHeaders)
			fallbackBefore := s.writeDamagedV3Log(path+".compacted", migrationDamageShort)

			fl, err := NewFileLog(path, target)

			s.Require().ErrorIs(err, ErrInvalidV3Header)
			s.Require().ErrorIs(err, ErrInvalidV3FileSize)
			s.Nil(fl)
			after, err := os.ReadFile(path)
			s.Require().NoError(err)
			s.Equal(before, after)
			fallbackAfter, err := os.ReadFile(path + ".compacted")
			s.Require().NoError(err)
			s.Equal(fallbackBefore, fallbackAfter)
		})
	}
}

func (s *V3FileTestSuite) TestMigration_DamagedV3WithoutFallbackPreservesSource() {
	for _, target := range []uint64{LogFileVersionV2, LogFileVersionV3} {
		for _, damage := range []string{migrationDamageHeaders, migrationDamageShort, migrationDamageLong} {
			s.Run(fmt.Sprintf("target%d/%s", target, damage), func() {
				path := filepath.Join(s.T().TempDir(), "head.log")
				before := s.writeDamagedV3Log(path, damage)

				fl, err := NewFileLog(path, target)

				s.Require().Error(err)
				s.Nil(fl)
				after, err := os.ReadFile(path)
				s.Require().NoError(err)
				s.Equal(before, after)
			})
		}
	}
}

func (s *V3FileTestSuite) TestMigration_FailedRewritePreservesSource() {
	initial := newAppendTestRecord(1, 100)
	s.Require().NoError(s.fl.Write(&initial.SerializedRecord))
	s.Require().NoError(s.fl.Close())

	before, err := os.ReadFile(s.filePath)
	s.Require().NoError(err)
	s.Require().NoError(os.Mkdir(s.filePath+".compacted", 0o700))

	s.fl, err = NewFileLogV2(s.filePath)
	s.Require().NoError(err)

	_, err = New(clockwork.NewFakeClock(), s.fl, DefaultIDGenerator{}, DefaultMaxLogFileSize, nil)
	s.Require().Error(err)
	s.True(s.fl.NeedsRewrite())

	after, err := os.ReadFile(s.filePath)
	s.Require().NoError(err)
	s.Equal(before, after)
	s.Require().ErrorIs(s.fl.Write(&initial.SerializedRecord), ErrLogNeedsRewrite)
}

func (s *V3FileTestSuite) TestMigration_HardReadErrorPreservesFallback() {
	path := filepath.Join(s.T().TempDir(), "head.log")
	s.Require().NoError(os.Mkdir(path, 0o700))
	s.writeRecoveryLog(path+".compacted", LogFileVersionV3, newAppendTestRecord(1, 100))
	before, err := os.ReadFile(path + ".compacted")
	s.Require().NoError(err)

	fl, err := NewFileLogV3(path)

	s.Require().ErrorIs(err, syscall.EISDIR)
	s.Nil(fl)
	after, err := os.ReadFile(path + ".compacted")
	s.Require().NoError(err)
	s.Equal(before, after)
}

func (s *V3FileTestSuite) TestMigration_OpeningDoesNotChangeSource() {
	path := filepath.Join(s.T().TempDir(), "head.log")
	before, err := os.ReadFile("testdata/headv2.log")
	s.Require().NoError(err)
	s.Require().NoError(os.WriteFile(path, before, logFilePerm))

	fl, err := NewFileLogV3(path)
	s.Require().NoError(err)
	s.T().Cleanup(func() { _ = fl.Close() })

	s.True(fl.NeedsRewrite())

	after, err := os.ReadFile(path)
	s.Require().NoError(err)
	s.Equal(before, after)
	s.Require().ErrorIs(fl.Write(&newAppendTestRecord(1, 100).SerializedRecord), ErrLogNeedsRewrite)
}

func (s *V3FileTestSuite) TestMigration_UnreadableLogClassification() {
	for _, source := range []string{"missing", "empty", migrationSourceUnsupported} {
		s.Run(source, func() {
			path := filepath.Join(s.T().TempDir(), "head.log")
			if source != "missing" {
				var data []byte
				if source == migrationSourceUnsupported {
					data = []byte{0xff}
				}

				s.Require().NoError(os.WriteFile(path, data, logFilePerm))
			}

			fl, err := openFileLog(path, path, LogFileVersionV3, syncDir)

			s.Require().ErrorIs(err, ErrUnreadableLogFile)
			s.Nil(fl)
		})
	}
}

func (s *V3FileTestSuite) writeDamagedV3Log(path, damage string) []byte {
	s.T().Helper()
	s.writeRecoveryLog(path, LogFileVersionV3, newAppendTestRecord(9, 900))

	switch damage {
	case migrationDamageHeaders:
		data, err := os.ReadFile(path)
		s.Require().NoError(err)
		data[fileHeaderGenerationOffsetV3] ^= 1
		data[fileHeaderSlotSizeV3+fileHeaderGenerationOffsetV3] ^= 1
		s.Require().NoError(os.WriteFile(path, data, logFilePerm))
	case migrationDamageShort:
		s.Require().NoError(os.Truncate(path, fileHeaderAreaSizeV3))
	case migrationDamageLong:
		s.Require().NoError(os.Truncate(path, fileSizeV3+1))
	default:
		s.FailNow("unknown log damage", damage)
	}

	data, err := os.ReadFile(path)
	s.Require().NoError(err)

	return data
}

func (s *V3FileTestSuite) writeRecoveryLog(path string, version uint64, record *Record) {
	s.T().Helper()
	fl, err := NewFileLog(path, version)
	s.Require().NoError(err)
	s.Require().NoError(fl.Write(&record.SerializedRecord))
	s.Require().NoError(fl.Close())
}
