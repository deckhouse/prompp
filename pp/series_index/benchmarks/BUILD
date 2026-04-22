package(default_visibility = ["//visibility:public"])

filegroup(
    name = "sources",
    srcs = glob(["*_benchmark.cpp"]),
    visibility = ["//visibility:public"],
)

cc_binary(
    name = "querier",
    srcs = ["querier_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:series_index",
        "//:benchmark",
    ],
)
