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
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/textparse"
	"github.com/prometheus/prometheus/pp/go/cppbridge"
	"github.com/prometheus/prometheus/util/fuzzing/scrapecorpus"
)

// diffSymbolTable is shared across iterations to keep allocations out of the
// hot loop, mirroring util/fuzzing.
var diffSymbolTable = labels.NewSymbolTable()

// FuzzScraperHashdexAgainstTextparse compares the C++ parser that prompp runs in
// production against the Go parser that upstream Prometheus runs, on the same
// scrape body.
//
// Only one direction is asserted: whatever the Go parser accepts, the C++ parser
// must accept too, with the same number of samples and the same metadata. A
// divergence in that direction means prompp silently drops or miscounts data
// that upstream Prometheus would have ingested.
//
// The opposite direction is knowingly not an error — the C++ parser is more
// permissive in places (for example it accepts a label set with a missing comma
// between labels, see the seed corpus) — so those inputs are only skipped.
//
// Bodies of six kinds are skipped rather than compared, because on those the
// answer says nothing about prompp losing data:
//
//   - invalid UTF-8 anywhere in the body, which prompp rejects on purpose;
//   - a NUL byte anywhere, which both take for the end of the input but at
//     different points;
//   - a last line the body cuts short, where both parsers recover as they see
//     fit and prompp additionally hits a known bug;
//   - a label set with neither a comma nor whitespace between two labels, which
//     prompp is known to reject;
//   - a metadata line with something other than whitespace after the metric
//     name, or with a quoted name it never closes, where the two disagree about
//     where the text begins;
//   - a sample value too small for a float64, which prompp is known to reject.
func FuzzScraperHashdexAgainstTextparse(f *testing.F) {
	for _, corpus := range scrapecorpus.GetCorpusForPrometheusText() {
		f.Add(corpus, false)
	}
	for _, corpus := range scrapecorpus.GetCorpusForOpenMetricsText() {
		f.Add(corpus, true)
	}

	f.Fuzz(func(t *testing.T, in []byte, openMetrics bool) {
		if len(in) > maxScrapeInputSize {
			t.Skip()
		}

		if !utf8.Valid(in) {
			// prompp validates UTF-8 over the whole body, including the bytes it
			// does not tokenize, and rejects the scrape (see the v0.8.11 entry in
			// CHANGELOG.md). The Go parser only looks at the parts it reads, so
			// invalid UTF-8 in, say, a comment is a deliberate difference.
			t.Skip()
		}

		goResult := parseWithTextparse(t, in, openMetrics)
		if goResult.err != nil {
			// The Go parser rejected the body; the C++ parser is allowed to be
			// more permissive, so there is nothing to compare.
			t.Skip()
		}
		if hasLabelWithoutSeparator(in) {
			// A known strictness gap, pinned by TestParseLabelWithoutSeparator.
			t.Skip()
		}
		if hasMetadataNameWithoutSpace(in) {
			t.Skip()
		}
		if hasUnderflowingFloat(in) {
			// A known strictness gap, pinned by TestParseUnderflowingValue.
			t.Skip()
		}
		if bytes.IndexByte(in, 0) >= 0 {
			// Both parsers take a NUL byte for the end of the input, but they
			// stop at different points — the Go lexer wherever it meets one, the
			// C++ tokenizer only where a line may start — so from here on the two
			// are reading different bodies. The single-parser targets keep the
			// NUL corpus entries.
			t.Skip()
		}
		if !bytes.HasSuffix(in, []byte("\n")) {
			// Neither parser has meaningful recovery for a last line that the
			// body cuts short, and they disagree in both directions: the C++ one
			// rejects bodies the Go one accepts (a known bug, see
			// TestParseUnterminatedLastLineAtEOF), and where both accept, the Go
			// one can report metadata the C++ one read differently. Comparing
			// them teaches nothing until that EOF handling is reworked.
			t.Skip()
		}

		cppResult := parseCopy(t, newScraperHashdex(openMetrics), in)

		if cppResult.err != nil {
			t.Fatalf("the C++ parser rejected a body the Go parser accepted: %v", cppResult.err)
		}
		if int(cppResult.scraped) != goResult.series {
			t.Fatalf("sample count differs: C++ %d, Go %d", cppResult.scraped, goResult.series)
		}

		cppMetadata := canonicalMetadata(cppResult.metadata, openMetrics)
		if !slices.Equal(cppMetadata, goResult.metadata) {
			t.Fatalf("metadata differs:\nC++ %q\nGo  %q", cppMetadata, goResult.metadata)
		}
	})
}

