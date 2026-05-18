"""Module extension declaring third-party C/C++ dependencies with custom BUILD
files and/or local patches.

These declarations live here (rather than in BCR) because we maintain custom
BUILD files and/or local patches per repository. For each repository we keep
its canonical name so that existing `@name//:target` references in BUILD files
and patches resolve correctly after `use_repo(third_party_deps, "name")` in
MODULE.bazel.
"""

load("@bazel_tools//tools/build_defs/repo:git.bzl", "git_repository", "new_git_repository")
load("@bazel_tools//tools/build_defs/repo:http.bzl", "http_archive", "http_file")

def _third_party_deps_impl(_ctx):
    git_repository(
        name = "gtest",
        commit = "dc3c9eda2f02ba32de9329dd27ace7e527f492dc",
        patches = [
            Label("//third_party/patches/gtest:0001-no-werror.patch"),
        ],
        remote = "https://github.com/google/googletest",
        shallow_since = "1656350095 -0400",
    )

    http_archive(
        name = "google_benchmark",
        patches = [
            Label("//third_party/patches/google_benchmark:0001-BUILD.bazel.patch"),
        ],
        sha256 = "9631341c82bac4a288bef951f8b26b41f69021794184ece969f8473977eaa340",
        strip_prefix = "benchmark-1.9.5/",
        url = "https://github.com/google/benchmark/archive/refs/tags/v1.9.5.tar.gz",
    )

    http_archive(
        name = "tracy",
        build_file = Label("//third_party:tracy.BUILD"),
        sha256 = "ce2fb5b89aeb6db8401d7efe1bfe8393b7a81ca551273e8c6dd46ed37c02a040",
        strip_prefix = "tracy-0.12.0/",
        url = "https://github.com/wolfpld/tracy/archive/refs/tags/v0.12.0.tar.gz",
    )

    http_archive(
        name = "jemalloc",
        build_file = Label("//third_party:jemalloc.BUILD"),
        patch_args = ["-p1"],
        patches = [
            Label("//third_party/patches/jemalloc:0001-musl-noexcept-fix.patch"),
            Label("//third_party/patches/jemalloc:0002-manual-init.patch"),
            Label("//third_party/patches/jemalloc:0003-svacer_fixes.patch"),
            Label("//third_party/patches/jemalloc:0004-werror_fixes.patch"),
        ],
        sha256 = "2db82d1e7119df3e71b7640219b6dfe84789bc0537983c3b7ac4f7189aecfeaa",
        strip_prefix = "jemalloc-5.3.0/",
        url = "https://github.com/jemalloc/jemalloc/releases/download/5.3.0/jemalloc-5.3.0.tar.bz2",
    )

    http_archive(
        name = "parallel_hashmap",
        build_file = Label("//third_party:parallel_hashmap.BUILD"),
        patches = [
            Label("//third_party/patches/parallel_hashmap:0001-svacer_fixes.patch"),
            Label("//third_party/patches/parallel_hashmap:btree.h.patch"),
            Label("//third_party/patches/parallel_hashmap:0003-custom_hash_eq.patch"),
        ],
        sha256 = "b61435437713e2d98ce2a5539a0bff7e6e9e6a6b9fe507dbf490a852b8c2904f",
        strip_prefix = "parallel-hashmap-1.35",
        url = "https://github.com/greg7mdp/parallel-hashmap/archive/refs/tags/1.35.zip",
    )

    http_archive(
        name = "scope_exit",
        build_file = Label("//third_party:scope_exit.BUILD"),
        sha256 = "9428fcdf00714e25fc7c67c28faf0821787b3234165b542a7f2223f464747d83",
        strip_prefix = "SC22WG21_Papers-7f9c58dabea6872f86f7960157e2ca38880e14cb/workspace/P0052_scope_exit/src",
        url = "https://github.com/PeterSommerlad/SC22WG21_Papers/archive/7f9c58dabea6872f86f7960157e2ca38880e14cb.zip",
    )

    http_archive(
        name = "lz4",
        build_file = Label("//third_party:lz4.BUILD"),
        patches = [
            Label("//third_party/patches/lz4:0001-remove_visibility_attribute.patch"),
            Label("//third_party/patches/lz4:0002-add_allocated_memory.patch"),
            Label("//third_party/patches/lz4:0003-fix_offsetof_calculation.patch"),
            Label("//third_party/patches/lz4:0004-svacer_fixes.patch"),
        ],
        sha256 = "658ba6191fa44c92280d4aa2c271b0f4fbc0e34d249578dd05e50e76d0e5efcc",
        strip_prefix = "lz4-1.9.2",
        url = "https://github.com/lz4/lz4/archive/v1.9.2.tar.gz",
    )

    http_archive(
        name = "roaring",
        build_file = Label("//third_party:roaring.BUILD"),
        patch_args = ["-p1"],
        patch_cmds = [
            "cp cpp/* include/roaring",
        ],
        patches = [
            Label("//third_party/patches/roaring:0001-disable-test-dependencies.patch"),
            Label("//third_party/patches/roaring:0002-svacer_fixes.patch"),
            Label("//third_party/patches/roaring:0003-werror_fixes.patch"),
        ],
        sha256 = "a037e12a3f7c8c2abb3e81fc9669c23e274ffa2d8670d2034a2e05969e53689b",
        strip_prefix = "CRoaring-1.3.0/",
        url = "https://github.com/RoaringBitmap/CRoaring/archive/refs/tags/v1.3.0.zip",
    )

    http_archive(
        name = "xxHash",
        build_file = Label("//third_party:xxHash.BUILD"),
        sha256 = "aae608dfe8213dfd05d909a57718ef82f30722c392344583d3f39050c7f29a80",
        strip_prefix = "xxHash-0.8.3/",
        url = "https://github.com/Cyan4973/xxHash/archive/refs/tags/v0.8.3.tar.gz",
    )

    http_archive(
        name = "com_google_absl",
        patches = [
            Label("//third_party/patches/com_google_absl:0001-no-werror.patch"),
            Label("//third_party/patches/com_google_absl:0002-svacer_fixes.patch"),
            Label("//third_party/patches/com_google_absl:0003-null_dereference_fixes.patch"),
            Label("//third_party/patches/com_google_absl:0004-array_bounds_fixes.patch"),
        ],
        sha256 = "f8903111260a18d2cc4618cd5bf35a22bcc28f372ebe4f04024b49e88a2e16c1",
        strip_prefix = "abseil-cpp-20240116.rc1/",
        url = "https://github.com/abseil/abseil-cpp/archive/refs/tags/20240116.rc1.tar.gz",
    )

    git_repository(
        name = "snappy",
        commit = "27f34a580be4a3becf5f8c0cba13433f53c21337",
        patches = [
            Label("//third_party/patches/snappy:0001-svacer_fixes.patch"),
        ],
        remote = "https://github.com/google/snappy",
        shallow_since = "1689185568 -0700",
    )

    http_archive(
        name = "re2",
        patches = [
            Label("//third_party/patches/re2:0001-no-werror.patch"),
            Label("//third_party/patches/re2:0002-svacer_fixes.patch"),
            Label("//third_party/patches/re2:0003-null_dereference_fixes.patch"),
        ],
        sha256 = "cd191a311b84fcf37310e5cd876845b4bf5aee76fdd755008eef3b6478ce07bb",
        strip_prefix = "re2-2024-02-01/",
        url = "https://github.com/google/re2/archive/refs/tags/2024-02-01.tar.gz",
    )

    new_git_repository(
        name = "cedar",
        build_file = Label("//third_party:cedar.BUILD"),
        commit = "38fa7f615f14bf867834c796945841d54cb45e0f",
        patch_args = ["-p1"],
        patches = [
            Label("//third_party/patches/cedar:0001-cedarpp.h.patch"),
        ],
        remote = "https://github.com/DevO2012/cedar",
    )

    new_git_repository(
        name = "quasis_crypto",
        build_file = Label("//third_party:quasis_crypto.BUILD"),
        commit = "7d3c4c648b1013e37d25d247c31078033b10d172",
        patch_args = ["-p1"],
        patches = [
            Label("//third_party/patches/quasis_crypto:0001-md5.hh.patch"),
        ],
        remote = "https://github.com/quasis/crypto",
    )

    http_archive(
        name = "simdutf",
        build_file = Label("//third_party:simdutf.BUILD"),
        sha256 = "66c85f591133e3baa23cc441d6e2400dd2c94c4902820734ddbcd9e04dd3988b",
        url = "https://github.com/simdutf/simdutf/releases/download/v6.2.0/singleheader.zip",
    )

    http_file(
        name = "fastfloat_header",
        downloaded_file_path = "fastfloat/fast_float.h",
        sha256 = "1335e82c61fda54476ecbd94b92356deebeb3f0122802c3f103ee528ac08624e",
        url = "https://github.com/fastfloat/fast_float/releases/download/v8.0.0/fast_float.h",
    )

third_party_deps = module_extension(implementation = _third_party_deps_impl)
