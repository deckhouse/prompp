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
	"fmt"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/util/fuzzing/scrapecorpus"
)

// maxScrapeInputSize is the input size above which the fuzzer stops learning
// anything new and only slows itself down. Same rationale (and value) as in
// util/fuzzing: https://google.github.io/oss-fuzz/getting-started/new-project-guide/#input-size
const maxScrapeInputSize = 10240

// scraperHashdex is the parsing surface shared by the Prometheus and the
// OpenMetrics scraper hashdexes, i.e. exactly what scrapeLoop.appendCpp uses.
type scraperHashdex interface {
	Parse(buffer []byte, defaultTimestamp int64) (uint32, error)
	RangeMetadata(f func(metadata cppbridge.WALScraperHashdexMetadata) bool)
}

// parseResult is everything a single Parse call produces.
type parseResult struct {
	scraped  uint32
	err      error
	metadata []cppbridge.WALScraperHashdexMetadata
}

// FuzzPrometheusScraperHashdexParse fuzzes the C++ parser behind the Prometheus
// text exposition format. This is the parser that actually runs in production:
// scrapeLoop.appendCpp hands the raw scrape body to it, so the reachable input
// is precisely an arbitrary byte string.
//
// Note that Go's coverage-guided mutator is blind here — it only instruments Go
// code, and the parser is a static library built by bazel. The seed corpus and
// the dictionary in util/fuzzing/scrapecorpus are what make this target
// effective; run it with -asan to get memory-safety checking on the C++ side.
func FuzzPrometheusScraperHashdexParse(f *testing.F) {
	for _, corpus := range scrapecorpus.GetCorpusForPrometheusText() {
		f.Add(corpus)
	}
	// The OpenMetrics corpus is valid input for this parser too: a target may
	// return anything regardless of the negotiated content type.
	for _, corpus := range scrapecorpus.GetCorpusForOpenMetricsText() {
		f.Add(corpus)
	}

	f.Fuzz(func(t *testing.T, in []byte) {
		if len(in) > maxScrapeInputSize {
			t.Skip()
		}
		checkParseIsDeterministic(t, in, func() scraperHashdex {
			return cppbridge.NewPrometheusScraperHashdex()
		})
	})
}

// FuzzOpenMetricsScraperHashdexParse fuzzes the C++ parser behind the
// OpenMetrics exposition format, reached from appendCpp when the target
// responds with "application/openmetrics-text".
func FuzzOpenMetricsScraperHashdexParse(f *testing.F) {
	for _, corpus := range scrapecorpus.GetCorpusForOpenMetricsText() {
		f.Add(corpus)
	}
	for _, corpus := range scrapecorpus.GetCorpusForPrometheusText() {
		f.Add(corpus)
	}

	f.Fuzz(func(t *testing.T, in []byte) {
		if len(in) > maxScrapeInputSize {
			t.Skip()
		}
		checkParseIsDeterministic(t, in, func() scraperHashdex {
			return cppbridge.NewOpenMetricsScraperHashdex()
		})
	})
}

// FuzzScraperHashdexReuse fuzzes hashdex reuse across scrapes. A scrape loop
// creates a fresh hashdex per scrape today, but Parse is written to be
// re-entrant (it overwrites the retained buffer) and the C++ object keeps state
// between calls. This target pins that down: parsing second into a hashdex that
// has already seen first must give exactly the same result as parsing second
// into a brand-new hashdex.
func FuzzScraperHashdexReuse(f *testing.F) {
	prometheusCorpus := scrapecorpus.GetCorpusForPrometheusText()
	openMetricsCorpus := scrapecorpus.GetCorpusForOpenMetricsText()

	for i, first := range prometheusCorpus {
		second := prometheusCorpus[(i+1)%len(prometheusCorpus)]
		f.Add(first, second, false)
	}
	for i, first := range openMetricsCorpus {
		second := openMetricsCorpus[(i+1)%len(openMetricsCorpus)]
		f.Add(first, second, true)
	}

	f.Fuzz(func(t *testing.T, first, second []byte, openMetrics bool) {
		if len(first) > maxScrapeInputSize || len(second) > maxScrapeInputSize {
			t.Skip()
		}

		newHashdex := func() scraperHashdex {
			if openMetrics {
				return cppbridge.NewOpenMetricsScraperHashdex()
			}
			return cppbridge.NewPrometheusScraperHashdex()
		}

		reused := newHashdex()
		parseCopy(t, reused, first)
		got := parseCopy(t, reused, second)

		want := parseCopy(t, newHashdex(), second)

		if !sameResult(got, want) {
			t.Fatalf("reuse changed the result for the same input:\nreused %s\nfresh  %s",
				formatResult(got), formatResult(want))
		}
	})
}

