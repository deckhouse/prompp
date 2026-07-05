def _first_non_empty(values):
    for value in values:
        if value:
            return value
    return ""

def _libclang_path(repository_ctx, attr_name, env_name, llvm_root, suffix):
    attr_value = getattr(repository_ctx.attr, attr_name)
    env_value = repository_ctx.os.environ.get(env_name, "")
    if attr_value:
        return attr_value
    if env_value:
        return env_value
    if llvm_root:
        return llvm_root + suffix
    return ""

def _libclang_repository_impl(repository_ctx):
    llvm_root = _first_non_empty([
        repository_ctx.attr.llvm_root,
        repository_ctx.os.environ.get("LIBCLANG_LLVM_ROOT", ""),
        "/usr/lib/llvm-21",
    ])
    include_dir = _libclang_path(repository_ctx, "include_dir", "LIBCLANG_INCLUDE_DIR", llvm_root, "/include")
    lib_dir = _libclang_path(repository_ctx, "lib_dir", "LIBCLANG_LIB_DIR", llvm_root, "/lib")
    library_name = _first_non_empty([
        repository_ctx.attr.library_name,
        repository_ctx.os.environ.get("LIBCLANG_LIBRARY", ""),
        "clang",
    ])

    if not include_dir:
        fail("libclang include directory is not configured")
    if not lib_dir:
        fail("libclang library directory is not configured")

    include_path = repository_ctx.path(include_dir)
    lib_path = repository_ctx.path(lib_dir)
    if not include_path.exists:
        fail("libclang include directory does not exist: " + include_dir)
    if not lib_path.exists:
        fail("libclang library directory does not exist: " + lib_dir)

    copy_result = repository_ctx.execute([
        "cp",
        "-R",
        include_dir,
        "include",
    ])
    if copy_result.return_code != 0:
        fail("failed to copy libclang headers: " + copy_result.stderr)

    linkopts = [
        "-L" + lib_dir,
        "-l" + library_name,
    ]
    if repository_ctx.attr.rpath:
        linkopts.append("-Wl,-rpath," + lib_dir)

    repository_ctx.file(
        "BUILD.bazel",
        """package(default_visibility = ["//visibility:public"])

cc_library(
    name = "libclang",
    hdrs = glob(["include/**/*.h"]),
    includes = ["include"],
    linkopts = {linkopts},
)

alias(
    name = "headers",
    actual = ":libclang",
)
""".format(linkopts = repr(linkopts)),
    )

libclang_repository = repository_rule(
    implementation = _libclang_repository_impl,
    attrs = {
        "include_dir": attr.string(),
        "lib_dir": attr.string(),
        "library_name": attr.string(default = "clang"),
        "llvm_root": attr.string(),
        "rpath": attr.bool(default = True),
    },
    environ = [
        "LIBCLANG_INCLUDE_DIR",
        "LIBCLANG_LIB_DIR",
        "LIBCLANG_LIBRARY",
        "LIBCLANG_LLVM_ROOT",
    ],
)

_libclang_config = tag_class(
    attrs = {
        "include_dir": attr.string(),
        "lib_dir": attr.string(),
        "library_name": attr.string(default = "clang"),
        "llvm_root": attr.string(),
        "rpath": attr.bool(default = True),
    },
)

def _libclang_extension_impl(module_ctx):
    configs = []
    for module in module_ctx.modules:
        for config in module.tags.configure:
            configs.append(config)

    if len(configs) > 1:
        fail("libclang_extension accepts at most one configure(...) tag")

    if configs:
        config = configs[0]
        libclang_repository(
            name = "system_libclang",
            include_dir = config.include_dir,
            lib_dir = config.lib_dir,
            library_name = config.library_name,
            llvm_root = config.llvm_root,
            rpath = config.rpath,
        )
        return

    libclang_repository(name = "system_libclang")

libclang_extension = module_extension(
    implementation = _libclang_extension_impl,
    tag_classes = {
        "configure": _libclang_config,
    },
)