// hasUnderflowingFloat reports whether the body contains a decimal literal that
// is too small for a float64 and therefore reads as zero.
//
// Go's strconv rounds such a literal to zero without complaining, so upstream
// ingests the sample, while the C++ parser calls it an invalid value and drops
// the whole scrape. See TestParseUnderflowingValue for the pinned reproducer.
//
// Overflow is not affected: both parsers reject a literal that is too large.
func hasUnderflowingFloat(in []byte) bool {
	for _, literal := range bytes.FieldsFunc(in, func(r rune) bool { return !isFloatRune(r) }) {
		value, err := strconv.ParseFloat(string(literal), 64)
		if err != nil || value != 0 {
			continue
		}
		if bytes.ContainsFunc(literal, func(r rune) bool { return r >= '1' && r <= '9' }) {
			return true
		}
	}

	return false
}

func isFloatRune(r rune) bool {
	return r >= '0' && r <= '9' || r == '.' || r == 'e' || r == 'E' || r == '+' || r == '-'
}

// hasMetadataNameWithoutSpace reports whether a HELP, TYPE or UNIT line puts
// something other than whitespace directly after the metric name.
//
// The two parsers then cut the text differently — the C++ one takes the rest of
// the line, the Go one drops the offending byte — and the exposition format has
// no such line, so neither is more right than the other. Only well-formed
// metadata lines are worth comparing.
func hasMetadataNameWithoutSpace(in []byte) bool {
	for _, line := range bytes.Split(in, []byte("\n")) {
		text, ok := metadataText(line)
		if !ok {
			continue
		}

		if len(text) == 0 {
			continue
		}

		name, ok := metadataNameEnd(text)
		if !ok {
			// A quoted name the line never closes runs on into the next one,
			// where the same disagreement about the text plays out.
			return true
		}
		if name == 0 || name == len(text) {
			continue
		}
		if text[name] != ' ' && text[name] != '\t' {
			return true
		}
	}

	return false
}

// metadataNameEnd returns the offset just past the metric name at the start of
// the metadata text, or 0 when there is no name to speak of. The name is either
// a bare one or, for names outside the legacy character set, a quoted string.
//
// The second return value is false when the line holds an opening quote it never
// closes, so where the name ends cannot be told from this line alone.
func metadataNameEnd(text []byte) (int, bool) {
	if text[0] != '"' {
		end := 0
		for end < len(text) && isMetricNameChar(text[end]) {
			end++
		}

		return end, true
	}

	end := 1
	for end < len(text) && text[end] != '"' {
		if text[end] == '\\' {
			end++
		}
		end++
	}
	if end >= len(text) {
		return 0, false
	}

	return end + 1, true
}

// metadataText returns what follows the metadata keyword on a HELP, TYPE or UNIT
// line, with the leading whitespace removed.
func metadataText(line []byte) ([]byte, bool) {
	rest := bytes.TrimLeft(line, " \t")
	if !bytes.HasPrefix(rest, []byte("#")) {
		return nil, false
	}

	rest = bytes.TrimLeft(rest[1:], " \t")
	for _, keyword := range []string{"HELP", "TYPE", "UNIT"} {
		if !bytes.HasPrefix(rest, []byte(keyword)) {
			continue
		}
		// "# HELPer" is a comment, not a HELP line.
		rest = rest[len(keyword):]
		if len(rest) == 0 || rest[0] != ' ' && rest[0] != '\t' {
			return nil, false
		}

		return bytes.TrimLeft(rest, " \t"), true
	}

	return nil, false
}

func isMetricNameChar(c byte) bool {
	return isLabelNameStart(c) || c == ':' || c >= '0' && c <= '9'
}

// hasLabelWithoutSeparator reports whether a label set holds a label name that
// directly follows the closing quote of the previous label value, with neither a
// comma nor whitespace between them.
//
// The C++ parser rejects that body and the Go parser reads both labels, so
// prompp drops a scrape upstream Prometheus ingests. See
// TestParseLabelWithoutSeparator for the pinned reproducer.
func hasLabelWithoutSeparator(in []byte) bool {
	inLabelSet := false
	for i := 0; i < len(in); i++ {
		switch in[i] {
		case '{':
			inLabelSet = true
		case '}', '\n':
			inLabelSet = false
		case '"':
			if !inLabelSet {
				continue
			}
			// Walk to the closing quote, honouring escapes.
			for i++; i < len(in) && in[i] != '"'; i++ {
				if in[i] == '\\' {
					i++
				}
			}
			if i+1 < len(in) && isLabelNameStart(in[i+1]) {
				return true
			}
		}
	}

	return false
}

