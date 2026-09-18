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

package cppbridge_test

import (
	"bytes"
	"context"
	"math"
	"runtime"
	"strconv"
	"testing"
	"time"

	"github.com/golang/snappy"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/prompb"
)

// maxHeadWalInputSize is the input size above which the fuzzer stops learning
// anything new and only slows itself down. Same rationale (and value) as in
// util/fuzzing and wal_scraper_hashdex_fuzz_test.go:
// https://google.github.io/oss-fuzz/getting-started/new-project-guide/#input-size
const maxHeadWalInputSize = 10240

// headWalSeedShards is the number of shards used to build every seed
// segment. A single shard is enough to exercise the decoder: the shard count
// only changes how AppendRelabelerSeries buckets the incoming series.
const headWalSeedShards = 1

// headWalSeedWriteRequests returns prompb.WriteRequests that, once run
// through buildHeadSegment, cover the corners of the head WAL format: an
// empty segment, one series, many series, many samples on a single series,
// extreme timestamps, non-finite sample values and duplicate label names.
func headWalSeedWriteRequests() []*prompb.WriteRequest {
	now := time.Now().UnixMilli()

	manySeries := &prompb.WriteRequest{}
	for i := 0; i < 64; i++ {
		manySeries.Timeseries = append(manySeries.Timeseries, prompb.TimeSeries{
			Labels: []prompb.Label{
				{Name: "__name__", Value: "many_series"},
				{Name: "shard", Value: strconv.Itoa(i)},
			},
			Samples: []prompb.Sample{{Value: float64(i), Timestamp: now}},
		})
	}

	manySamples := &prompb.WriteRequest{
		Timeseries: []prompb.TimeSeries{{
			Labels: []prompb.Label{{Name: "__name__", Value: "many_samples"}},
		}},
	}
	for i := 0; i < 256; i++ {
		manySamples.Timeseries[0].Samples = append(manySamples.Timeseries[0].Samples, prompb.Sample{
			Value:     float64(i),
			Timestamp: now + int64(i),
		})
	}

	return []*prompb.WriteRequest{
		// Empty: no series at all.
		{},
		// A single series, single sample.
		{
			Timeseries: []prompb.TimeSeries{{
				Labels:  []prompb.Label{{Name: "__name__", Value: "single"}},
				Samples: []prompb.Sample{{Value: 1, Timestamp: now}},
			}},
		},
		manySeries,
		manySamples,
		// Extreme timestamps, kept in ascending order and non-negative so the
		// seed itself is a well-formed segment: the encoder's delta base is
		// the smallest timestamp in the batch, and its own overflow guard
		// (max_int64 - base) is unsound for a negative base, rejecting
		// virtually any negative timestamp long before it reaches the wire
		// format this fuzzer covers. That is an encoder-side limitation
		// orthogonal to decoding, so it is not worked around here; the
		// decoder fuzzer is still free to reach negative decoded timestamps
		// through byte mutation of the seeds below.
		{
			Timeseries: []prompb.TimeSeries{{
				Labels: []prompb.Label{{Name: "__name__", Value: "extreme_ts"}},
				Samples: []prompb.Sample{
					{Value: 1, Timestamp: 0},
					{Value: 2, Timestamp: 1},
					{Value: 3, Timestamp: math.MaxInt64 - 1},
				},
			}},
		},
		// Non-finite sample values.
		{
			Timeseries: []prompb.TimeSeries{{
				Labels: []prompb.Label{{Name: "__name__", Value: "non_finite"}},
				Samples: []prompb.Sample{
					{Value: math.NaN(), Timestamp: now},
					{Value: math.Inf(1), Timestamp: now + 1},
					{Value: math.Inf(-1), Timestamp: now + 2},
				},
			}},
		},
		// Duplicate label names.
		{
			Timeseries: []prompb.TimeSeries{{
				Labels: []prompb.Label{
					{Name: "__name__", Value: "dup_labels"},
					{Name: "dup", Value: "first"},
					{Name: "dup", Value: "second"},
				},
				Samples: []prompb.Sample{{Value: 1, Timestamp: now}},
			}},
		},
	}
}

