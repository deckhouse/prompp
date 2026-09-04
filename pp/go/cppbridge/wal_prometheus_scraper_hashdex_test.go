package cppbridge_test

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/stretchr/testify/suite"

	"github.com/prometheus/prometheus/pp/go/cppbridge"
)

type PrometheusScraperHashdexSuite struct {
	suite.Suite
	hasdex *cppbridge.WALPrometheusScraperHashdex
}

func TestPrometheusScraperHashdexSuite(t *testing.T) {
	suite.Run(t, new(PrometheusScraperHashdexSuite))
}

func (s *PrometheusScraperHashdexSuite) SetupTest() {
	s.hasdex = cppbridge.NewPrometheusScraperHashdex()
}

func (s *PrometheusScraperHashdexSuite) TestParseOk() {
	// Arrange
	input := `# HELP go_gc_duration_seconds A summary of the GC invocation durations.
# 	TYPE go_gc_duration_seconds summary
go_gc_duration_seconds{quantile="0"} 4.9351e-05
go_gc_duration_seconds{quantile="0.25",} 7.424100000000001e-05
go_gc_duration_seconds{quantile="0.5",a="b"} 8.3835e-05
go_gc_duration_seconds{quantile="0.8", a="b"} 8.3835e-05
go_gc_duration_seconds{ quantile="0.9", a="b"} 8.3835e-05
# Hrandom comment starting with prefix of HELP
#
wind_speed{A="2",c="3"} 12345
# comment with escaped \n newline
# comment with escaped \ escape character
# HELP nohelp1
# HELP nohelp2
go_gc_duration_seconds{ quantile="1.0", a="b" } 8.3835e-05
go_gc_duration_seconds { quantile="1.0", a="b" } 8.3835e-05
go_gc_duration_seconds { quantile= "1.0", a= "b", } 8.3835e-05
go_gc_duration_seconds { quantile = "1.0", a = "b" } 8.3835e-05
go_gc_duration_seconds { quantile = "2.0" a = "b" } 8.3835e-05
go_gc_duration_seconds_count 99
some:aggregate:rate5m{a_b="c"}	1
# HELP go_goroutines Number of goroutines that currently exist.
# TYPE go_goroutines gauge
go_goroutines 33  	123123
_metric_starting_with_underscore 1
testmetric{_label_starting_with_underscore="foo"} 1
testmetric{label="\"bar\""} 1`
	input += "\n# HELP metric foo\x00bar"
	input += "\nnull_byte_metric{a=\"abc\x00\"} 1\n"

	// Act
	scraped, err := s.hasdex.Parse([]byte(input), -1)
	expectedMetadata := []cppbridge.WALScraperHashdexMetadata{
		{MetricName: "go_gc_duration_seconds", Text: "A summary of the GC invocation durations.", Type: cppbridge.HashdexMetadataHelp},
		{MetricName: "go_gc_duration_seconds", Text: "summary", Type: cppbridge.HashdexMetadataType},
		{MetricName: "nohelp1", Text: "", Type: cppbridge.HashdexMetadataHelp},
		{MetricName: "nohelp2", Text: "", Type: cppbridge.HashdexMetadataHelp},
		{MetricName: "go_goroutines", Text: "Number of goroutines that currently exist.", Type: cppbridge.HashdexMetadataHelp},
		{MetricName: "go_goroutines", Text: "gauge", Type: cppbridge.HashdexMetadataType},
		{MetricName: "metric", Text: "foo\x00bar", Type: cppbridge.HashdexMetadataHelp},
	}
	actualMetadata := make([]cppbridge.WALScraperHashdexMetadata, 0, len(expectedMetadata))
	s.hasdex.RangeMetadata(func(md cppbridge.WALScraperHashdexMetadata) bool {
		actualMetadata = append(actualMetadata, md)
		return true
	})

	// Assert
	s.Require().NoError(err)
	s.Equal(uint32(18), scraped)
	s.Equal(expectedMetadata, actualMetadata)
}

func (s *PrometheusScraperHashdexSuite) TestParseErrScraperParseUnexpectedToken() {
	// Arrange
	input := []byte("a{b='c'} 1\n")

	// Act
	scraped, err := s.hasdex.Parse(input, -1)

	// Assert
	s.Require().ErrorIs(err, cppbridge.ErrScraperParseUnexpectedToken)
	s.Equal(uint32(0), scraped)
}

func (s *PrometheusScraperHashdexSuite) TestParseErrScraperParseNoMetricName() {
	// Arrange
	input := []byte("{b=\"c\"} 1\n")

	// Act
	scraped, err := s.hasdex.Parse(input, -1)

	// Assert
	s.Require().ErrorIs(err, cppbridge.ErrScraperParseNoMetricName)
	s.Equal(uint32(0), scraped)
}

