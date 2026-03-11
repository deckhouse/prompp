package(default_visibility = ["//visibility:public"])

cc_binary(
    name = "find_or_emplace",
    srcs = ["find_or_emplace_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:primitives",
        "//:benchmark",
    ],
)