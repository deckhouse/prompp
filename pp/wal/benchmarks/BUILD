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
