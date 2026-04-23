## New part of make to replace r-scripts
include scripts/bazel.mk

all_files := $(shell git ls-files -co --exclude-standard)
vendored := $(patsubst ./%,%,$(shell find . -path '*/vendor/*'))
our_files := $(filter-out $(vendored),$(all_files))
cc_files := $(patsubst ./%,%,$(shell find . -type f -regex '.*\.\(h\|hpp\|cpp\)' -a -not -path './third_party/*'))
our_cc_files := $(filter $(cc_files),$(all_files))

.PHONY: help
help: ## Show this message
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-10s\033[0m %s\n", $$1, $$2}'

.PHONY: build-entrypoint
build-entrypoint: ## Build entry point and copy artifacts to Go directory
	$(MAKE) -C entrypoint clean install

.PHONY: cyrillic
cyrillic: $(our_files) ## Check there is no cyrillic symbols in code
	@if LC_ALL=C.utf8 grep -I -H --color='auto' -P -n "\p{Cyrillic}" $^; then \
		exit 1;\
	fi

.PHONY: format
format: $(our_cc_files)
	@clang-format --dry-run  --Werror $^

.PHONY: reformat
reformat: $(our_cc_files)
	@clang-format -i $^

.PHONY: tidy
tidy: ## Check clang-tidy for all libraries code
tidy: tidy_flags := --config=clang-tidy
tidy: tidy_flags += --aspects @bazel_clang_tidy//clang_tidy:clang_tidy.bzl%clang_tidy_aspect
tidy: tidy_flags += --@bazel_clang_tidy//:clang_tidy_config=//:clang_tidy_config
tidy: compilation_mode := fastbuild
tidy: cc_targets := $(shell bazel query 'kind("cc_.*", //...)')
tidy:
	@$(bazel_build) $(tidy_flags) --output_groups=report --platform_suffix=tidy $(cc_targets)

.PHONY: test
test: ## Run all cpp tests
ifdef target
ifdef filter
test: test_args := --test_arg=--gtest_filter=$(filter)
endif
else
test: target := $(shell bazel query 'kind("cc_test.*", //...)')
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
repetitions ?= 10
filter ?= .*
encoded_sequence_values_file ?= performance_tests/test_data/values_for_encoder_test
lss_file ?= performance_tests/test_data/lss
compact_samples_file ?= performance_tests/test_data/compact_samples
compact_samples_ts_offset ?= 1698395400012
prom_scraper_file ?= performance_tests/test_data/extended-monitoring-exporter.metrics
# time_unit: ns | us | ms | s (default: ns)
time_unit ?= ns
bench_all_time_unit := $(if $(filter ns us ms s,$(time_unit)),$(time_unit),ns)
# min: set to 1/true/yes to pipe output through grep _min (default: off)
bench_all_only_min := $(if $(filter 1 y Y on ON true TRUE yes YES,$(min)),| grep _min,)
# list: set to 1/true/yes to print benchmarks list (default: off)
bench_all_print_list := $(if $(filter 1 y Y on ON true TRUE yes YES,$(list)),true,false)
bench_all: ## Run all benchmarks (optional: min=1, time_unit=ns|us|ms|s)
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
		$(bench_all_only_min)
