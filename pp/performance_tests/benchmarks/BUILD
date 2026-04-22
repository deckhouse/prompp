package(default_visibility = ["//visibility:public"])

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
