load(":build_defs.bzl", "entrypoint_codegen_copts")

package(default_visibility = ["//visibility:private"])

ENTRYPOINT_CODEGEN_COPTS = entrypoint_codegen_copts()

cc_library(
    name = "facts",
    srcs = [
        "facts/entrypoint_facts.cpp",
        "facts/string_table.cpp",
    ],
    hdrs = [
        "facts/entrypoint_facts.h",
        "facts/facts.h",
        "facts/string_table.h",
        "facts/tagged_index.h",
    ],
    copts = ENTRYPOINT_CODEGEN_COPTS,
)

cc_test(
    name = "entrypoint_facts_test",
    srcs = ["facts/entrypoint_facts_tests.cpp"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":facts",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "clang_adapter",
    srcs = [
        "clang_adapter/cursor_reader.hpp",
        "clang_adapter/function_extractor.hpp",
        "clang_adapter/parse.cpp",
        "clang_adapter/session.hpp",
    ],
    hdrs = [
        "clang_adapter/parse.h",
    ],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":facts",
        "@system_libclang//:libclang",
    ],
)

cc_library(
    name = "validate",
    srcs = ["validate/validate.cpp"],
    hdrs = ["validate/validate.h"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [":facts"],
)

cc_test(
    name = "validate_test",
    srcs = ["validate/validate_tests.cpp"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":facts",
        ":validate",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "diagnostic_text",
    srcs = ["emit/diagnostic_text.cpp"],
    hdrs = ["emit/diagnostic_text.h"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":facts",
    ],
)

cc_test(
    name = "diagnostic_text_test",
    srcs = ["emit/diagnostic_text_tests.cpp"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":diagnostic_text",
        ":facts",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "emit_json",
    srcs = ["emit/json.cpp"],
    hdrs = ["emit/json.h"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":diagnostic_text",
        ":facts",
    ],
)

cc_test(
    name = "emit_json_test",
    srcs = ["emit/json_tests.cpp"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":emit_json",
        ":facts",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "emit_diagnostics",
    srcs = ["emit/diagnostics.cpp"],
    hdrs = ["emit/diagnostics.h"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":diagnostic_text",
        ":facts",
    ],
)

cc_test(
    name = "emit_diagnostics_test",
    srcs = ["emit/diagnostics_tests.cpp"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":emit_diagnostics",
        ":facts",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "app",
    srcs = [
        "app/argparse.cpp",
        "app/run.cpp",
        "app/runtime_debug.cpp",
    ],
    hdrs = [
        "app/argparse.h",
        "app/run.h",
        "app/runtime_debug.h",
    ],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":clang_adapter",
        ":emit_diagnostics",
        ":emit_json",
        ":facts",
        ":validate",
    ],
)

cc_binary(
    name = "entrypoint_codegen",
    srcs = ["app/main.cpp"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    visibility = ["//visibility:public"],
    deps = [":app"],
)
