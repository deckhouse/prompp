load("@rules_foreign_cc//foreign_cc:cmake.bzl", "cmake")

filegroup(
    name = "src",
    srcs = glob([
        "**"
    ]),
    visibility = ["//visibility:public"],
)

cmake(
    name = "tracy_client",
    lib_source = ":src",
    generate_args = [
        "-DTRACY_ENABLE=ON",
        "-DTRACY_VERBOSE=ON",
        "-DTRACY_NO_EXIT=ON",
    ],
    copts = [
        "-Wno-error"
    ],
    build_args = ["-j `nproc`"],
    targets = ["TracyClient"],
    out_static_libs = ["libTracyClient.a"],
    visibility = ["//visibility:private"],
)

cc_library(
    name = "tracy_headers",
    hdrs = glob(["public/**/*.h", "public/**/*.hpp"]),
    strip_include_prefix = "public",
    visibility = ["//visibility:public"],
)

cc_library(
    name = "tracy",
    hdrs = glob(["public/**/*.h", "public/**/*.hpp"]),
    deps = [
        ":tracy_headers",
        ":tracy_client",
    ],
    strip_include_prefix = "public",
    visibility = ["//visibility:public"],
)