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
    name = "diagnostics",
    srcs = ["diagnostics/diagnostics.cpp"],
    hdrs = ["diagnostics/diagnostics.h"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [":facts"],
)

cc_test(
    name = "diagnostics_test",
    srcs = ["diagnostics/diagnostics_tests.cpp"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":diagnostics",
        ":facts",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "diagnostic_catalog",
    srcs = ["diagnostics/diagnostic_catalog.cpp"],
    hdrs = ["diagnostics/diagnostic_catalog.h"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [":diagnostics"],
)

cc_test(
    name = "diagnostic_catalog_test",
    srcs = ["diagnostics/diagnostic_catalog_tests.cpp"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":diagnostic_catalog",
        ":diagnostics",
        ":facts",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "entrypoint_contract",
    srcs = ["contract/entrypoint_contract.cpp"],
    hdrs = ["contract/entrypoint_contract.h"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":facts",
    ],
)

cc_test(
    name = "entrypoint_contract_test",
    srcs = ["contract/entrypoint_contract_tests.cpp"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":entrypoint_contract",
        ":facts",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "clang_adapter",
    srcs = [
        "clang_adapter/aggregate_source.cpp",
        "clang_adapter/aggregate_source.h",
        "clang_adapter/clang_runtime.h",
        "clang_adapter/clang_runtime.cpp",
        "clang_adapter/parse.cpp",
    ],
    hdrs = ["clang_adapter/parse.h"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":diagnostics",
        ":entrypoint_contract",
        ":facts",
        "@system_libclang//:libclang",
    ],
)

cc_test(
    name = "clang_adapter_test",
    srcs = ["clang_adapter/parse_tests.cpp"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":clang_adapter",
        ":diagnostics",
        ":facts",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "validate",
    srcs = ["validate/validate.cpp"],
    hdrs = ["validate/validate.h"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":diagnostics",
        ":entrypoint_contract",
        ":facts",
    ],
)

cc_test(
    name = "validate_test",
    srcs = ["validate/validate_tests.cpp"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":diagnostics",
        ":facts",
        ":validate",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "emit_json",
    srcs = ["emit/json.cpp"],
    hdrs = ["emit/json.h"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":diagnostic_catalog",
        ":diagnostics",
        ":facts",
    ],
)

cc_test(
    name = "emit_json_test",
    srcs = ["emit/json_tests.cpp"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":diagnostics",
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
        ":diagnostic_catalog",
        ":diagnostics",
        ":facts",
    ],
)

cc_test(
    name = "emit_diagnostics_test",
    srcs = ["emit/diagnostics_tests.cpp"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":diagnostics",
        ":emit_diagnostics",
        ":facts",
        "@gtest//:gtest_main",
    ],
)

cc_library(
    name = "app_options",
    hdrs = ["app/options.h"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
)

cc_library(
    name = "app_argparse",
    srcs = [
        "app/argparse.cpp",
    ],
    hdrs = [
        "app/argparse.h",
    ],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":app_options",
    ],
)

cc_library(
    name = "app_runtime_debug",
    srcs = ["app/runtime_debug.cpp"],
    hdrs = ["app/runtime_debug.h"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":diagnostics",
    ],
)

cc_library(
    name = "app_run",
    srcs = ["app/run.cpp"],
    hdrs = [
        "app/run.h",
    ],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":app_options",
        ":app_runtime_debug",
        ":clang_adapter",
        ":diagnostics",
        ":emit_diagnostics",
        ":emit_json",
        ":facts",
        ":validate",
    ],
)

cc_test(
    name = "argparse_test",
    srcs = ["app/argparse_tests.cpp"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":app_argparse",
        "@gtest//:gtest_main",
    ],
)

cc_test(
    name = "runtime_debug_test",
    srcs = ["app/runtime_debug_tests.cpp"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":app_runtime_debug",
        ":diagnostics",
        "@gtest//:gtest_main",
    ],
)

cc_test(
    name = "run_test",
    srcs = ["app/run_tests.cpp"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    deps = [
        ":app_run",
        "@gtest//:gtest_main",
    ],
)

cc_binary(
    name = "entrypoint_codegen",
    srcs = ["app/main.cpp"],
    copts = ENTRYPOINT_CODEGEN_COPTS,
    visibility = ["//visibility:public"],
    deps = [
        ":app_argparse",
        ":app_run",
    ],
)
