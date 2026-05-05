load("@bazel_skylib//rules:common_settings.bzl", "bool_flag", "string_flag")
load(":cc_toolchain_config.bzl", "cc_toolchain_config")

package(default_visibility = ["//visibility:public"])

config_setting(
    name = "arm64",
    values = {"cpu": "aarch64"},
)

config_setting(
    name = "amd64",
    values = {"cpu": "x86"},
)

bool_flag(
    name = "clang_tidy",
    build_setting_default = 0,
)

bool_flag(
    name = "asan",
    build_setting_default = 0,
)

config_setting(
    name = "with_asan",
    flag_values = {"asan": "1"},
)

bool_flag(
    name = "profiling",
    build_setting_default = 0,
)

string_flag(
    name = "profiling_opts",
    build_setting_default = "default",
)

config_setting(
    name = "with_profiling",
    flag_values = {"profiling": "1"},
)

string_flag(
    name = "march",
    build_setting_default = "native",
)

cc_toolchain_suite(
    name = "g++-14",
    toolchains = {
        "k8": ":g++-14_toolchain_x86_64",
        "aarch64": ":g++-14_toolchain_aarch64",
    },
)

filegroup(name = "empty")

cc_toolchain(
    name = "g++-14_toolchain_x86_64",
    all_files = ":empty",
    compiler_files = ":empty",
    dwp_files = ":empty",
    linker_files = ":empty",
    objcopy_files = ":empty",
    strip_files = ":empty",
    supports_param_files = 0,
    toolchain_config = ":g++-14_toolchain_config_x86_64",
    toolchain_identifier = "g++-14_toolchain",
)

cc_toolchain(
    name = "g++-14_toolchain_aarch64",
    all_files = ":empty",
    compiler_files = ":empty",
    dwp_files = ":empty",
    linker_files = ":empty",
    objcopy_files = ":empty",
    strip_files = ":empty",
    supports_param_files = 0,
    toolchain_config = ":g++-14_toolchain_config_aarch64",
    toolchain_identifier = "g++-14_toolchain",
)

toolchain(
    name = "cc_toolchain_for_g++-14_toolchain_x86_64",
    toolchain = ":g++-14_toolchain_x86_64",
    toolchain_type = "@bazel_tools//tools/cpp:toolchain_type",
    exec_compatible_with = [
        "@platforms//cpu:x86_64",
        "@platforms//os:linux",
    ],
    target_compatible_with = [
        "@platforms//cpu:x86_64",
        "@platforms//os:linux",
    ],
)

toolchain(
    name = "cc_toolchain_for_g++-14_toolchain_aarch64",
    toolchain = ":g++-14_toolchain_aarch64",
    toolchain_type = "@bazel_tools//tools/cpp:toolchain_type",
    exec_compatible_with = [
        "@platforms//cpu:aarch64",
        "@platforms//os:linux",
    ],
    target_compatible_with = [
        "@platforms//cpu:aarch64",
        "@platforms//os:linux",
    ],
)

cc_toolchain_config(
    name = "g++-14_toolchain_config_x86_64",
    builtin_include_directories = [
        "/usr/lib/gcc/x86_64-linux-gnu/14",
        "/usr/include",
        "/usr/lib/linux",
    ],
    march = ":march",
    clang_tidy = ":clang_tidy",
    with_asan = ":asan",
    with_profiling = ":profiling",
    profiling_opts = ":profiling_opts",
)

cc_toolchain_config(
    name = "g++-14_toolchain_config_aarch64",
    builtin_include_directories = [
        "/usr/lib/gcc/aarch64-linux-gnu/14",
        "/usr/include",
        "/usr/lib/linux",
    ],
    march = ":march",
    clang_tidy = ":clang_tidy",
    with_asan = ":asan",
    with_profiling = ":profiling",
    profiling_opts = ":profiling_opts",
)
