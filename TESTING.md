# Running Tests

## Environment

Since the project's build relies on specific versions of the compiler and libraries, it is convenient to use a pre-configured container based on `Dockerfile.ci` (Debian Trixie, GCC 14, LLVM 21 toolchain). You can build it locally with the following command:
```bash
docker build -t prompp-build -f Dockerfile.ci --build-arg BAZEL_ARCH=arm64 --build-arg GO_ARCH=arm64 .
```

Choose the architecture according to the machine you plan to run tests on.

To run and mount the source library, use the following command:
```bash
docker run -it -v .:/src -w /src prompp-build /bin/bash
```

## Testing C++

### Unit Tests

All C++ code, along with tests, is located in the `pp` directory. Within this directory, there is a `Makefile` with the `test` target. This target will compile and run all unit tests for the C++ code.

To running test in only one package use command
```sh
make test target=//:bare_bones_test
```

It is possible also add gtest filter to run only specific tests
```sh
make test target=//:bare_bones_test filter=BareBonesVectorAllocatedMemoryFixture.ObjectWithoutAllocatedMemoryMethod
```

### Benchmarks

TODO

### Performance Tests

TODO

## Testing Go

Since many tests require integration with C++ code, artifacts must be built beforehand. To do this, run the following command in the `pp` directory:
```bash
make build-entrypoint
```

### Unit Tests

Currently, tests cover code management and interaction with C++ within the `pp/go` directory. To run these tests, navigate to the specified directory and execute:
```bash
go test ./...
```

### Fuzzing Tests

Currently, some HTTP interface service endpoints are covered by fuzzing tests. These tests are located in the `web/web_fuzzy_test.go` file. Running these tests can consume a large amount of memory, which might cause failures due to insufficient available resources. To avoid this, you should explicitly limit resource consumption through environment variables:
```bash
GOGC=10 GOMEMLIMIT=50GiB go test --run Web --fuzz Web --fuzztime 1h .
```

#### Scrape Path

The scrape path — everything between a target's HTTP response and the relabeler — has its own set of targets. The exposition-format parser it feeds is written in C++, so those targets live next to the cgo bindings rather than in `util/fuzzing`.

| Target | Package | What it covers |
| --- | --- | --- |
| `FuzzPrometheusScraperHashdexParse` | `pp/go/cppbridge` | The C++ Prometheus text parser: invariants of the samples and metadata a parse produces. |
| `FuzzOpenMetricsScraperHashdexParse` | `pp/go/cppbridge` | The same for the OpenMetrics parser. |
| `FuzzScraperHashdexReuse` | `pp/go/cppbridge` | Hashdex reuse: a scraper parsing several bodies must match a fresh one. |
| `FuzzScraperHashdexAgainstTextparse` | `pp/go/cppbridge` | Differential: whatever upstream's Go parser accepts, the C++ parser must accept identically. |
| `FuzzReadResponse` | `pp-pkg/scrape` | Reading a target's response: gzip, `body_size_limit`, and the pooled readers. |
| `FuzzTargetsFromGroup` | `pp-pkg/scrape` | Target construction: discovered labels and relabel rules in, scrapeable targets out. |

The seed corpora and the libFuzzer dictionary for the exposition formats are shared through `util/fuzzing/scrapecorpus`, which deliberately depends on nothing but the standard library so that both the cgo targets and an external corpus generator can use it.

Every target also runs as an ordinary test over its seed corpus, so `make test` and CI already exercise the seeds. To actually fuzz, run one target at a time from the container described in [Environment](#environment) — the targets link against the C++ bindings, so build them first:
```bash
docker run --rm -it -v "$PWD":/workspace -w /workspace \
  -e CGO_ENABLED=1 -e CGO_CFLAGS="-Wno-error -I/usr/include" -e CGO_LDFLAGS="-L/usr/lib/" \
  prompp-build bash -c '
    git config --global --add safe.directory /workspace
    cd pp && make build-entrypoint && cd ..
    go test -race -run=FuzzReadResponse -fuzz="^FuzzReadResponse$" -fuzztime=10m ./pp-pkg/scrape/
  '
```

Go's coverage instrumentation does not reach the C++ parser, so the mutator gets no feedback from it: the corpus and the dictionary in `scrapecorpus` do most of the work for the hashdex targets, and it is worth extending them when adding syntax.

A failing input is written to `testdata/fuzz/<target>/` inside the package. Since the container writes it as root, take ownership before committing it as a regression seed:
```bash
docker run --rm -v "$PWD":/workspace busybox chown -R "$(id -u):$(id -g)" /workspace
```

Four known parser bugs are pinned by skipped tests in `pp/go/cppbridge/wal_prometheus_scraper_hashdex_test.go` instead of being rediscovered on every run, and each one has a matching workaround in the harness that should be removed together with its skip:

- `TestParseReadsPastBufferEnd` — the tokenizer reads a few bytes past the end of a body that ends mid-token, so the verdict depends on adjacent memory. `parseCopy` pads the buffer with zeroes to make the targets deterministic.
- `TestParseUnterminatedLastLineAtEOF` — a body whose last line is not newline-terminated can be rejected outright, samples included. The differential target skips such bodies, since upstream's own recovery there is just as arbitrary.
- `TestParseLabelWithoutSeparator` — a label name directly after the closing quote of the previous label value is rejected, while upstream reads both labels. The differential target skips those label sets.
- `TestParseUnderflowingValue` — a sample value too small for a float64 is rejected, while upstream rounds it to zero and ingests the sample. The differential target skips bodies holding such a literal.
