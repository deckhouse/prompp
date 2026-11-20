//go:build fuzz

// Copyright The Prometheus Authors
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

// This file is gated behind the fuzz build tag so that the ~9MB real-WAL
// fixtures it embeds (see testdata/wal_replay) are only linked into the test
// binary when someone actually asks for it: `go test -tags=fuzz ...`.
// Without the tag, a plain `go test ./pp/go/cppbridge/...` never reads or
// embeds these files, keeping the default test binary and build time
// unaffected by this fuzz corpus.
package cppbridge_test

import (
	"bytes"
	_ "embed"
	"fmt"
	"runtime"
	"sync"
	"testing"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/pp/go/storage/head/shard/wal"
	walreader "github.com/prometheus/prometheus/pp/go/storage/head/shard/wal/reader"
)

//
// FuzzHeadWalDecoderReplayDecode / FuzzHeadWalDecoderReplayDecodeToDataStorage
//

// maxHeadWalReplayInputSize is the size cap for the fuzzed *last* segment in
// FuzzHeadWalDecoderReplayDecode(ToDataStorage). It is far more permissive
// than maxHeadWalInputSize: real segments captured from production traffic
// run up to a few MB (see headWalReplayPrefixFile below), so a 10KB cap
// would reject realistic mutations of a real "next segment" outright.
const maxHeadWalReplayInputSize = 500 * 1024 * 1024

// headWalReplayPrefixFile is a real HeadWalDecoder capture: the first 104
// segments (including the 2-byte file header) of a production head WAL
// file, copied byte-for-byte including their size/crc32/sample_count varint
// framing. 104 was chosen so the segment right after it (see
// headWalReplaySeedSegment) is small; that no longer matters now that
// maxHeadWalReplayInputSize is generous, but the prefix itself is left as
// is.
//
//go:embed testdata/wal_replay/prefix_104_segments.bin
var headWalReplayPrefixFile []byte

// headWalReplaySeedSegment is the raw body of the segment that immediately
// follows headWalReplayPrefixFile in the source WAL file (11,450 samples,
// 8,464 bytes): a valid "next segment" once the prefix has been replayed.
//
//go:embed testdata/wal_replay/seed_segment_104.bin
var headWalReplaySeedSegment []byte

// headWalReplayPrefixOnce, headWalReplayPrefixSegments and
// headWalReplayPrefixErr cache the one-time parse of headWalReplayPrefixFile
// into its constituent segment payloads: re-parsing ~9MB on every fuzz
// iteration would dwarf the cost of the decode calls it is there to set up
// for.
var (
	headWalReplayPrefixOnce     sync.Once
	headWalReplayPrefixSegments [][]byte
	headWalReplayPrefixErr      error
)

// headWalReplaySegments returns the segment payloads making up
// headWalReplayPrefixFile, in order. Replaying all of them through a single
// HeadWalDecoder, in order, establishes the same accumulated
// gorilla/ts_base_/LSS state a long-lived decoder has after ~9MB of real
// traffic -- state that FuzzHeadWalDecoderDecode and
// FuzzHeadWalDecoderDecodeToDataStorage can never reach, since they always
// start from a brand new decoder.
func headWalReplaySegments(tb testing.TB) [][]byte {
	tb.Helper()

	headWalReplayPrefixOnce.Do(func() {
		fileFormatVersion, _, headerSize, err := walreader.ReadHeader(bytes.NewReader(headWalReplayPrefixFile))
		if err != nil {
			headWalReplayPrefixErr = fmt.Errorf("read wal_replay prefix header: %w", err)
			return
		}
		if fileFormatVersion != wal.FileFormatVersion {
			headWalReplayPrefixErr = fmt.Errorf(
				"wal_replay prefix has file format version %d, only %d is supported",
				fileFormatVersion, wal.FileFormatVersion,
			)
			return
		}

		body := bytes.NewReader(headWalReplayPrefixFile[headerSize:])
		err = wal.NewSegmentWalReader(body, walreader.NewSegment).ForEachSegment(func(segment *walreader.Segment) error {
			// segment.Bytes() is backed by a pooled buffer that gets reused
			// (and mutated) on the next ReadFrom/Reset, so it must be
			// copied before it is retained past this callback.
			payload := make([]byte, segment.Length())
			copy(payload, segment.Bytes())
			headWalReplayPrefixSegments = append(headWalReplayPrefixSegments, payload)
			return nil
		})
		if err != nil {
			headWalReplayPrefixErr = fmt.Errorf("parse wal_replay prefix segments: %w", err)
		}
	})

	if headWalReplayPrefixErr != nil {
		tb.Fatalf("failed to load head WAL replay prefix: %v", headWalReplayPrefixErr)
	}

	return headWalReplayPrefixSegments
}

// replayHeadWalPrefix decodes every segment returned by headWalReplaySegments
// into innerSeries using decoder, in order. Every one of these segments is a
// real, previously-valid capture, so a decode failure here is a regression
// in its own right rather than something a fuzzed input is expected to
// trigger.
func replayHeadWalPrefix(t *testing.T, decoder *cppbridge.HeadWalDecoder, innerSeries *cppbridge.InnerSeries) {
	t.Helper()

	for i, segment := range headWalReplaySegments(t) {
		if err := decoder.Decode(segment, innerSeries); err != nil {
			t.Fatalf("replaying known-good prefix segment %d failed: %v", i, err)
		}
	}
}

