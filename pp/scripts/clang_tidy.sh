#!/usr/bin/env bash
set -euo pipefail

# Examine source arguments only, not compiler flags or the response file after --.
header_filter=
for arg in "$@"; do
    case "$arg" in
        --) break ;;
        *_tests.cpp|bare_bones/tests/coredump_test_separate.cpp)
            # The separate coredump executable is a test helper, not a GoogleTest.
            header_filter='(^|/)(bare_bones|entrypoint|head|metrics|primitives|prometheus|series_data|series_index|wal)/(.*/)?(tests/.*|[^/]*(test|mock|fixture)[^/]*[.]h)$'
            ;;
        */benchmarks/*.cpp)
            header_filter='(^|/)(benchmark|profiling)/.*|(^|/)(bare_bones|head|primitives|series_data|series_index|wal)/benchmarks/.*'
            ;;
    esac
done
if [[ -n "$header_filter" ]]; then
    exec clang-tidy "--header-filter=$header_filter" \
        --extra-arg=-Xclang --extra-arg=-analyzer-config \
        --extra-arg=-Xclang --extra-arg=mode=shallow \
        --extra-arg=-Xclang --extra-arg=-analyzer-inline-max-stack-depth \
        --extra-arg=-Xclang --extra-arg=1 "$@"
fi
exec clang-tidy "$@"
