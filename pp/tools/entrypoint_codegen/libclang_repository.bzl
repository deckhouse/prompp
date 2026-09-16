def _first_non_empty(values):
    for value in values:
        if value:
            return value
    return ""

def _check_expected_version(repository_ctx, llvm_root):
    expected_version = repository_ctx.attr.expected_version
    if not expected_version:
        return

    version_tool = _first_non_empty([
        repository_ctx.attr.version_tool,
        llvm_root + "/bin/clang" if llvm_root else "",
    ])
    if not version_tool or not repository_ctx.path(version_tool).exists:
        fail("clang version tool is required to validate libclang version " + expected_version)

    version = repository_ctx.execute([
        version_tool,
        "--version",
    ])
    if version.return_code != 0:
        fail("failed to query libclang LLVM version with " + version_tool + ": " + version.stderr)

    actual_version = version.stdout.splitlines()[0].strip()
    if expected_version not in actual_version:
        fail("expected libclang LLVM version " + expected_version + ", got " + actual_version + " from " + version_tool)

def _system_libclang_repository_impl(repository_ctx):
    llvm_root = _first_non_empty([
        repository_ctx.attr.llvm_root,
        repository_ctx.getenv("LIBCLANG_LLVM_ROOT"),
    ])

    if repository_ctx.attr.llvm_root_label:
        llvm_root = str(repository_ctx.path(repository_ctx.attr.llvm_root_label).dirname)

    include_dir = _first_non_empty([
        repository_ctx.attr.include_dir,
        repository_ctx.getenv("LIBCLANG_INCLUDE_DIR"),
        llvm_root + "/include" if llvm_root else "",
    ])
    lib_dir = _first_non_empty([
        repository_ctx.attr.lib_dir,
        repository_ctx.getenv("LIBCLANG_LIB_DIR"),
        llvm_root + "/lib" if llvm_root else "",
    ])
    library = _first_non_empty([
        repository_ctx.attr.library,
        repository_ctx.getenv("LIBCLANG_LIBRARY"),
        "clang",
    ])

    if not include_dir or not repository_ctx.path(include_dir).exists:
        fail("libclang include directory does not exist. Set LIBCLANG_INCLUDE_DIR or LIBCLANG_LLVM_ROOT.")
    if not lib_dir or not repository_ctx.path(lib_dir).exists:
        fail("libclang library directory does not exist. Set LIBCLANG_LIB_DIR or LIBCLANG_LLVM_ROOT.")

    _check_expected_version(repository_ctx, llvm_root)

    copy_headers = repository_ctx.execute([
        "/bin/cp",
        "-R",
        include_dir,
        "include",
    ])
    if copy_headers.return_code != 0:
        fail("failed to copy libclang headers: " + copy_headers.stderr)

    repository_ctx.symlink(lib_dir, "lib")
    repository_ctx.file(
        "BUILD.bazel",
        """
cc_library(
    name = "libclang",
    hdrs = glob(["include/**/*.h"]),
    includes = ["include"],
    linkopts = [
        "-L{lib_dir}",
        "-l{library}",
        "-Wl,-rpath,{lib_dir}",
    ],
    visibility = ["//visibility:public"],
)
""".format(
            lib_dir = lib_dir,
            library = library,
        ),
    )

_system_libclang_repository = repository_rule(
    implementation = _system_libclang_repository_impl,
    attrs = {
        "expected_version": attr.string(),
        "include_dir": attr.string(),
        "lib_dir": attr.string(),
        "library": attr.string(),
        "llvm_root_label": attr.label(),
        "llvm_root": attr.string(),
        "version_tool": attr.string(),
    },
    environ = [
        "LIBCLANG_INCLUDE_DIR",
        "LIBCLANG_LIB_DIR",
        "LIBCLANG_LIBRARY",
        "LIBCLANG_LLVM_ROOT",
    ],
)

_libclang_configure = tag_class(attrs = {
    "expected_version": attr.string(),
    "include_dir": attr.string(),
    "lib_dir": attr.string(),
    "library": attr.string(),
    "llvm_root_label": attr.label(),
    "llvm_root": attr.string(),
    "version_tool": attr.string(),
})

def _libclang_extension_impl(module_ctx):
    config = None

    for module in module_ctx.modules:
        for configure in module.tags.configure:
            next_config = struct(
                expected_version = configure.expected_version,
                include_dir = configure.include_dir,
                lib_dir = configure.lib_dir,
                library = configure.library,
                llvm_root = configure.llvm_root,
                llvm_root_label = configure.llvm_root_label,
                version_tool = configure.version_tool,
            )
            if config == None or module.is_root:
                config = next_config

    if config == None:
        fail("libclang_extension requires a configure tag from the root module")

    _system_libclang_repository(
        name = "system_libclang",
        expected_version = config.expected_version,
        include_dir = config.include_dir,
        lib_dir = config.lib_dir,
        library = config.library,
        llvm_root = config.llvm_root,
        llvm_root_label = config.llvm_root_label,
        version_tool = config.version_tool,
    )

libclang_extension = module_extension(
    implementation = _libclang_extension_impl,
    tag_classes = {"configure": _libclang_configure},
)
