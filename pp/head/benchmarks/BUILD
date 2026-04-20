package(default_visibility = ["//visibility:public"])

cc_binary(
    name = "chunk_recoder",
    srcs = ["chunk_recoder_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:head",
        "//:benchmark",
    ],
)
