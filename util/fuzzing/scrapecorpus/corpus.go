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

// Package scrapecorpus holds seed corpora and libFuzzer dictionaries for the
// scrape ingestion path: the exposition-format payloads that a target can
// return.
//
// It is a separate package from util/fuzzing on purpose. The scrape hot path is
// parsed by C++ (see pp/go/cppbridge), so its fuzz targets live next to the cgo
// code and cannot import util/fuzzing without dragging the whole PromQL test
// machinery into a cgo test binary. Keeping the corpus dependency-free (stdlib
// only) lets both sides — and the OSS-Fuzz corpus generator — share it.
package scrapecorpus

// GetCorpusForPrometheusText returns seed payloads in the Prometheus
// text exposition format ("text/plain").
//
// The corpus deliberately mixes well-formed input with the edge cases that the
// parsers are known to disagree on or that historically broke them: optional
// whitespace around braces and equals signs, a trailing comma in the label set,
// a missing comma between labels (accepted by the C++ parser, rejected by the
// Go one), NUL bytes inside HELP text and label values, invalid UTF-8, integer
// overflow in the timestamp, and exposition escapes.
func GetCorpusForPrometheusText() [][]byte {
	return [][]byte{
		[]byte(""),
		[]byte("\n"),
		// Minimal series, with and without a trailing newline.
		[]byte("metric_name 1.0"),
		[]byte("metric_name 1.0\n"),
		// Metadata.
		[]byte("# HELP metric_name help text\n# TYPE metric_name counter\nmetric_name 1.0\n"),
		[]byte("# TYPE metric_name untyped\nmetric_name 1\n"),
		[]byte("# HELP nohelp1\n# HELP nohelp2\n"),
		[]byte("# HELP m escaped \\n newline and \\\\ backslash\nm 1\n"),
		// Metadata keyword glued to the hash, or a quoted name that never closes
		// on the line: the two parsers cut the text differently, so the
		// differential target skips these; the single-parser targets must not
		// panic on them.
		[]byte("#HELP "),
		[]byte("# HELP \"\n\"0\n"),
		[]byte("#1010101010101010\n# TYPE"),
		[]byte("# UNIT metric_name_seconds seconds\nmetric_name_seconds 1\n"),
		// Every metric type keyword, since appendMetadata switches on the text.
		[]byte("# TYPE m counter\n# TYPE m gauge\n# TYPE m histogram\n# TYPE m gaugehistogram\n" +
			"# TYPE m summary\n# TYPE m info\n# TYPE m stateset\n# TYPE m unknown\n# TYPE m untyped\nm 1\n"),
		// Unknown metric type: appendMetadata must turn this into an error, not a panic.
		[]byte("# TYPE m bogus_type\nm 1\n"),
		// Comments, including ones that start with a metadata prefix.
		[]byte("# Hrandom comment starting with prefix of HELP\n#\nwind_speed{A=\"2\",c=\"3\"} 12345\n"),
		[]byte("# comment with escaped \\n newline\n# comment with escaped \\ escape character\n"),
		// Invalid UTF-8 inside a comment: not tokenized, so only whole-body
		// validation sees it.
		[]byte("#\x9c\nmetric 1\n"),
		// Last line without a newline, leaving the tokenizer mid-construct.
		[]byte(" "),
		[]byte("# "),
		[]byte("# HELP A"),
		[]byte("metric 1\n "),
		[]byte("metric 1 "),
		// Whitespace variations around the label set.
		[]byte("go_gc_duration_seconds{quantile=\"0\"} 4.9351e-05\n"),
		[]byte("go_gc_duration_seconds{quantile=\"0.25\",} 7.424100000000001e-05\n"),
		[]byte("go_gc_duration_seconds{ quantile=\"0.9\", a=\"b\"} 8.3835e-05\n"),
		[]byte("go_gc_duration_seconds { quantile= \"1.0\", a= \"b\", } 8.3835e-05\n"),
		[]byte("go_gc_duration_seconds { quantile = \"1.0\", a = \"b\" } 8.3835e-05\n"),
		// Missing comma between labels: the C++ parser accepts this, Go does not.
		[]byte("go_gc_duration_seconds { quantile = \"2.0\" a = \"b\" } 8.3835e-05\n"),
		// Missing comma and no whitespace either: the other way around, Go reads
		// both labels and the C++ parser rejects the body.
		[]byte("go_gc_duration_seconds{quantile=\"2.0\"a=\"b\"} 8.3835e-05\n"),
		// Tabs as separators.
		[]byte("some:aggregate:rate5m{a_b=\"c\"}\t1\n"),
		[]byte("go_goroutines 33  \t123123\n"),
		// Names and labels starting with an underscore.
		[]byte("_metric_starting_with_underscore 1\n"),
		[]byte("testmetric{_label_starting_with_underscore=\"foo\"} 1\n"),
		// Escapes inside a label value.
		[]byte("testmetric{label=\"\\\"bar\\\"\"} 1\n"),
		[]byte("msdos_file_access_time_ms{path=\"C:\\\\DIR\\\\FILE.TXT\",error=\"Cannot find file:\\n\\\"FILE.TXT\\\"\"} 1.234e3\n"),
		// Special float values.
		[]byte("something_weird{problem=\"division by zero\"} +Inf -3982045\n"),
		[]byte("http_request_duration_seconds_bucket{le=\"+Inf\"} 144320\n"),
		[]byte("m NaN\nm -Inf\nm Inf\nm -0\nm 1e-300\nm 1e300\n"),
		// Out of float64 range: underflow reads as zero upstream, overflow is
		// rejected by both parsers.
		[]byte("m 1e-700\nm 1e400\n"),
		// Explicit timestamps, including one that overflows int64.
		[]byte("http_request_count{method=\"post\",code=\"200\"} 1027 1395066363000\n"),
		[]byte("a{b=\"c\"} 1 -9223372036854775808\n"),
		[]byte("a{b=\"c\"} 1 9223372036854775808\n"),
		// No metric name.
		[]byte("{b=\"c\"} 1\n"),
		// Single-quoted label value: unexpected token.
		[]byte("a{b='c'} 1\n"),
		// Non-numeric value.
		[]byte("a{b=\"c\"} v\n"),
		// NUL bytes in HELP text and in a label value.
		[]byte("# HELP metric foo\x00bar\nnull_byte_metric{a=\"abc\x00\"} 1\n"),
		// Invalid UTF-8 in a label value.
		[]byte("a{b=\"\x80\"} 1\n"),
		// UTF-8 quoted names (the metric name as a label).
		[]byte("{\"metric.name\",\"label.name\"=\"value\"} 1\n"),
		// CRLF line endings.
		[]byte("metric_name 1.0\r\nmetric_name 2.0\r\n"),
		// Duplicate label names and an empty label value.
		[]byte("a{b=\"1\",b=\"2\"} 1\n"),
		[]byte("a{b=\"\"} 1\n"),
		// Classic histogram and summary, as a real exporter would expose them.
		[]byte("# HELP http_request_duration_seconds A histogram.\n" +
			"# TYPE http_request_duration_seconds histogram\n" +
			"http_request_duration_seconds_bucket{le=\"0.1\"} 1\n" +
			"http_request_duration_seconds_bucket{le=\"1\"} 2\n" +
			"http_request_duration_seconds_bucket{le=\"+Inf\"} 3\n" +
			"http_request_duration_seconds_sum 1.5\n" +
			"http_request_duration_seconds_count 3\n"),
		[]byte("# TYPE go_gc_duration_seconds summary\n" +
			"go_gc_duration_seconds{quantile=\"0\"} 1\n" +
			"go_gc_duration_seconds_sum 2\n" +
			"go_gc_duration_seconds_count 3\n"),
	}
}

