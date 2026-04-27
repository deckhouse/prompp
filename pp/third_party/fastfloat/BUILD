genrule(
    name = "fast_float_copy",
    srcs = ["@fastfloat_header//file"],
    outs = ["include/fastfloat/fast_float.h"],
    cmd = "cp $< $@",
)

cc_library(
    name = "fastfloat",
    hdrs = [":fast_float_copy"],
    strip_include_prefix = "include",
    visibility = ["//visibility:public"],
)
