"""Use upstream clang-tidy aspects for production and shallow analysis."""

load("@aspect_rules_lint//lint:clang_tidy.bzl", "lint_clang_tidy_aspect")

def _clang_tidy(args = []):
    return lint_clang_tidy_aspect(
        binary = Label("//:clang_tidy"),
        global_config = [Label("//:.clang-tidy")],
        angle_includes_are_system = False,
        rule_kinds = ["cc_binary", "cc_library", "cc_test"],
        args = args,
    )

clang_tidy = _clang_tidy()

clang_tidy_shallow = _clang_tidy(args = [
    "--extra-arg=-Xclang",
    "--extra-arg=-analyzer-config",
    "--extra-arg=-Xclang",
    "--extra-arg=mode=shallow",
    "--extra-arg=-Xclang",
    "--extra-arg=-analyzer-inline-max-stack-depth",
    "--extra-arg=-Xclang",
    "--extra-arg=1",
])

# This first-party module is external to the main Bazel workspace.
# Its angle includes can retain system classification in the external toolchain.
# Match its repository exactly and keep libclang extension repositories excluded.
clang_tidy_codegen = _clang_tidy(args = [
    "--header-filter=(^|/)external/entrypoint_codegen[~+]?/",
    "--exclude-header-filter=^$",
    "--system-headers",
])
