# Description:
#   Jemalloc library

load("@rules_foreign_cc//foreign_cc:configure.bzl", "configure_make")

licenses(["notice"])  # LGPL license

exports_files(["COPYING"])

filegroup(
    name = "src",
    srcs = glob([
        "**",
    ]),
    visibility = ["//visibility:public"],
)

configure_make(
    name = "make_jemalloc",
    args = ["-j `nproc`"],
    autoconf = True,
    configure_in_place = True,
    configure_options = [
        "--enable-xmalloc",
        "--with-lg-hugepage=21",
        "--enable-prof",
        "--enable-shared=\"no\"",
        "--enable-prof-libunwind=\"1\"",
    ] + select({
        "@//bazel/toolchain:arm64": ["--with-lg-page=\"16\""],
        "//conditions:default": ["--with-lg-page=\"12\""],
    }),
    copts = [
        "-Wno-error=builtin-declaration-mismatch",
    ],
    lib_source = ":src",
    out_static_libs = [
        "libjemalloc.a",
    ],
    visibility = ["//visibility:public"],
)

cc_library(
    name = "jemalloc",
    visibility = ["//visibility:public"],
    deps = [":make_jemalloc"],
)
