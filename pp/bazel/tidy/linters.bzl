"""Use upstream clang-tidy aspects for production and shallow analysis."""

load("@aspect_rules_lint//lint:clang_tidy.bzl", "lint_clang_tidy_aspect")

def _clang_tidy(args = []):
    return lint_clang_tidy_aspect(
        binary = Label("//:clang_tidy"),
        global_config = [Label("//:.clang-tidy")],
        angle_includes_are_system = True,
        rule_kinds = ["cc_binary", "cc_library", "cc_test"],
        args = args,
    )

clang_tidy = _clang_tidy()

# Keep shallow analysis, but defer newly exposed header diagnostics.
# Source-file diagnostics and their failure policy remain enabled.
clang_tidy_shallow = _clang_tidy(args = [
    "--header-filter=^$",
    "--extra-arg=-Xclang",
    "--extra-arg=-analyzer-config",
    "--extra-arg=-Xclang",
    "--extra-arg=mode=shallow",
    "--extra-arg=-Xclang",
    "--extra-arg=-analyzer-inline-max-stack-depth",
    "--extra-arg=-Xclang",
    "--extra-arg=1",
])
