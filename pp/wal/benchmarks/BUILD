package(default_visibility = ["//visibility:public"])

filegroup(
    name = "sources",
    srcs = glob(["*_benchmark.cpp"]),
    visibility = ["//visibility:public"],
)

cc_binary(
    name = "scraper",
    srcs = ["scraper_benchmark.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:wal",
        "//:benchmark",
    ],
)

cc_binary(
    name = "parse_check",
    srcs = ["parse_check.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:wal",
    ],
)

cc_binary(
    name = "offset_overflow_demo",
    srcs = ["offset_overflow_demo.cpp"],
    malloc = "@jemalloc",
    deps = [
        "//:wal",
    ],
)