// GetCorpusForOpenMetricsText returns seed payloads in the OpenMetrics text
// exposition format ("application/openmetrics-text").
//
// On top of the plain-text cases this covers the OpenMetrics-only syntax:
// the mandatory "# EOF" terminator, exemplars, created timestamps and
// the _total suffix convention.
func GetCorpusForOpenMetricsText() [][]byte {
	return [][]byte{
		[]byte(""),
		[]byte("# EOF\n"),
		[]byte("# EOF"),
		// Missing terminator.
		[]byte("metric_name_total 1.0\n"),
		[]byte("# TYPE metric_name counter\nmetric_name_total 1.0\n# EOF\n"),
		[]byte("# HELP metric_name help text\n# TYPE metric_name counter\nmetric_name_total 1.0\n# EOF\n"),
		[]byte("# TYPE m info\nm_info{a=\"b\"} 1\n# EOF\n"),
		[]byte("# TYPE m stateset\nm{m=\"a\"} 0\nm{m=\"b\"} 1\n# EOF\n"),
		// Unit metadata, which OpenMetrics requires to match the name suffix.
		[]byte("# TYPE m_seconds counter\n# UNIT m_seconds seconds\nm_seconds_total 1\n# EOF\n"),
		[]byte("# UNIT m bogus\nm 1\n# EOF\n"),
		// Exemplars.
		[]byte("# TYPE m counter\nm_total 1 # {a=\"b\"} 0.5\n# EOF\n"),
		[]byte("# TYPE m counter\nm_total 1 123 # {a=\"b\"} 0.5 456\n# EOF\n"),
		[]byte("# TYPE m histogram\nm_bucket{le=\"1\"} 1 # {trace_id=\"KOO5S4vxi0o\"} 0.67\n# EOF\n"),
		// Malformed exemplar: no label set.
		[]byte("# TYPE m counter\nm_total 1 # 0.5\n# EOF\n"),
		// Created timestamp.
		[]byte("# TYPE m counter\nm_total 1\nm_created 1520430000.123\n# EOF\n"),
		// Content after the terminator.
		[]byte("# EOF\nm 1\n"),
		// Special values and timestamps.
		[]byte("# TYPE m gauge\nm NaN\nm +Inf\nm -Inf\n# EOF\n"),
		[]byte("m 1 1520879607.789\n# EOF\n"),
		// NUL byte and invalid UTF-8.
		[]byte("# HELP m foo\x00bar\nm 1\n# EOF\n"),
		[]byte("m{a=\"\x80\"} 1\n# EOF\n"),
	}
}