// checkParseIsDeterministic parses in twice, each time into a fresh hashdex,
// and requires both parses to agree. Parsing is a pure function of the input
// bytes, so a mismatch means the parser depends on something it should not:
// uninitialised memory, leftover global state, or — as was the case before the
// buffer contract was documented — the fact that Parse rewrites the buffer it
// is handed.
func checkParseIsDeterministic(t *testing.T, in []byte, newHashdex func() scraperHashdex) {
	t.Helper()

	first := parseCopy(t, newHashdex(), in)
	second := parseCopy(t, newHashdex(), in)

	if !sameResult(first, second) {
		t.Fatalf("parsing the same input twice gave different results:\nfirst  %s\nsecond %s",
			formatResult(first), formatResult(second))
	}
}

// scrapeBufferPadding is the number of readable zero bytes kept after the body.
//
// The C++ tokenizer reads a few bytes past the end of the buffer when the body
// ends mid-token, so the parse verdict otherwise depends on whatever happens to
// follow in memory — see TestParseReadsPastBufferEnd for a reproducer. Padding
// pins that down so these targets can look for other bugs instead of
// rediscovering this one on every run.
const scrapeBufferPadding = 8

// parseCopy parses a private copy of in and asserts the invariants that hold
// for any input, well-formed or not.
//
// The copy is not an optimisation: Parse rewrites the buffer it is given
// (label values are unescaped in place by the C++ parser), so a caller that
// parses the same slice twice gets a corrupted second parse.
func parseCopy(t *testing.T, h scraperHashdex, in []byte) parseResult {
	t.Helper()

	buffer := make([]byte, len(in), len(in)+scrapeBufferPadding)
	copy(buffer, in)
	scraped, err := h.Parse(buffer, cppbridge.NullTimestamp)

	// Every sample needs its own line, so the line count bounds the result.
	// A bogus count here means the C++ side walked past the buffer.
	//
	// Note that a non-zero count together with an error is expected: the parser
	// reports the samples it managed to read before the failure, and appendCpp
	// discards the whole scrape in that case.
	if upper := uint32(bytes.Count(in, []byte("\n")) + 1); scraped > upper {
		t.Fatalf("parse reported %d samples for at most %d lines", scraped, upper)
	}

	metadata := collectMetadata(h)
	checkMetadata(t, buffer, metadata)

	// The metadata lives in C++ memory that RangeMetadata frees once it
	// returns, and the parser holds a pointer into the Go buffer. Force a GC
	// and read it again: under -asan a dangling pointer on either side of the
	// boundary shows up here, and a plain mismatch means the second read sees
	// different data than the first.
	runtime.GC()
	if again := collectMetadata(h); !slices.Equal(metadata, again) {
		t.Fatalf("metadata changed on re-read:\nfirst  %#v\nsecond %#v", metadata, again)
	}
	runtime.KeepAlive(buffer)

	return parseResult{scraped: scraped, err: err, metadata: metadata}
}

// checkMetadata asserts that the metadata the C++ parser reports is consistent
// with the buffer it was parsed from. Both metadata strings are cut out of that
// buffer, so they must be found in it — as long as the buffer is the one Parse
// left behind rather than the original input.
func checkMetadata(t *testing.T, buffer []byte, metadata []cppbridge.WALScraperHashdexMetadata) {
	t.Helper()

	for _, md := range metadata {
		if md.Type > cppbridge.HashdexMetadataUnit {
			t.Fatalf("metadata has unknown type %d: %#v", md.Type, md)
		}
		if !bytes.Contains(buffer, []byte(md.MetricName)) {
			t.Fatalf("metadata name %q does not occur in the parsed buffer: %#v", md.MetricName, md)
		}
		if !bytes.Contains(buffer, []byte(md.Text)) {
			t.Fatalf("metadata text %q does not occur in the parsed buffer: %#v", md.Text, md)
		}
	}
}

// collectMetadata drains RangeMetadata into a slice. The strings handed to the
// callback point into C++ memory that is freed as soon as RangeMetadata
// returns, so they are cloned rather than retained.
func collectMetadata(h scraperHashdex) []cppbridge.WALScraperHashdexMetadata {
	var metadata []cppbridge.WALScraperHashdexMetadata
	h.RangeMetadata(func(md cppbridge.WALScraperHashdexMetadata) bool {
		metadata = append(metadata, cppbridge.WALScraperHashdexMetadata{
			MetricName: strings.Clone(md.MetricName),
			Text:       strings.Clone(md.Text),
			Type:       md.Type,
		})
		return true
	})
	return metadata
}

func sameResult(a, b parseResult) bool {
	return a.scraped == b.scraped &&
		sameError(a.err, b.err) &&
		slices.Equal(a.metadata, b.metadata)
}

func sameError(a, b error) bool {
	switch {
	case a == nil && b == nil:
		return true
	case a == nil || b == nil:
		return false
	default:
		return a.Error() == b.Error()
	}
}

func formatResult(r parseResult) string {
	return fmt.Sprintf("scraped=%d err=%v metadata=%#v", r.scraped, r.err, r.metadata)
}
