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

package scrape

import (
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"testing"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/prometheus/util/fuzzing/scrapecorpus"
)

// maxFuzzBodySize caps the seed and mutated payloads. readResponse copies the
// whole body into memory, so without a cap the fuzzer spends its time on
// allocation instead of on the decompression and limit logic.
const maxFuzzBodySize = 1 << 20

// FuzzReadResponse fuzzes the transport half of the scrape loop: reading a
// target's response, optionally through gzip, under a body size limit.
//
// The scrape body itself is opaque here — parsing is fuzzed in
// pp/go/cppbridge — so the target checks the properties readResponse promises
// its caller:
//
//   - the response body is closed exactly once, whatever happens;
//   - a body that is not announced as gzip reaches the buffer byte for byte;
//   - a gzip body round-trips to the bytes the target compressed;
//   - a body that reaches bodySizeLimit fails with errBodySizeLimit, and one
//     that does not is never truncated;
//   - the content type is only reported together with a nil error;
//   - the pooled bufio/gzip readers carry nothing over, so the same response
//     read twice gives the same answer.
func FuzzReadResponse(f *testing.F) {
	seed := func(body []byte) {
		f.Add(body, false, false, uint8(0), int64(0), "text/plain")
		f.Add(body, true, true, uint8(0), int64(0), "text/plain")
		f.Add(body, true, true, uint8(3), int64(0), "text/plain")
		f.Add(body, true, false, uint8(0), int64(0), "text/plain")
		f.Add(body, false, false, uint8(0), int64(len(body)), "text/plain")
		f.Add(body, false, false, uint8(0), int64(len(body)+1), "text/plain")
		f.Add(body, true, true, uint8(0), int64(len(body)), "application/openmetrics-text")
	}
	for _, body := range scrapecorpus.GetCorpusForPrometheusText() {
		seed(body)
	}
	for _, body := range scrapecorpus.GetCorpusForOpenMetricsText() {
		seed(body)
	}
	// A gzip bomb: highly compressible, so the limit must be enforced on the
	// decompressed stream rather than on the wire size.
	seed(bytes.Repeat([]byte("metric_name 1\n"), 4096))

	metrics, err := newScrapeMetrics(prometheus.NewRegistry())
	if err != nil {
		f.Fatalf("cannot create scrape metrics: %s", err)
	}

	f.Fuzz(func(
		t *testing.T,
		body []byte,
		announceGzip bool,
		compress bool,
		truncate uint8,
		limit int64,
		contentType string,
	) {
		if len(body) > maxFuzzBodySize {
			t.Skip("body larger than maxFuzzBodySize")
		}
		if !isValidHeaderValue(contentType) {
			t.Skip("content type is not a valid header value")
		}

		payload, truncated := makePayload(t, body, compress, truncate)

		// A limit of at most one byte past the payload keeps both sides of the
		// limit check reachable; readResponse treats anything <= 0 as no limit.
		limit %= int64(len(payload)) + 2

		got := readResponseOnce(t, metrics, payload, announceGzip, limit, contentType)

		// The pooled readers are the only state shared between calls, so
		// reading the same response again has to give the same answer.
		if again := readResponseOnce(t, metrics, payload, announceGzip, limit, contentType); !got.equal(again) {
			t.Fatalf("reading the same response twice gave different results:\nfirst:  %s\nsecond: %s", got, again)
		}

		effectiveLimit := limit
		if effectiveLimit <= 0 {
			effectiveLimit = math.MaxInt64
		}

		switch {
		case got.err != nil:
			if got.contentType != "" {
				t.Fatalf("content type %q reported together with error %s", got.contentType, got.err)
			}
		case int64(got.body.Len()) >= effectiveLimit:
			t.Fatalf("read %d bytes without hitting the body size limit %d", got.body.Len(), effectiveLimit)
		case got.contentType != contentType:
			t.Fatalf("content type %q does not match the response header %q", got.contentType, contentType)
		}

		// The bytes readResponse has to deliver, for the cases where the
		// payload is unambiguous. A body that is not announced as gzip is
		// passed through, and an intact gzip stream decompresses back to what
		// the target compressed. Anything else — a truncated stream, or raw
		// bytes announced as gzip — may legitimately fail anywhere inside the
		// gzip reader, so only the invariants above apply.
		var want []byte
		switch {
		case !announceGzip:
			want = payload
		case compress && !truncated:
			want = body
		default:
			return
		}

		if int64(len(want)) >= effectiveLimit {
			if !errors.Is(got.err, errBodySizeLimit) {
				t.Fatalf("body of %d bytes with limit %d: got %s, want %s",
					len(want), effectiveLimit, got, errBodySizeLimit)
			}
			return
		}
		if got.err != nil {
			t.Fatalf("reading a %d byte body with limit %d failed: %s", len(want), effectiveLimit, got.err)
		}
		if !bytes.Equal(got.body.Bytes(), want) {
			t.Fatalf("body was rewritten: got %d bytes %q, want %d bytes %q",
				got.body.Len(), got.body.Bytes(), len(want), want)
		}
	})
}