// buildHeadSegment runs wr through the same pipeline the appender uses to
// turn incoming samples into a head WAL segment: WALSnappyProtobufHashdex ->
// PerGoroutineRelabeler.Relabeling -> PerGoroutineRelabeler.AppendRelabelerSeries
// -> HeadWalEncoder. See pp/go/storage/appender/appender.go and
// pp/go/cppbridge/head_wal_test.go (TestHeadWalEncoder_EncodeAndFinalize) for
// the production and unit-test counterparts of this sequence.
//
// This is seed generation, not the code under test, so callers decide what
// to do with an error (typically: skip that seed).
func buildHeadSegment(wr *prompb.WriteRequest) ([]byte, error) {
	data, err := wr.Marshal()
	if err != nil {
		return nil, err
	}

	hashdex, err := cppbridge.NewWALSnappyProtobufHashdex(snappy.Encode(nil, data), cppbridge.DefaultWALHashdexLimits())
	if err != nil {
		return nil, err
	}

	inputLss := cppbridge.NewLssStorage()
	targetLss := cppbridge.NewQueryableLssStorage()

	statelessRelabeler, err := cppbridge.NewStatelessRelabeler(nil)
	if err != nil {
		return nil, err
	}

	state := cppbridge.NewStateV2WithoutLock()
	options := cppbridge.RelabelerOptions{}
	state.SetRelabelerOptions(&options)
	state.SetStatelessRelabeler(statelessRelabeler)
	state.Reconfigure(0, headWalSeedShards, nil)

	pgr := cppbridge.NewPerGoroutineRelabeler(headWalSeedShards, 0)

	shardedInnerSeries := cppbridge.NewShardedInnerSeries(headWalSeedShards)
	shardedRelabeledSeries := cppbridge.NewShardedRelabeledSeries(headWalSeedShards)
	shardedStateUpdates := cppbridge.NewShardedStateUpdates(headWalSeedShards)

	ctx := context.Background()

	// Stage 1: relabel the incoming hashdex into shardedRelabeledSeries.
	if _, _, err = pgr.Relabeling(
		ctx,
		inputLss,
		targetLss,
		state,
		hashdex,
		shardedInnerSeries.DataByShard(0),
		shardedRelabeledSeries.DataByShard(0),
	); err != nil {
		return nil, err
	}

	// Stage 2: append the relabeled series to targetLss and materialize
	// shardedInnerSeries, exactly what the encoder consumes.
	if _, err = pgr.AppendRelabelerSeries(
		ctx,
		targetLss,
		shardedInnerSeries.DataByShard(0),
		shardedRelabeledSeries.DataByShard(0),
		shardedStateUpdates.DataByShard(0),
	); err != nil {
		return nil, err
	}

	// logShards is 0 for a single encoder, see pp/go/storage/builder.go.
	encoder := cppbridge.NewHeadWalEncoder(0, 0, targetLss)
	if _, err = encoder.Encode(shardedInnerSeries.DataByShard(0)); err != nil {
		return nil, err
	}
	runtime.KeepAlive(shardedInnerSeries)

	segmentData, err := encoder.Finalize()
	if err != nil {
		return nil, err
	}

	var buf bytes.Buffer
	if _, err = segmentData.WriteTo(&buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// addHeadWalSeeds builds a segment for every scenario in
// headWalSeedWriteRequests and feeds it to f.Add. A scenario that cannot be
// turned into a segment (e.g. a WriteRequest the relabeler legitimately
// rejects) is skipped rather than failing corpus setup: fuzzing is still
// useful with a partial seed corpus, and what is "legitimately rejected" here
// is exactly the kind of thing that may change as the relabeler evolves.
func addHeadWalSeeds(f *testing.F) {
	f.Helper()

	for _, wr := range headWalSeedWriteRequests() {
		segment, err := buildHeadSegment(wr)
		if err != nil {
			f.Logf("skipping head WAL seed: %v", err)
			continue
		}
		f.Add(segment)
	}
}

//
// FuzzHeadWalDecoderDecode
//

// headWalDecodeResult is everything a single HeadWalDecoder.Decode call
// produces that we care about for determinism checks.
type headWalDecodeResult struct {
	size uint64
	err  error
}

// FuzzHeadWalDecoderDecode fuzzes HeadWalDecoder.Decode, the entry point used
// by pp/go/cppbridge/head_wal_test.go to turn a raw segment back into
// InnerSeries (e.g. when re-encoding a segment during head migration). Unlike
// the ingestion-path WALDecoder, this one is backed by a caller-owned LSS, so
// a fresh LSS/decoder pair is created per fuzz iteration to isolate any
// crash to the single input that triggered it.
func FuzzHeadWalDecoderDecode(f *testing.F) {
	addHeadWalSeeds(f)

	f.Fuzz(func(t *testing.T, in []byte) {
		if len(in) > maxHeadWalInputSize {
			t.Skip()
		}

		first := decodeHeadWalInnerSeries(t, in)
		second := decodeHeadWalInnerSeries(t, in)

		if (first.err == nil) != (second.err == nil) {
			t.Fatalf("decoding the same input twice gave different errors:\nfirst  %v\nsecond %v", first.err, second.err)
		}
		if first.size != second.size {
			t.Fatalf("decoding the same input twice gave different sizes:\nfirst  %d\nsecond %d", first.size, second.size)
		}
	})
}

// decodeHeadWalInnerSeries decodes in with a fresh decoder and LSS and
// asserts the invariants that hold for any input, well-formed or not: the
// call must not panic, and a successful decode must not report an
// implausibly large series count. The latter guards against the decoder
// looping on, or mis-parsing, a malformed length field: an actual bomb would
// need it to be checked against maxHeadWalInputSize post-decompression
// rather than against len(in), since V3 segments are LZ4-compressed.
func decodeHeadWalInnerSeries(t *testing.T, in []byte) headWalDecodeResult {
	t.Helper()

	lss := cppbridge.NewQueryableLssStorage()
	decoder := cppbridge.NewHeadWalDecoder(lss, cppbridge.EncodersVersion())

	shardedInnerSeries := cppbridge.NewShardedInnerSeries(1)
	innerSeries := shardedInnerSeries.DataByShard(0)

	err := decoder.Decode(in, &innerSeries[0])
	size := innerSeries[0].Size()
	runtime.KeepAlive(shardedInnerSeries)

	const implausibleSeriesCount = 1 << 20
	if err == nil && size > implausibleSeriesCount {
		t.Fatalf("decode reported an implausible series count %d for a %d byte segment", size, len(in))
	}

	return headWalDecodeResult{size: size, err: err}
}

//
// FuzzHeadWalDecoderDecodeToDataStorage
//

// headWalDataStorageResult is everything a single
// HeadWalDecoder.DecodeToDataStorage call produces that we care about for
// determinism checks.
type headWalDataStorageResult struct {
	interval cppbridge.TimeInterval
	err      error
}

// FuzzHeadWalDecoderDecodeToDataStorage fuzzes
// HeadWalDecoder.DecodeToDataStorage, the entry point actually used during
// head storage replay: pp/go/storage/loader.go calls it once per segment
// while loading a shard's WAL file. This is the highest-value target of the
// two, since a crash or corruption here is directly reachable by replaying an
// on-disk WAL file written (or tampered with) by anything.
func FuzzHeadWalDecoderDecodeToDataStorage(f *testing.F) {
	addHeadWalSeeds(f)

	f.Fuzz(func(t *testing.T, in []byte) {
		if len(in) > maxHeadWalInputSize {
			t.Skip()
		}

		first := decodeHeadWalToDataStorage(t, in)
		second := decodeHeadWalToDataStorage(t, in)

		if (first.err == nil) != (second.err == nil) {
			t.Fatalf("decoding the same input twice gave different errors:\nfirst  %v\nsecond %v", first.err, second.err)
		}
		if first.interval != second.interval {
			t.Fatalf("decoding the same input twice gave different time intervals:\nfirst  %+v\nsecond %+v", first.interval, second.interval)
		}
	})
}

// decodeHeadWalToDataStorage decodes in with a fresh decoder and DataStorage,
// mirroring loader.go's real usage. It asserts that the call does not panic
// and that a successful decode never leaves the DataStorage with an inverted
// time interval (MinT > MaxT), which would mean the decoder accepted samples
// it never validated the ordering of.
func decodeHeadWalToDataStorage(t *testing.T, in []byte) headWalDataStorageResult {
	t.Helper()

	dataStorage := cppbridge.NewDataStorage(false, false)
	decoder := cppbridge.NewHeadWalDecoder(cppbridge.NewQueryableLssStorage(), cppbridge.EncodersVersion())

	_, _, err := decoder.DecodeToDataStorage(in, dataStorage)
	interval := dataStorage.TimeInterval(false)

	if err == nil && !interval.IsInvalid() && interval.MinT > interval.MaxT {
		t.Fatalf("decode produced an inverted time interval: %+v", interval)
	}

	return headWalDataStorageResult{interval: interval, err: err}
}

// FuzzHeadWalDecoderReplayDecode and FuzzHeadWalDecoderReplayDecodeToDataStorage
// live in head_wal_replay_fuzz_test.go, gated behind the fuzz build tag:
// `go test -tags=fuzz ...`. They fuzz the same two entry points as above,
// but after first replaying ~9MB of a real captured head WAL through the
// same decoder, exercising the accumulated gorilla/ts_base_/LSS state a
// long-lived decoder actually has instead of the fresh state every test in
// this file starts from. The tag keeps their ~9MB embedded corpus
// (testdata/wal_replay) out of the default test binary.
