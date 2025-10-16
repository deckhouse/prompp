package(default_visibility = ["//visibility:public"])

cc_binary(
    name = "generate_reverse_index",
    srcs = ["generate_reverse_index_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:performance_tests_headers",
        "@google_benchmark//:benchmark_main",
    ],
)

cc_binary(
    name = "generate_lss",
    srcs = ["generate_queryable_encoding_bimap_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:performance_tests_headers",
        "@google_benchmark//:benchmark_main",
    ],
)
