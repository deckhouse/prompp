## New part of make to replace r-scripts
include scripts/bazel.mk

all_files = $(shell git ls-files -co --exclude-standard)
vendored = $(patsubst ./%,%,$(shell find . -path '*/vendor/*'))
our_files = $(filter-out $(vendored),$(all_files))
cc_files = $(patsubst ./%,%,$(shell find . -type f -regex '.*\.\(h\|hpp\|cpp\)' -a -not -path './third_party/*'))
our_cc_files = $(filter $(cc_files),$(all_files))

.PHONY: help
help: ## Show this message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-10s\033[0m %s\n", $$1, $$2}'

.PHONY: build-entrypoint
build-entrypoint: ## Build entry point and copy artifacts to Go directory
	$(MAKE) -C entrypoint clean install

.SECONDEXPANSION:

.PHONY: cyrillic
cyrillic: $$(our_files) ## Check there is no cyrillic symbols in code
	@if LC_ALL=C.utf8 grep -I -H --color='auto' -P -n "\p{Cyrillic}" $^; then \
		exit 1;\
	fi

.PHONY: format
format: $$(our_cc_files)
	@clang-format --dry-run  --Werror $^

.PHONY: reformat
reformat: $$(our_cc_files)
	@clang-format -i $^

.PHONY: tidy
tidy: tidy-entrypoint tidy-bridge-header ## Check clang-tidy for all project C++ targets
tidy: tidy_flags := --config=clang-tidy
tidy: tidy_flags += --@aspect_rules_lint//lint:fail_on_violation
tidy: compilation_mode := fastbuild
# Select modes from existing source paths, including benchmark filegroups.
# Keep the mixed legacy performance harness under full analysis.
tidy: tidy_cc_query = kind("cc_.*", //...) except filter("^//(third_party|tools)(/|:)", //...)
tidy: tidy_shallow_query = attr("srcs", "(_tests[.]cpp|/benchmarks[:/]|bare_bones/tests/coredump_test_separate[.]cpp)", $(tidy_cc_query)) except //:performance_tests_bin
tidy: tidy_production_targets = $(shell $(bazel) query '$(tidy_cc_query) except ($(tidy_shallow_query))')
tidy: tidy_shallow_targets = $(shell $(bazel) query '$(tidy_shallow_query)')
tidy:
	@status=0; \
	$(bazel_build) $(tidy_flags) --aspects //bazel/tidy:linters.bzl%clang_tidy --output_groups=rules_lint_human --platform_suffix=tidy $(tidy_production_targets) || status=$$?; \
	$(bazel_build) $(tidy_flags) --aspects //bazel/tidy:linters.bzl%clang_tidy_shallow --output_groups=rules_lint_human --platform_suffix=tidy $(tidy_shallow_targets) || status=$$?; \
	exit $$status

# Generate the runtime source before Bazel discovers its source files.
.PHONY: tidy-entrypoint
tidy-entrypoint:
	@$(MAKE) -C entrypoint init/entrypoint.cpp

# The installed C API header has no C++ translation unit in the Bazel graph.
.PHONY: tidy-bridge-header
tidy-bridge-header:
	@clang-tidy --config-file=.clang-tidy go/cppbridge/entrypoint.h -- -x c++-header -std=c++23

.PHONY: test
test: ## Run all cpp tests
ifdef target
ifdef filter
test: test_args := --test_arg=--gtest_filter=$(filter)
endif
else
test: target = $(shell $(bazel) query 'kind("cc_test.*", //...)')
endif
test:
	@$(bazel_test) $(test_args) --test_output=errors -- $(target)

.PHONY: coverage
coverage: ## Run code coverage
ifndef target
coverage: target := //...
coverage: lcov_directory := ./bazel-testlogs/
else
coverage: lcov_directory := ./bazel-testlogs/$(target)/_coverage/bazel-out/
endif
coverage:
	@$(bazel) coverage $(bazel_flags) --cache_test_results=no  --experimental_fetch_all_coverage_outputs $(target)
	lcov -t "pp" -o pp.info -c -f -d $(lcov_directory) -b ./ --no-external \
		--ignore-errors unused -j `nproc` \
		-exclude "$(bazel_workspace_root)/external/*" -exclude "*_tests.cpp" -exclude "*_mock.h" \
		-exclude "$(bazel_workspace_root)/bazel-out/*" -exclude "$(bazel_workspace_root)/*/tests/*" \
		-exclude "$(bazel_workspace_root)/third_party/*"
	genhtml --hierarchical -q -o report pp.info --demangle-cpp

.PHONY: bench
bench: ## Run benchmark
	@$(call bazel_build_march,native) //$(package)/benchmarks:$(project)
	@cat $(package)/benchmarks/$(project).flags | ./bazel-bin/$(package)/benchmarks/$(project) `xargs`

.PHONY: bench_all
# repetitions: number of runs per benchmark (Google Benchmark --benchmark_repetitions)
repetitions ?= 10
# filter: regex for benchmark names (--benchmark_filter, default matches all)
filter ?= .*
# encoded_sequence_values_file: binary fixture for encoding / encoded-sequence benchmarks
encoded_sequence_values_file ?= performance_tests/test_data/values_for_encoder_test
# lss_file: binary serialized label-set file for benchmarks with lss
lss_file ?= performance_tests/test_data/lss
# compact_samples_file: compact samples file for benchmarks with DataStorage
compact_samples_file ?= performance_tests/test_data/compact_samples
# compact_samples_ts_offset: base timestamp (ms) applied for timestamp when loading compact_samples_file
compact_samples_ts_offset ?= 1698395400012
# prom_scraper_file: Prometheus text scrape file for scraper-related benchmarks
prom_scraper_file ?= performance_tests/test_data/extended-monitoring-exporter.metrics
# stream_v_byte_values_count: how many values to generate for stream_v_byte benchmarks
stream_v_byte_values_count ?= 1000
# time_unit: ns | us | ms | s (default: ns)
time_unit ?= ns
bench_all_time_unit := $(if $(filter ns us ms s,$(time_unit)),$(time_unit),ns)
# min: set to 1/true/yes to pipe output through grep _min (default: off)
bench_all_only_min := $(if $(filter 1 y Y on ON true TRUE yes YES,$(min)),| grep _min,)
# list: set to 1/true/yes to print benchmarks list (default: off)
bench_all_print_list := $(if $(filter 1 y Y on ON true TRUE yes YES,$(list)),true,false)
bench_all: ## Run all benchmarks (optional: repetitions=10, filter=.*, min=1, time_unit=ns|us|ms|s, list=false)
	@$(call bazel_build_march,native) benchmarks_all
	@./bazel-bin/benchmarks_all \
		--benchmark_repetitions=$(repetitions) \
		--benchmark_time_unit=$(bench_all_time_unit) \
		--benchmark_filter=$(filter) \
		--benchmark_list_tests=$(bench_all_print_list) \
		--benchmark_counters_tabular=true \
		--benchmark_context=encoded_sequence_values_file=$(encoded_sequence_values_file) \
		--benchmark_context=lss_file=$(lss_file) \
		--benchmark_context=compact_samples_file=$(compact_samples_file) \
		--benchmark_context=compact_samples_ts_offset=$(compact_samples_ts_offset) \
		--benchmark_context=prom_scraper_file=$(prom_scraper_file) \
		--benchmark_context=stream_v_byte_values_count=$(stream_v_byte_values_count) \
		$(bench_all_only_min)
