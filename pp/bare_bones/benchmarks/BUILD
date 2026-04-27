package(default_visibility = ["//visibility:public"])

filegroup(
    name = "sources",
    srcs = glob(["*_benchmark.cpp"]),
    visibility = ["//visibility:public"],
)

cc_binary(
    name = "stream_v_byte_sequence",
    srcs = ["stream_v_byte_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:bare_bones",
        "//:benchmark",
    ],
)

cc_binary(
    name = "encoding",
    srcs = ["encoding_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:bare_bones",
        "//:benchmark",
    ],
)
