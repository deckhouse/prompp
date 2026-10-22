"""Use the upstream clang-tidy aspect on existing C++ targets."""

load("@aspect_rules_lint//lint:clang_tidy.bzl", "lint_clang_tidy_aspect")

clang_tidy = lint_clang_tidy_aspect(
    binary = Label("//:clang_tidy"),
    global_config = [Label("//:.clang-tidy")],
    rule_kinds = ["cc_binary", "cc_library", "cc_test"],
)
