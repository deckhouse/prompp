package(default_visibility = ["//visibility:public"])

filegroup(
    name = "sources",
    srcs = glob(["*_benchmark.cpp"]),
    visibility = ["//visibility:public"],
)

cc_binary(
    name = "find_or_emplace",
    srcs = ["find_or_emplace_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:primitives",
        "//:benchmark",
    ],
)