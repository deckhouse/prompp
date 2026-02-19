package(default_visibility = ["//visibility:public"])

cc_binary(
    name = "generate_reverse_index",
    srcs = ["generate_reverse_index_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:performance_tests_headers",
        "//:benchmark",
    ],
)

cc_binary(
    name = "copy_lss",
    srcs = ["queryable_encoding_bimap_copy_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:performance_tests_headers",
        "//:entrypoint",
        "//:benchmark",
    ],
)

cc_binary(
    name = "resolve_lss",
    srcs = ["queryable_encoding_bimap_resolve_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:performance_tests_headers",
        "//:benchmark",
    ],
)
