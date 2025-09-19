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
tidy: ## Check clang-tidy for all librarie code
tidy: tidy_flags := --aspects @bazel_clang_tidy//clang_tidy:clang_tidy.bzl%clang_tidy_aspect
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

# wal, scraper

# ARCH="native" \
# PACKAGE="bare_bones" && \
# PROJECT="stream_v_byte_sequence" && \
# bazel build -c opt --march=$ARCH //$PACKAGE/benchmarks:$PROJECT && \
# ./bazel-bin/$PACKAGE/benchmarks/$PROJECT \
#   --benchmark_repetitions=1 \
#   --benchmark_time_unit=us \
#   --benchmark_context=values=5000

