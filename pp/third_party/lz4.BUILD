# Description:
#   LZ4 library

licenses(["notice"])  # BSD license

exports_files(["LICENSE"])

cc_library(
    name = "lz4",
    srcs = [
        "lib/lz4.c",
        "lib/lz4frame.c",
        "lib/lz4hc.c",
    ],
    hdrs = [
        "lib/lz4.h",
        "lib/lz4frame.h",
        "lib/lz4hc.h",
        "lib/xxhash.h",
    ],
    defines = [
        "XXH_PRIVATE_API",
        # Expand LZ4FLIB_API/LZ4LIB_API without the visibility attribute, which is
        # what patch 0001-remove_visibility_attribute did until 1.10.0. Its original
        # rationale is not recorded (it arrived with the rest of the lz4 patches in
        # #144), so the effect is preserved rather than dropped. Upstream guards both
        # macros with #ifndef since 1.10.0, so defining them empty needs no patch.
        "LZ4LIB_VISIBILITY=",
        "LZ4FLIB_VISIBILITY=",
    ],
    includes = [
        "lib",
    ],
    textual_hdrs = [
        "lib/xxhash.c",
        "lib/lz4.c",
    ],
    visibility = ["//visibility:public"],
)