// FuzzHeadWalDecoderReplayDecode fuzzes HeadWalDecoder.Decode after first
// replaying headWalReplaySegments (~9MB / 104 segments of a real captured
// head WAL) through the same decoder, so the fuzzed input is decoded against
// the accumulated state a long-lived decoder actually has, rather than the
// fresh state FuzzHeadWalDecoderDecode always starts from. This is a
// distinct crash surface: bugs that only manifest after many segments
// (state that only grows, id/counter overflow, cache eviction, ...) are
// invisible to a fresh-decoder-per-iteration harness.
func FuzzHeadWalDecoderReplayDecode(f *testing.F) {
	f.Add(headWalReplaySeedSegment)
	addHeadWalSeeds(f) // also try the synthetic seeds as "corrupted next segment" cases.

	f.Fuzz(func(t *testing.T, in []byte) {
		if len(in) > maxHeadWalReplayInputSize {
			t.Skip()
		}

		first := decodeHeadWalReplay(t, in)
		second := decodeHeadWalReplay(t, in)

		if (first.err == nil) != (second.err == nil) {
			t.Fatalf("decoding the same input twice gave different errors:\nfirst  %v\nsecond %v", first.err, second.err)
		}
		if first.size != second.size {
			t.Fatalf("decoding the same input twice gave different sizes:\nfirst  %d\nsecond %d", first.size, second.size)
		}
	})
}

// decodeHeadWalReplay replays headWalReplaySegments into a fresh decoder and
// LSS, then decodes in on top of that accumulated state. Asserts the same
// invariants as decodeHeadWalInnerSeries for the fuzzed segment: no panic,
// no implausible series count on a successful decode.
func decodeHeadWalReplay(t *testing.T, in []byte) headWalDecodeResult {
	t.Helper()

	lss := cppbridge.NewQueryableLssStorage()
	decoder := cppbridge.NewHeadWalDecoder(lss, cppbridge.EncodersVersion())

	shardedInnerSeries := cppbridge.NewShardedInnerSeries(1)
	innerSeries := shardedInnerSeries.DataByShard(0)

	replayHeadWalPrefix(t, decoder, &innerSeries[0])

	err := decoder.Decode(in, &innerSeries[0])
	size := innerSeries[0].Size()
	runtime.KeepAlive(shardedInnerSeries)

	const implausibleSeriesCount = 1 << 20
	if err == nil && size > implausibleSeriesCount {
		t.Fatalf("decode reported an implausible series count %d for a %d byte segment", size, len(in))
	}

	return headWalDecodeResult{size: size, err: err}
}

// FuzzHeadWalDecoderReplayDecodeToDataStorage is the DecodeToDataStorage
// counterpart of FuzzHeadWalDecoderReplayDecode: it replays
// headWalReplaySegments through DecodeToDataStorage before fuzzing the next
// segment, mirroring loader.go's real replay loop (a WAL file is decoded
// segment by segment into the same DataStorage/decoder pair) even more
// closely than FuzzHeadWalDecoderReplayDecode does.
func FuzzHeadWalDecoderReplayDecodeToDataStorage(f *testing.F) {
	f.Add(headWalReplaySeedSegment)
	addHeadWalSeeds(f)

	f.Fuzz(func(t *testing.T, in []byte) {
		if len(in) > maxHeadWalReplayInputSize {
			t.Skip()
		}

		first := decodeHeadWalReplayToDataStorage(t, in)
		second := decodeHeadWalReplayToDataStorage(t, in)

		if (first.err == nil) != (second.err == nil) {
			t.Fatalf("decoding the same input twice gave different errors:\nfirst  %v\nsecond %v", first.err, second.err)
		}
		if first.interval != second.interval {
			t.Fatalf("decoding the same input twice gave different time intervals:\nfirst  %+v\nsecond %+v", first.interval, second.interval)
		}
	})
}

// decodeHeadWalReplayToDataStorage replays headWalReplaySegments into a
// fresh decoder/DataStorage pair via DecodeToDataStorage, then decodes in on
// top of that accumulated state. Asserts the same invariant as
// decodeHeadWalToDataStorage for the fuzzed segment: no panic, no inverted
// time interval on a successful decode.
func decodeHeadWalReplayToDataStorage(t *testing.T, in []byte) headWalDataStorageResult {
	t.Helper()

	dataStorage := cppbridge.NewDataStorage(false, false)
	decoder := cppbridge.NewHeadWalDecoder(cppbridge.NewQueryableLssStorage(), cppbridge.EncodersVersion())

	for i, segment := range headWalReplaySegments(t) {
		if _, _, err := decoder.DecodeToDataStorage(segment, dataStorage); err != nil {
			t.Fatalf("replaying known-good prefix segment %d failed: %v", i, err)
		}
	}

	_, _, err := decoder.DecodeToDataStorage(in, dataStorage)
	interval := dataStorage.TimeInterval(false)

	if err == nil && !interval.IsInvalid() && interval.MinT > interval.MaxT {
		t.Fatalf("decode produced an inverted time interval: %+v", interval)
	}

	return headWalDataStorageResult{interval: interval, err: err}
}
