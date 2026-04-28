package(default_visibility = ["//visibility:public"])

filegroup(
    name = "sources",
    srcs = glob(["*_benchmark.cpp"]),
    visibility = ["//visibility:public"],
)

cc_binary(
    name = "series_data_encoder",
    srcs = ["series_data_encoder_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:series_data",
        "//:benchmark",
    ],
)

cc_binary(
    name = "serializer",
    srcs = ["serializer_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:series_data",
        "//:benchmark",
    ],
)