// GetDictForFuzzScraper returns libFuzzer dictionary tokens for the exposition
// format parsers. A dictionary matters more here than for the pure-Go targets:
// Go's coverage instrumentation does not reach the C++ parser, so the mutator
// gets no feedback from it and has to rely on meaningful tokens to build
// syntactically interesting input.
func GetDictForFuzzScraper() []string {
	return []string{
		// Metadata and structural keywords.
		"# HELP ",
		"# TYPE ",
		"# UNIT ",
		"# EOF",
		"#",
		// Metric type names accepted by appendMetadata.
		"counter",
		"gauge",
		"histogram",
		"gaugehistogram",
		"summary",
		"info",
		"stateset",
		"unknown",
		"untyped",
		// Label set punctuation.
		"{",
		"}",
		"=\"",
		"\",",
		"\"",
		",",
		" ",
		"\t",
		"\n",
		"\r\n",
		// Suffixes that carry meaning for histograms, summaries and OpenMetrics.
		"_total",
		"_created",
		"_sum",
		"_count",
		"_bucket",
		"le=\"",
		"le=\"+Inf\"",
		"quantile=\"",
		// Value tokens.
		"NaN",
		"+Inf",
		"-Inf",
		"0",
		"1",
		"-1",
		"1e-05",
		"8.3835e-05",
		"9223372036854775807",
		"9223372036854775808",
		"-9223372036854775808",
		"1395066363000",
		// Escapes and bytes that have broken parsers before.
		"\\\\",
		"\\\"",
		"\\n",
		"\x00",
		"\x80",
	}
}
