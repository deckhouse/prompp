// Copyright 2019 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package promql

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/go-kit/log"
	"github.com/grafana/regexp"
	"github.com/stretchr/testify/require"
)

type queryLogWriter struct {
	written int
	err     error
}

func (w *queryLogWriter) Write(p []byte) (int, error) {
	if w.err != nil {
		return 0, w.err
	}
	w.written += len(p)
	return len(p), nil
}

func (*queryLogWriter) Sync() error {
	return nil
}

type shortQueryLogWriter struct{}

func (*shortQueryLogWriter) Write(p []byte) (int, error) {
	return len(p) - 1, nil
}

func (*shortQueryLogWriter) Sync() error {
	return nil
}

type syncErrorQueryLogWriter struct {
	queryLogWriter
	syncErr error
}

func (w *syncErrorQueryLogWriter) Sync() error {
	return w.syncErr
}

func TestQueryLogging(t *testing.T) {
	fileAsBytes := make([]byte, 4096)
	queryLogger := ActiveQueryTracker{
		mmappedFile:  fileAsBytes,
		logger:       nil,
		getNextIndex: make(chan int, 4),
	}

	queryLogger.generateIndices(4)
	veryLongString := "MassiveQueryThatNeverEndsAndExceedsTwoHundredBytesWhichIsTheSizeOfEntrySizeAndShouldThusBeTruncatedAndIamJustGoingToRepeatTheSameCharactersAgainProbablyBecauseWeAreStillOnlyHalfWayDoneOrMaybeNotOrMaybeMassiveQueryThatNeverEndsAndExceedsTwoHundredBytesWhichIsTheSizeOfEntrySizeAndShouldThusBeTruncatedAndIamJustGoingToRepeatTheSameCharactersAgainProbablyBecauseWeAreStillOnlyHalfWayDoneOrMaybeNotOrMaybeMassiveQueryThatNeverEndsAndExceedsTwoHundredBytesWhichIsTheSizeOfEntrySizeAndShouldThusBeTruncatedAndIamJustGoingToRepeatTheSameCharactersAgainProbablyBecauseWeAreStillOnlyHalfWayDoneOrMaybeNotOrMaybeMassiveQueryThatNeverEndsAndExceedsTwoHundredBytesWhichIsTheSizeOfEntrySizeAndShouldThusBeTruncatedAndIamJustGoingToRepeatTheSameCharactersAgainProbablyBecauseWeAreStillOnlyHalfWayDoneOrMaybeNotOrMaybeMassiveQueryThatNeverEndsAndExceedsTwoHundredBytesWhichIsTheSizeOfEntrySizeAndShouldThusBeTruncatedAndIamJustGoingToRepeatTheSameCharactersAgainProbablyBecauseWeAreStillOnlyHalfWayDoneOrMaybeNotOrMaybe"
	queries := []string{
		"TestQuery",
		veryLongString,
		"",
		"SpecialCharQuery{host=\"2132132\", id=123123}",
	}

	trimmedLongString := trimStringByBytes(veryLongString, entrySize-40)
	want := []string{
		`^{"query":"TestQuery","timestamp_sec":\d+}\x00*,$`,
		`^{"query":"` + trimmedLongString + `","timestamp_sec":\d+}\x00*,$`,
		`^{"query":"","timestamp_sec":\d+}\x00*,$`,
		`^{"query":"SpecialCharQuery{host=\\"2132132\\", id=123123}","timestamp_sec":\d+}\x00*,$`,
	}

	// Check for inserts of queries.
	for i := range 4 {
		start := 1 + i*entrySize
		end := start + entrySize

		queryLogger.Insert(context.Background(), queries[i])

		have := string(fileAsBytes[start:end])
		require.True(t, regexp.MustCompile(want[i]).MatchString(have),
			"Query not written correctly: %s", queries[i])
	}

	// Check if all queries have been deleted.
	for i := range 4 {
		queryLogger.Delete(1 + i*entrySize)
	}
	require.True(t, regexp.MustCompile(`^\x00+$`).Match(fileAsBytes[1:1+entrySize*4]),
		"All queries not deleted properly. Want only null bytes \\x00")
}

func TestIndexReuse(t *testing.T) {
	queryBytes := make([]byte, 1+3*entrySize)
	queryLogger := ActiveQueryTracker{
		mmappedFile:  queryBytes,
		logger:       nil,
		getNextIndex: make(chan int, 3),
	}

	queryLogger.generateIndices(3)
	queryLogger.Insert(context.Background(), "TestQuery1")
	queryLogger.Insert(context.Background(), "TestQuery2")
	queryLogger.Insert(context.Background(), "TestQuery3")

	queryLogger.Delete(1 + entrySize)
	queryLogger.Delete(1)
	newQuery2 := "ThisShouldBeInsertedAtIndex2"
	newQuery1 := "ThisShouldBeInsertedAtIndex1"
	queryLogger.Insert(context.Background(), newQuery2)
	queryLogger.Insert(context.Background(), newQuery1)

	want := []string{
		`^{"query":"ThisShouldBeInsertedAtIndex1","timestamp_sec":\d+}\x00*,$`,
		`^{"query":"ThisShouldBeInsertedAtIndex2","timestamp_sec":\d+}\x00*,$`,
		`^{"query":"TestQuery3","timestamp_sec":\d+}\x00*,$`,
	}

	// Check all bytes and verify new query was inserted at index 2
	for i := 0; i < 3; i++ {
		start := 1 + i*entrySize
		end := start + entrySize

		have := queryBytes[start:end]
		require.True(t, regexp.MustCompile(want[i]).Match(have),
			"Index not reused properly.")
	}
}

