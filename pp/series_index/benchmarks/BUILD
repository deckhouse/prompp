package(default_visibility = ["//visibility:public"])

cc_binary(
    name = "querier",
    srcs = ["querier_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:series_index",
        "@google_benchmark//:benchmark_main",
        "//:profiling",
    ],
)