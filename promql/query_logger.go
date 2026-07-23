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
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/edsrzf/mmap-go"
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/mailru/easyjson/jwriter"
)

var (
	emptyFiller = "\x00"
	emptyEntry  = strings.Repeat(emptyFiller, entrySize)
)

type ActiveQueryTracker struct {
	mmappedFile   []byte
	getNextIndex  chan int
	logger        log.Logger
	closer        io.Closer
	maxConcurrent int
}

var _ io.Closer = &ActiveQueryTracker{}

type Entry struct {
	Query     string `json:"query"`
	Timestamp int64  `json:"timestamp_sec"`
}

// MarshalJSON implements json.Marshaler.
func (e *Entry) MarshalJSON() ([]byte, error) {
	w := jwriter.Writer{}
	w.RawString(`{"query":`)
	w.String(e.Query)
	w.RawString(`,"timestamp_sec":`)
	w.Int64(e.Timestamp)
	w.RawByte('}')
	return w.BuildBytes()
}

const (
	entrySize int = 1000
)

func parseBrokenJSON(brokenJSON []byte) (string, bool) {
	queries := strings.ReplaceAll(string(brokenJSON), emptyFiller, "")
	if queries != "" {
		queries = queries[:len(queries)-1] + "]"
	}

	// Conditional because of implementation detail: len() = 1 implies file consisted of a single char: '['.
	if len(queries) <= 1 {
		return "[]", false
	}

	return queries, true
}

type syncWriter interface {
	io.Writer
	Sync() error
}

func allocateQueryLogFile(file syncWriter, filesize int) error {
	zeroes := make([]byte, min(filesize, 32*1024))
	remaining := filesize
	for remaining > 0 {
		writeBytes := zeroes
		if remaining < len(writeBytes) {
			writeBytes = writeBytes[:remaining]
		}
		n, err := file.Write(writeBytes)
		if err != nil {
			return err
		}
		if n != len(writeBytes) {
			return io.ErrShortWrite
		}
		remaining -= n
	}
	return file.Sync()
}

func logUnfinishedQueries(filename string, filesize int, logger log.Logger) {
	if _, err := os.Stat(filename); err == nil {
		fd, err := os.Open(filename)
		if err != nil {
			level.Error(logger).Log("msg", "Failed to open query log file", "err", err)
			return
		}
		defer fd.Close()

		brokenJSON := make([]byte, filesize)
		_, err = fd.Read(brokenJSON)
		if err != nil {
			level.Error(logger).Log("msg", "Failed to read query log file", "err", err)
			return
		}

		queries, queriesExist := parseBrokenJSON(brokenJSON)
		if !queriesExist {
			return
		}
		level.Info(logger).Log("msg", "These queries didn't finish in prometheus' last run:", "queries", queries)
	}
}

type mmappedFile struct {
	f io.Closer
	m mmap.MMap
}

func (f *mmappedFile) Close() error {
	err := f.m.Unmap()
	if err != nil {
		err = fmt.Errorf("mmappedFile: unmapping: %w", err)
	}
	if fErr := f.f.Close(); fErr != nil {
		return errors.Join(fmt.Errorf("close mmappedFile.f: %w", fErr), err)
	}

	return err
}

func getMMappedFile(filename string, filesize int, logger log.Logger) ([]byte, io.Closer, error) {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0o666)
	if err != nil {
		absPath, pathErr := filepath.Abs(filename)
		if pathErr != nil {
			absPath = filename
		}
		level.Error(logger).Log("msg", "Error opening query log file", "file", absPath, "err", err)
		return nil, nil, err
	}

	if err := allocateQueryLogFile(file, filesize); err != nil {
		file.Close()
		level.Error(logger).Log("msg", "Error allocating query log file", "filesize", filesize, "err", err)
		return nil, nil, err
	}

	fileAsBytes, err := mmap.Map(file, mmap.RDWR, 0)
	if err != nil {
		file.Close()
		level.Error(logger).Log("msg", "Failed to mmap", "file", filename, "Attempted size", filesize, "err", err)
		return nil, nil, err
	}

	return fileAsBytes, &mmappedFile{f: file, m: fileAsBytes}, err
}

