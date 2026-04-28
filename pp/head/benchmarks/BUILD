package(default_visibility = ["//visibility:public"])

filegroup(
    name = "sources",
    srcs = glob(["*_benchmark.cpp"]),
    visibility = ["//visibility:public"],
)

cc_binary(
    name = "chunk_recoder",
    srcs = ["chunk_recoder_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:head",
        "//:benchmark",
    ],
)