func (s *PrometheusScraperHashdexSuite) TestParseErrScraperInvalidUtf8() {
	// Arrange
	input := []byte("a{b=\"\x80\"} 1\n")

	// Act
	scraped, err := s.hasdex.Parse(input, -1)

	// Assert
	s.Require().ErrorIs(err, cppbridge.ErrScraperInvalidUtf8)
	s.Equal(uint32(0), scraped)
}

func (s *PrometheusScraperHashdexSuite) TestParseErrScraperParseInvalidValue() {
	// Arrange
	input := []byte("a{b=\"c\"} v\n")

	// Act
	scraped, err := s.hasdex.Parse(input, -1)

	// Assert
	s.Require().ErrorIs(err, cppbridge.ErrScraperParseInvalidValue)
	s.Equal(uint32(0), scraped)
}

func (s *PrometheusScraperHashdexSuite) TestParseErrScraperParseInvalidTimestamp() {
	// Arrange
	input := []byte("a{b=\"c\"} 1 9223372036854775808\n")

	// Act
	scraped, err := s.hasdex.Parse(input, -1)

	// Assert
	s.Require().ErrorIs(err, cppbridge.ErrScraperParseInvalidTimestamp)
	s.Equal(uint32(0), scraped)
}

// TestParseRewritesBuffer pins down the buffer contract documented on Parse:
// label values are unescaped in place, so the caller's body is modified and
// cannot be parsed a second time.
func (s *PrometheusScraperHashdexSuite) TestParseRewritesBuffer() {
	// Arrange
	input := []byte("testmetric{label=\"\\\"bar\\\"\"} 1\n")
	original := bytes.Clone(input)

	// Act
	scraped, err := s.hasdex.Parse(input, -1)

	// Assert
	s.Require().NoError(err)
	s.Equal(uint32(1), scraped)
	s.NotEqual(original, input, "Parse is expected to unescape label values in place")

	// Re-parsing the same buffer now sees the rewritten body.
	rescraped, reErr := s.hasdex.Parse(input, -1)
	s.Require().ErrorIs(reErr, cppbridge.ErrScraperParseUnexpectedToken)
	s.Equal(uint32(0), rescraped)
}

// TestParseInvalidUtf8DiscardsPreviousParse covers a hashdex that is reused
// across scrapes: whole-input UTF-8 validation used to bail out before the
// parser reset its state, so the rejected scrape still reported the previous
// scrape's samples and metadata, pointing into a buffer the caller had already
// released.
func (s *PrometheusScraperHashdexSuite) TestParseInvalidUtf8DiscardsPreviousParse() {
	// Arrange
	first := []byte("# HELP metric help text\nmetric 1\n")
	second := []byte("metric{label=\"\xff\"} 1\n")

	// Act
	firstScraped, firstErr := s.hasdex.Parse(first, -1)
	secondScraped, secondErr := s.hasdex.Parse(second, -1)
	var metadata []cppbridge.WALScraperHashdexMetadata
	s.hasdex.RangeMetadata(func(md cppbridge.WALScraperHashdexMetadata) bool {
		metadata = append(metadata, md)
		return true
	})

	// Assert
	s.Require().NoError(firstErr)
	s.Equal(uint32(1), firstScraped)
	s.Require().ErrorIs(secondErr, cppbridge.ErrScraperInvalidUtf8)
	s.Equal(uint32(0), secondScraped)
	s.Empty(metadata)
}

// TestParseReadsPastBufferEnd documents an out-of-bounds read found by
// FuzzPrometheusScraperHashdexParse. When a body ends mid-token the re2c
// tokenizer keeps matching without a bounds check — the generated YYFILL only
// stops when the cursor is exactly at the limit, and matching "# TYPE" needs up
// to five characters of lookahead. The parse verdict therefore depends on the
// bytes that happen to follow the buffer in memory.
//
// Skipped until the tokenizers guarantee a sentinel (or padding) past the end of
// the body; the assertion below is what the fixed behaviour looks like.
func (s *PrometheusScraperHashdexSuite) TestParseReadsPastBufferEnd() {
	s.T().Skip("known bug: the tokenizer reads past the end of the scrape body when it ends mid-token")

	// Arrange
	input := []byte("#1010101010101010\n# TYPE")

	// Act: parse the same body with different bytes following it in memory.
	results := make(map[string]struct{})
	for _, trailing := range []byte{0, ' ', '\n', 'x', 0xff} {
		backing := make([]byte, len(input), len(input)+64)
		copy(backing, input)
		for i := range backing[:cap(backing)][len(input):] {
			backing[:cap(backing)][len(input)+i] = trailing
		}

		_, err := cppbridge.NewPrometheusScraperHashdex().Parse(backing[:len(input):len(input)], -1)
		results[fmt.Sprint(err)] = struct{}{}
	}

	// Assert
	s.Len(results, 1, "the parse result must not depend on the bytes following the body: %v", results)
}

