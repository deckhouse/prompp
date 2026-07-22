# This line should be placed before any include
build_dir_absolute_path := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))$(build_dir)

include ../scripts/bazel.mk

# To distinct results built with different options we use this result_suffix
result_suffix := $(compilation_mode)
ifeq ($(asan),true)
result_suffix := $(result_suffix)_asan
endif

# Single non-flavored archive.
#
# We build one variant with the baseline march for the platform ($(generic_flavor))
# and link its symbols directly, without per-flavor prefixing or runtime dispatch.
$(result_dir)/$(platform)_entrypoint_aio_$(result_suffix).a:
	@mkdir -p ${@D}
	@$(bazel_in_root);\
		$(call bazel_build_march,$(generic_flavor)) -- //:entrypoint_aio
	@cp -f ../bazel-bin/entrypoint_aio.a $@
