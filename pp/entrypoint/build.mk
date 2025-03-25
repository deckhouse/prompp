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
		$(call bazel_build_march,$(generic_flavor)) -- //:entrypoint_init_aio
	@cp -f ../bazel-bin/entrypoint_init_aio.a $@

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


# We build all archives in bash loop because files contains escaped flavor in name but --march flag shouldn't be escaped
.PRECIOUS: $(archives)
$(archives):
	@mkdir -p $(build_dir)
	@$(bazel_in_root);\
		for i in $(flavors); do\
			$(call bazel_build_march,$$i) -- //:entrypoint_aio &&\
			cp -f bazel-bin/entrypoint_aio.a $(build_dir_absolute_path)/$(platform)_$$($(call escape,$$i))_entrypoint_aio.a ||\
			exit 1;\
		done