// TestParseUnterminatedLastLineAtEOF documents a divergence from upstream
// Prometheus found by FuzzScraperHashdexAgainstTextparse: for some bodies whose
// last line is not newline-terminated, the parser rejects the whole scrape,
// including the samples it had already read.
//
// The Go parser accepts every body below, and so does the C++ one once the
// missing newline is added. What the affected bodies have in common is that the
// tokenizer is mid-construct when the buffer ends — trailing whitespace it
// cannot attribute yet, or a metadata line whose text has not started — and
// PrometheusParser turns an unfinished token at EOF into an error (see
// validate_parse_result in pp/wal/hashdex/scraper/parser.h). So this is EOF
// handling in the tokenizer, not the grammar.
//
// Skipped until the EOF handling is reworked; the assertions are what the fixed
// behaviour looks like.
func (s *PrometheusScraperHashdexSuite) TestParseUnterminatedLastLineAtEOF() {
	s.T().Skip("known divergence: an unterminated last line can drop the whole scrape")

	for _, tc := range []struct {
		input   string
		scraped uint32
	}{
		{input: " ", scraped: 0},
		{input: "\t", scraped: 0},
		{input: "# ", scraped: 0},
		{input: "# HELP A", scraped: 0},
		{input: "metric 1\n ", scraped: 1},
		{input: "metric 1 ", scraped: 1},
	} {
		s.Run(fmt.Sprintf("%q", tc.input), func() {
			// Arrange
			hashdex := cppbridge.NewPrometheusScraperHashdex()

			// Act
			scraped, err := hashdex.Parse([]byte(tc.input), -1)

			// Assert
			s.Require().NoError(err)
			s.Equal(tc.scraped, scraped)
		})
	}
}

// TestParseLabelWithoutSeparator documents a divergence from upstream Prometheus
// found by FuzzScraperHashdexAgainstTextparse: when a label name directly
// follows the closing quote of the previous label value, the parser rejects the
// body and the scrape is dropped. Upstream reads both labels.
//
// The same body with a space instead of the missing comma parses, so the parser
// only accepts a label name after whitespace or a comma while its tokenizer is
// happy to return one right after a value.
//
// Skipped until the parser accepts it; the assertions are what the fixed
// behaviour looks like.
func (s *PrometheusScraperHashdexSuite) TestParseLabelWithoutSeparator() {
	s.T().Skip("known divergence: a label name right after a label value drops the whole scrape")

	// Arrange
	input := []byte("metric{a=\"1\"b=\"2\"} 0\n")

	// Act
	scraped, err := s.hasdex.Parse(input, -1)

	// Assert
	s.Require().NoError(err)
	s.Equal(uint32(1), scraped)
}

// TestParseUnderflowingValue documents a divergence from upstream Prometheus
// found by FuzzScraperHashdexAgainstTextparse: a sample value too small for a
// float64 is rejected as an invalid value, and the scrape is dropped with it.
// Upstream rounds the value to zero and ingests the sample, which is also what
// strconv.ParseFloat does — underflow is not an error there, unlike overflow,
// which both parsers reject.
//
// Skipped until the value parser rounds instead of failing; the assertions are
// what the fixed behaviour looks like.
func (s *PrometheusScraperHashdexSuite) TestParseUnderflowingValue() {
	s.T().Skip("known divergence: a value that underflows float64 drops the whole scrape")

	// Arrange
	input := []byte("metric 1e-700\n")

	// Act
	scraped, err := s.hasdex.Parse(input, -1)

	// Assert
	s.Require().NoError(err)
	s.Equal(uint32(1), scraped)
}

func (s *PrometheusScraperHashdexSuite) TestParseEmptyInput() {
	// Arrange
	input := []byte{}

	// Act
	scraped, err := s.hasdex.Parse(input, -1)
	var actualMetadata []cppbridge.WALScraperHashdexMetadata
	s.hasdex.RangeMetadata(func(md cppbridge.WALScraperHashdexMetadata) bool {
		actualMetadata = append(actualMetadata, md)
		return true
	})

	// Assert
	s.Require().NoError(err)
	s.Equal([]cppbridge.WALScraperHashdexMetadata(nil), actualMetadata)
	s.Equal(uint32(0), scraped)
}
