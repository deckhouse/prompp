# This line should be placed before any include
build_dir_absolute_path := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))$(build_dir)

include ../scripts/bazel.mk

# To distinct results built with different options we use this result_suffix
result_suffix := $(compilation_mode)
ifeq ($(asan),true)
result_suffix := $(result_suffix)_asan
endif

archives := $(patsubst %, $(build_dir)/$(platform)_%_entrypoint_aio.a, $(escaped_flavors))
prefixed_archives := $(patsubst $(build_dir)/%.a, $(result_dir)/%_prefixed_$(result_suffix).a, $(archives))

$(result_dir)/$(platform)_entrypoint_init_aio_$(result_suffix).a: init/entrypoint.cpp
	@mkdir -p ${@D}
	@$(bazel_in_root);\
		$(bazel_build) -- //:entrypoint_init_aio_$(call make_escape,$(generic_flavor))
	@cp -f ../bazel-bin/entrypoint_init_aio_$(call make_escape,$(generic_flavor)).a $@

# Build flavoured prefixed_archives with prefixed symbols
.PRECIOUS: $(prefixed_archives)
$(prefixed_archives): $(result_dir)/%_entrypoint_aio_prefixed_$(result_suffix).a: $(build_dir)/%.pairs | $(build_dir)/%_entrypoint_aio.a
	@mkdir -p ${@D}
	@objcopy --redefine-syms=$< $| $@

.INTERMEDIATE: $(build_dir)/%.pairs
$(build_dir)/%.pairs: $(build_dir)/%.symbols
	@cat $< | cut -d' ' -f3 | sort -u | awk '{ print $$0 " $(call make_escape,$*)_" $$0 }' > $@

.INTERMEDIATE: $(build_dir)/%.symbols
$(build_dir)/%.symbols: $(build_dir)/%_entrypoint_aio.a
	@nm --defined-only $< > $@


# All flavors are built in one bazel invocation (see bazel/rules/march_variant.bzl), so their
# compile actions run in parallel instead of one flavor after another.
.PRECIOUS: $(archives)
$(archives):
	@mkdir -p $(build_dir)
	@$(bazel_in_root);\
		$(bazel_build) -- $(patsubst %,//:entrypoint_aio_%,$(escaped_flavors)) &&\
		for i in $(escaped_flavors); do\
			cp -f bazel-bin/entrypoint_aio_$$i.a $(build_dir_absolute_path)/$(platform)_$${i}_entrypoint_aio.a || exit 1;\
		done
