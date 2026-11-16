"""Expose the clang-tidy installed in the build environment."""

def _local_clang_tidy_impl(ctx):
    binary = ctx.which("clang-tidy")
    if binary == None:
        fail("Install clang-tidy in the build environment and add it to PATH")
    ctx.symlink(binary, "clang-tidy")
    ctx.file("BUILD.bazel", 'exports_files(["clang-tidy"], visibility = ["//visibility:public"])')

local_clang_tidy = repository_rule(
    implementation = _local_clang_tidy_impl,
    local = True,
    configure = True,
    environ = ["PATH"],
)
