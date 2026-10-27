load("@rules_foreign_cc//foreign_cc:cmake.bzl", "cmake")

filegroup(
    name = "src",
    srcs = glob([
        "**"
    ]),
    visibility = ["//visibility:public"],
)

config_setting(
    name = "debug",
    values = {"compilation_mode": "dbg"},
)

cmake(
    name = "tracy_client",
    lib_source = ":src",
    generate_args = [
        "-DTRACY_ENABLE=ON",
        "-DTRACY_VERBOSE=ON",
        "-DTRACY_NO_EXIT=ON",
        "-DTRACY_NO_VSYNC_CAPTURE=ON",
        "-DTRACY_LIBUNWIND_BACKTRACE=ON",
    ],
    copts = [
        "-Wno-error"
    ],
    build_args = ["-j `nproc`"],
    targets = ["TracyClient"],
    # Tracy installs non-Release configurations into a configuration subdirectory.
    out_static_libs = select({
        ":debug": ["Debug/libTracyClient.a"],
        "//conditions:default": ["libTracyClient.a"],
    }),
    visibility = ["//visibility:private"],
)

# Built from the client's tag: the Tracy protocol is not compatible across
# versions.
cmake(
    name = "tracy_capture",
    lib_source = ":src",
    working_directory = "capture",
    generate_args = [
        # Release because dropping default_compiler_flags drops -O3; BINDIR
        # because neither tool includes GNUInstallDirs.
        "-DCMAKE_BUILD_TYPE=Release",
        "-DCMAKE_INSTALL_BINDIR=bin",
    ],
    # Named because the toolchain links through gcc, and CMake appends this after
    # the objects, unlike the linker flags dropped below.
    cache_entries = {
        "CMAKE_CXX_STANDARD_LIBRARIES": "-lstdc++ -lm",
    },
    # Our -Wextra is fatal under capstone's own -Werror, and our
    # -static-libstdc++ is placed ahead of the objects that need it.
    features = [
        "-default_compiler_flags",
        "-default_linker_flags",
    ],
    build_args = ["-j `nproc`"],
    # CPM fetches capstone, zstd, PPQSort and json during the build.
    tags = ["requires-network"],
    # Both, or `cmake --install` fails on the one that was not built.
    targets = [
        "tracy-capture",
        "tracy-capture-daemon",
    ],
    out_binaries = [
        "tracy-capture",
        "tracy-capture-daemon",
    ],
    visibility = ["//visibility:public"],
)

cmake(
    name = "tracy_csvexport",
    lib_source = ":src",
    working_directory = "csvexport",
    generate_args = [
        # Release because dropping default_compiler_flags drops -O3; BINDIR
        # because neither tool includes GNUInstallDirs.
        "-DCMAKE_BUILD_TYPE=Release",
        "-DCMAKE_INSTALL_BINDIR=bin",
    ],
    cache_entries = {
        "CMAKE_CXX_STANDARD_LIBRARIES": "-lstdc++ -lm",
    },
    features = [
        "-default_compiler_flags",
        "-default_linker_flags",
    ],
    build_args = ["-j `nproc`"],
    tags = ["requires-network"],
    targets = ["tracy-csvexport"],
    out_binaries = ["tracy-csvexport"],
    visibility = ["//visibility:public"],
)

# rules_foreign_cc names an output group after each produced file, which is how
# `bazel cquery --output=files` resolves to a single path.
filegroup(
    name = "tracy_capture_bin",
    srcs = [":tracy_capture"],
    output_group = "tracy-capture",
    visibility = ["//visibility:public"],
)

filegroup(
    name = "tracy_csvexport_bin",
    srcs = [":tracy_csvexport"],
    output_group = "tracy-csvexport",
    visibility = ["//visibility:public"],
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