// makePayload returns the bytes the fake target puts on the wire. When compress
// is set the body is gzipped and then cut short by truncate bytes, which is how
// a target that dies mid-response looks to the scraper.
func makePayload(t *testing.T, body []byte, compress bool, truncate uint8) (payload []byte, truncated bool) {
	t.Helper()

	if !compress {
		return body, false
	}

	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(body); err != nil {
		t.Fatalf("cannot gzip the body: %s", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("cannot close the gzip writer: %s", err)
	}

	payload = buf.Bytes()
	cut := int(truncate) % (len(payload) + 1)

	return payload[:len(payload)-cut], cut > 0
}

// readResponseOnce reads a response built from payload with a scraper of its
// own, so that the pooled readers are the only state left between calls.
func readResponseOnce(
	t *testing.T,
	metrics *scrapeMetrics,
	payload []byte,
	announceGzip bool,
	limit int64,
	contentType string,
) readResponseResult {
	t.Helper()

	header := http.Header{}
	header.Set("Content-Type", contentType)
	if announceGzip {
		header.Set("Content-Encoding", "gzip")
	}

	body := &countingBody{reader: bytes.NewReader(payload)}
	resp := &http.Response{
		Status:     "200 OK",
		StatusCode: http.StatusOK,
		Header:     header,
		Body:       body,
	}

	scraper := &targetScraper{bodySizeLimit: limit, metrics: metrics}

	var result readResponseResult
	result.contentType, result.err = scraper.readResponse(context.Background(), resp, &result.body)

	// readResponse owns the body: the scrape loop reuses the connection, which
	// needs the body drained and closed even on the error paths.
	if body.closes != 1 {
		t.Fatalf("response body was closed %d times, want exactly once", body.closes)
	}

	return result
}

// readResponseResult is everything a readResponse call produced.
type readResponseResult struct {
	contentType string
	err         error
	body        bytes.Buffer
}

func (r readResponseResult) equal(other readResponseResult) bool {
	return r.contentType == other.contentType &&
		fmt.Sprint(r.err) == fmt.Sprint(other.err) &&
		bytes.Equal(r.body.Bytes(), other.body.Bytes())
}

func (r readResponseResult) String() string {
	return fmt.Sprintf("content type %q, %d bytes, error %v", r.contentType, r.body.Len(), r.err)
}

// countingBody is an [io.ReadCloser] over a fixed payload that counts how often
// it was closed.
type countingBody struct {
	reader *bytes.Reader
	closes int
}

func (b *countingBody) Read(p []byte) (int, error) {
	return b.reader.Read(p)
}

func (b *countingBody) Close() error {
	b.closes++

	return nil
}

// isValidHeaderValue reports whether v can be stored in an [http.Header]. The
// fuzzer happily produces bytes that net/http would refuse to send, and a
// header the transport cannot represent says nothing about readResponse.
func isValidHeaderValue(v string) bool {
	for i := range len(v) {
		if c := v[i]; c < ' ' && c != '\t' || c == 0x7f {
			return false
		}
	}

	return true
}
