#!/usr/bin/env bash
# List packages for the "all packages" CI test run.
#
# Excludes packages that are either:
# - not used by cmd/prometheus, or
# - replaced by their pp-pkg counterparts (we test the pp-pkg variants instead).
set -euo pipefail

EXCLUDE_REGEX='prometheus/rules$|prometheus/cmd/promtool$|prometheus/documentation/examples/custom-sd|prometheus/pp/go/server$|prometheus/prompb/rwcommon$|prometheus/promql/promqltest$|prometheus/util/fmtutil$|prometheus/util/junitxml$|prometheus/scrape$|prometheus/storage/remote|prometheus/web/ui/node_modules/'

go list ./... 2>/dev/null | grep -v -E "$EXCLUDE_REGEX"
