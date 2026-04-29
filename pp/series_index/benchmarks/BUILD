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

cc_binary(
    name = "generate_reverse_index",
    srcs = ["generate_reverse_index_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:series_index",
        "//:benchmark",
    ],
)

cc_binary(
    name = "copy_lss",
    srcs = ["queryable_encoding_bimap_copy_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:series_index",
        "//:benchmark",
    ],
)