func isLabelNameStart(c byte) bool {
	return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}

// newScraperHashdex returns the hashdex for the format under comparison.
func newScraperHashdex(openMetrics bool) scraperHashdex {
	if openMetrics {
		return cppbridge.NewOpenMetricsScraperHashdex()
	}
	return cppbridge.NewPrometheusScraperHashdex()
}

// textparseResult is the Go-side parse, reduced to what can be compared against
// the hashdex: the number of float samples and the metadata entries.
type textparseResult struct {
	series   int
	metadata []string
	err      error
}

// parseWithTextparse drains the upstream Go parser over its own copy of the
// body, since the C++ parser rewrites the buffer it is given.
func parseWithTextparse(t *testing.T, in []byte, openMetrics bool) textparseResult {
	t.Helper()

	contentType := "text/plain"
	if openMetrics {
		contentType = "application/openmetrics-text"
	}

	p, warning := textparse.New(bytes.Clone(in), contentType, false, diffSymbolTable)
	if p == nil || warning != nil {
		// Both content types are valid, so this cannot happen.
		t.Fatalf("textparse.New rejected content type %q: %v", contentType, warning)
	}

	result := textparseResult{}
	for {
		entry, err := p.Next()
		if err != nil {
			if !errors.Is(err, io.EOF) {
				result.err = err
			}
			return result
		}

		switch entry {
		case textparse.EntrySeries:
			// Series() does the actual label parsing, so a body that only ever
			// gets counted would not be fully parsed.
			_, _, _ = p.Series()
			result.series++
		case textparse.EntryHelp:
			name, text := p.Help()
			result.metadata = append(result.metadata, canonicalMetadataEntry("help", name, text))
		case textparse.EntryType:
			name, metricType := p.Type()
			result.metadata = append(result.metadata, canonicalMetadataEntry("type", name, []byte(metricType)))
		case textparse.EntryUnit:
			name, unit := p.Unit()
			result.metadata = append(result.metadata, canonicalMetadataEntry("unit", name, unit))
		}
	}
}

// Unescapers for HELP text, which the Go parser resolves and the hashdex hands
// out raw. The two formats accept different escapes: OpenMetrics uses the label
// value set, Prometheus text a smaller one.
var (
	promHelpUnescaper        = strings.NewReplacer(`\\`, "\\", `\n`, "\n")
	openMetricsHelpUnescaper = strings.NewReplacer(`\"`, "\"", `\\`, "\\", `\n`, "\n")
)

// canonicalMetadata renders hashdex metadata in the same shape as the Go parser
// metadata, so the two can be compared directly.
func canonicalMetadata(metadata []cppbridge.WALScraperHashdexMetadata, openMetrics bool) []string {
	unescaper := promHelpUnescaper
	if openMetrics {
		unescaper = openMetricsHelpUnescaper
	}

	canonical := make([]string, 0, len(metadata))
	for _, md := range metadata {
		// Only HELP text carries escapes; a type or a unit is a bare word.
		kind, text := "help", unescaper.Replace(md.Text)
		switch md.Type {
		case cppbridge.HashdexMetadataType:
			kind, text = "type", md.Text
		case cppbridge.HashdexMetadataUnit:
			kind, text = "unit", md.Text
		}
		canonical = append(canonical, canonicalMetadataEntry(kind, []byte(md.MetricName), []byte(text)))
	}
	return canonical
}

// canonicalMetadataEntry normalises a single metadata entry.
//
// Two known, intentional differences are normalised away. The Go parser maps the
// legacy "untyped" type to "unknown", while the hashdex reports the text as
// written. And the hashdex drops all the whitespace between the metric name and
// the text, while the Go parser only drops the first run of it.
func canonicalMetadataEntry(kind string, name, text []byte) string {
	text = bytes.TrimLeft(text, " \t")
	if kind == "type" && bytes.Equal(text, []byte("untyped")) {
		text = []byte("unknown")
	}

	return fmt.Sprintf("%s:%s=%s", kind, name, text)
}