func NewActiveQueryTracker(localStoragePath string, maxConcurrent int, logger log.Logger) (*ActiveQueryTracker, error) {
	err := os.MkdirAll(localStoragePath, 0o777)
	if err != nil {
		level.Error(logger).Log("msg", "Failed to create directory for logging active queries")
		return nil, fmt.Errorf("create active query log directory: %w", err)
	}

	if maxConcurrent < 1 {
		return nil, fmt.Errorf("maxConcurrent must be greater than 0")
	}

	if maxConcurrent > math.MaxInt32/1000 { // 2GB max size of the file
		return nil, fmt.Errorf("maxConcurrent must be less than %d", math.MaxInt32)
	}

	filename, filesize := filepath.Join(localStoragePath, "queries.active"), 1+maxConcurrent*entrySize
	logUnfinishedQueries(filename, filesize, logger)

	fileAsBytes, closer, err := getMMappedFile(filename, filesize, logger)
	if err != nil {
		return nil, fmt.Errorf("create mmap-ed active query log: %w", err)
	}

	copy(fileAsBytes, "[")
	activeQueryTracker := ActiveQueryTracker{
		mmappedFile:   fileAsBytes,
		closer:        closer,
		getNextIndex:  make(chan int, maxConcurrent),
		logger:        logger,
		maxConcurrent: maxConcurrent,
	}

	activeQueryTracker.generateIndices(maxConcurrent)

	return &activeQueryTracker, nil
}

func trimStringByBytes(str string, size int) string {
	trimIndex := len(str)
	if size < len(str) {
		for size > 0 && !utf8.RuneStart(str[size]) {
			size--
		}
		trimIndex = size
	}

	return str[:trimIndex]
}

func _newJSONEntry(query string, timestamp int64, logger log.Logger) []byte {
	entry := Entry{query, timestamp}
	jsonEntry, err := entry.MarshalJSON()
	if err != nil {
		level.Error(logger).Log("msg", "Cannot create json of query", "query", query)
		return []byte{}
	}

	return jsonEntry
}

func newJSONEntry(query string, logger log.Logger) []byte {
	timestamp := time.Now().Unix()
	// Leave one byte for the trailing ',' written by Insert.
	maxLen := entrySize - 1
	minEntryJSON := _newJSONEntry("", timestamp, logger)

	query = trimStringByBytes(query, maxLen-len(minEntryJSON))
	for {
		jsonEntry := _newJSONEntry(query, timestamp, logger)
		if len(jsonEntry) <= maxLen || query == "" {
			return jsonEntry
		}

		// JSON escaping (e.g. ", \, <, control chars) can make the marshaled
		// entry longer than the raw query byte budget; shrink and retry.
		overflow := len(jsonEntry) - maxLen
		next := max(len(query)-overflow, 0)
		query = trimStringByBytes(query, next)
	}
}

func (tracker ActiveQueryTracker) generateIndices(maxConcurrent int) {
	for i := range maxConcurrent {
		tracker.getNextIndex <- 1 + (i * entrySize)
	}
}

func (tracker ActiveQueryTracker) GetMaxConcurrent() int {
	return tracker.maxConcurrent
}

func (tracker ActiveQueryTracker) Delete(insertIndex int) {
	copy(tracker.mmappedFile[insertIndex:], emptyEntry)
	tracker.getNextIndex <- insertIndex
}

func (tracker ActiveQueryTracker) Insert(ctx context.Context, query string) (int, error) {
	select {
	case i := <-tracker.getNextIndex:
		fileBytes := tracker.mmappedFile
		entry := newJSONEntry(query, tracker.logger)
		start, end := i, i+entrySize

		copy(fileBytes[start:], entry)
		copy(fileBytes[end-1:], ",")
		return i, nil
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// Close closes tracker.
func (tracker *ActiveQueryTracker) Close() error {
	if tracker == nil || tracker.closer == nil {
		return nil
	}
	if err := tracker.closer.Close(); err != nil {
		return fmt.Errorf("close ActiveQueryTracker.closer: %w", err)
	}
	return nil
}
