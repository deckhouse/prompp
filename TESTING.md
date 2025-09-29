# Running Tests

## Environment

Since the project's build relies on specific versions of the compiler and libraries, it is convenient to use a pre-configured container based on `Dockerfile.ci`. You can build it locally with the following command:
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
