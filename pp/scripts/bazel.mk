# This is includ file with

compilation_mode?=opt## Bazel compilation mode opt or dbg
asan?=false## Build with address sanitizer
use_docker?=true## Use bazel in docker
bazel_verbose?=false## More verbose output
profiling?=false## Build with Tracy instrumentation zones enabled

# Bazel workspace directory relative to this file
bazel_workspace_root := $(abspath $(dir $(abspath $(lastword $(MAKEFILE_LIST))))..)

bazel := bazel

# On host system bazel run inside docker
#
# For general purpose using alpine (musl runtime bindings) as production build.
# But musl doesn't support sanitizers and we use based on ubuntu (glibc) environment.

# Every --platform_suffix set here or in bazel_build_march overrides the one
# --config=profile brings, so each has to carry the profiling state itself.
platform_suffix_prefix :=
ifeq ($(profiling),true)
	bazel_flags += --config=profile
	platform_suffix_prefix := prof_
endif

bazel_flags += --compilation_mode=$(compilation_mode)
ifeq ($(asan),true)
	bazel_flags += --asan --strip=never --platform_suffix=$(platform_suffix_prefix)asan
endif
ifeq ($(bazel_verbose),true)
	bazel_flags += --subcommands --verbose_failures --sandbox_debug
endif


# bazel_in_root is a shortcut to run command in project root
bazel_in_root := cd $(bazel_workspace_root)

# bazel_build exposed to bazel build command prefix
#
# Use this string in manner
# ```
# $(bazel_in_root);\
# 	$(bazel_build) -- //:my-target
# ```
#
# In cases if you need to specify concrete march option (default is native),
# use bazel_build_march
bazel_build = $(bazel) build $(bazel_flags)

# bazel_test exposed to bazel test command prefix
#
# Use this string in manner
# ```
# $(bazel_in_root);\
# 	$(bazel_test) -- //:my-target
# ```
bazel_test = $(bazel) test $(bazel_flags)

# bazel_build_march is a function to add --march flag to standard bazel_build
#
# Use this function in manner
# ```
# $(bazel_in_root);\
# 	$(call bazel_build_march,nehalem) -- //:my_target
# ```
bazel_build_march = $(bazel_build) --march=$(1) --platform_suffix=$(platform_suffix_prefix)$$(echo $(1) | sed 's/[^a-zA-Z0-9]/_/g')