func TestMMapFile(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "mmappedFile")
	const data = "ab"

	fileAsBytes, closer, err := getMMappedFile(fpath, 2, nil)
	require.NoError(t, err)
	copy(fileAsBytes, data)
	require.NoError(t, closer.Close())

	f, err := os.Open(fpath)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = f.Close()
	})

	bytes := make([]byte, 4)
	n, err := f.Read(bytes)
	require.NoError(t, err, "Unexpected error while reading file.")
	require.Equal(t, 2, n)
	require.Equal(t, []byte(data), bytes[:2], "Mmap failed")
}

func TestAllocateQueryLogFile(t *testing.T) {
	writer := &queryLogWriter{}
	require.NoError(t, allocateQueryLogFile(writer, 100_000))
	require.Equal(t, 100_000, writer.written)
}

func TestAllocateQueryLogFileReturnsWriteErrors(t *testing.T) {
	require.ErrorIs(t, allocateQueryLogFile(&queryLogWriter{err: os.ErrPermission}, 1), os.ErrPermission)
	require.ErrorIs(t, allocateQueryLogFile(&shortQueryLogWriter{}, 1), io.ErrShortWrite)
}

func TestAllocateQueryLogFileReturnsSyncError(t *testing.T) {
	require.ErrorIs(t, allocateQueryLogFile(&syncErrorQueryLogWriter{syncErr: os.ErrPermission}, 1), os.ErrPermission)
}

func TestNewActiveQueryTrackerReturnsError(t *testing.T) {
	localStoragePath := filepath.Join(t.TempDir(), "not-a-directory")
	require.NoError(t, os.WriteFile(localStoragePath, nil, 0o666))

	logger := log.NewNopLogger()
	queryLogger, err := NewActiveQueryTracker(localStoragePath, 1, logger)
	require.Error(t, err)
	require.Nil(t, queryLogger)
}

func TestTrimStringByBytes(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		size     int
		expected string
	}{
		{
			name:     "normal ASCII string",
			input:    "hello",
			size:     3,
			expected: "hel",
		},
		{
			name:     "no trimming needed",
			input:    "hi",
			size:     10,
			expected: "hi",
		},
		{
			name:     "UTF-8 multibyte character boundary",
			input:    "日本", // 6 bytes (3 bytes per character)
			size:     4,
			expected: "日", // trims back to complete character boundary
		},
		{
			name:     "invalid UTF-8 continuation-only bytes",
			input:    string([]byte{0x80, 0x81, 0x82, 0x83, 0x84}), // only continuation bytes
			size:     4,
			expected: "",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NotPanics(t, func() {
				result := trimStringByBytes(tc.input, tc.size)
				require.Equal(t, tc.expected, result)
			})
		})
	}
}

func TestParseBrokenJSON(t *testing.T) {
	for _, tc := range []struct {
		b []byte

		ok  bool
		out string
	}{
		{
			b: []byte(""),
		},
		{
			b: []byte("\x00\x00"),
		},
		{
			b: []byte("\x00[\x00"),
		},
		{
			b:   []byte("\x00[]\x00"),
			ok:  true,
			out: "[]",
		},
		{
			b:   []byte("[\"up == 0\",\"rate(http_requests[2w]\"]\x00\x00\x00"),
			ok:  true,
			out: "[\"up == 0\",\"rate(http_requests[2w]\"]",
		},
	} {
		t.Run("", func(t *testing.T) {
			out, ok := parseBrokenJSON(tc.b)
			require.Equal(t, tc.ok, ok)
			if ok {
				require.Equal(t, tc.out, out)
			}
		})
	}
}

func TestNewJSONEntryFitsEntrySize(t *testing.T) {
	// Long query with characters that expand under JSON escaping (", \).
	// Without accounting for escaping, the marshaled entry exceeds entrySize.
	query := `(sum(namedprocess_namegroup_cpu_seconds_total{instance=~\"127.0.0.1:9256\", job=\"process-exporter\", groupname=~\"(groupname1|groupname2|groupname3-1|groupname4-1|groupname5|groupname_6|groupname_7|groupname8-session-1|groupname_9|groupname10-notifier|groupname-11-upd|groupname-12-dat|groupname13-manager|groupname14|groupname15|groupname16-|groupname17|groupname18-f|groupname19-extract|groupname20: server|groupname21: client|groupname22-deskto|groupname23|groupname24-ask|groupname25n|groupname26-timedat|groupname27-resolve|groupname28|groupname29|groupname30-journal|groupname31|groupname32|groupname33|groupname34-cont|groupname35|groupname36|groupname37|groupname38|groupname39|groupname40|groupname41\\\\.groupname42|groupname43-i|groupname44|groupname45|groupname46|groupname47|groupname48|groupname49|groupname50|groupname51|groupname52|groupname53|groupname54|groupname55-ng|groupname56|groupname57|groupname58-daemon|groupname59|groupname60|groupname61\\\\.62|groupname63\\\\.groupname64|groupname65\\\\.groupname66|groupname67|groupname68|groupname69|groupname70|pgroupname71|groupname72|groupname73|groupname74-|groupname75|groupname76"}))`
	require.Greater(t, len(query), entrySize)

	jsonEntry := newJSONEntry(query, log.NewNopLogger())
	// Insert reserves the last byte of each slot for a trailing comma.
	require.LessOrEqual(t, len(jsonEntry), entrySize-1, "json entry: %s", jsonEntry)
	require.True(t, json.Valid(jsonEntry))
}
