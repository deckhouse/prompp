#!/usr/bin/env python3
"""Verify project header diagnostics with the installed clang-tidy and configuration."""

import argparse
import json
from pathlib import Path
import shlex
import subprocess
import tempfile


def effective_arguments(workspace, bazel, aspect, target, source_suffix):
    command = bazel + [
        "aquery", "--compilation_mode=fastbuild", "--config=clang-tidy",
        f"--aspects=//bazel/tidy:linters.bzl%{aspect}",
        "--output_groups=rules_lint_human", "--platform_suffix=tidy",
        "--include_param_files", "--output=jsonproto",
        f'mnemonic("AspectRulesLintClangTidy", {target})',
    ]
    result = subprocess.run(command, cwd=workspace, text=True, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    if result.returncode:
        raise RuntimeError(f"Cannot inspect {aspect} actions:\n{result.stderr}")
    graph = json.loads(result.stdout)
    for action in graph.get("actions", []):
        arguments = action.get("arguments", [])
        if not any(arg.endswith(source_suffix) for arg in arguments):
            continue
        executable = next(i for i, arg in enumerate(arguments) if arg.endswith("/clang_tidy.exe"))
        before = arguments[executable + 1:arguments.index("--")]
        options = []
        for arg in before:
            if arg.endswith(source_suffix):
                continue
            if arg.startswith("--config-file="):
                path = workspace / arg.split("=", 1)[1]
                arg = f"--config-file={path}"
            options.append(arg)
        compiler = arguments[arguments.index("--") + 1:]
        if not compiler:
            raise RuntimeError(f"No compiler arguments in {aspect} action")
        # LZ4 declares a system include directory in CcInfo. The aspect must emit -I.
        include_kind = next((compiler[i - 1] for i, arg in enumerate(compiler)
                             if i and "third_party_deps~lz4" in arg and arg.endswith("/lib")), None)
        return options, include_kind
    raise RuntimeError(f"No {source_suffix} lint action for {aspect}")


def check(command, root, expected):
    result = subprocess.run(command, cwd=root, text=True, stdout=subprocess.PIPE, stderr=subprocess.STDOUT)
    diagnostics = [line for line in result.stdout.splitlines() if ": error:" in line or ": warning:" in line]
    if result.returncode == 0 or len(diagnostics) != 1 or expected not in result.stdout:
        raise RuntimeError(f"Expected one fatal project header diagnostic: {command}\n{result.stdout}")
    if any(symbol in result.stdout for symbol in ("VendoredWarning", "ExternalWarning", "SystemWarning", "LibclangWarning")):
        raise RuntimeError(f"Dependency diagnostic escaped the filter:\n{result.stdout}")
    if "[google-explicit-constructor,-warnings-as-errors]" not in diagnostics[0]:
        raise RuntimeError(f"Project warning did not retain its failure policy:\n{result.stdout}")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--bazel", default="bazel")
    options = parser.parse_args()
    workspace = Path(__file__).resolve().parents[1]
    bazel = shlex.split(options.bazel)
    modes = [
        ("clang_tidy", "//:bare_bones_exceptions", "bare_bones/exception.cpp"),
        ("clang_tidy_shallow", "//:bare_bones_test", "bare_bones/memory_tests.cpp"),
        ("clang_tidy_codegen", "@entrypoint_codegen//:entrypoint_codegen_test", "clang_adapter/parse_tests.cpp"),
    ]
    effective = {mode: effective_arguments(workspace, bazel, mode, target, suffix)
                 for mode, target, suffix in modes}
    cases = 0
    with tempfile.TemporaryDirectory(prefix="prompp-tidy-filter-") as directory:
        root = Path(directory)
        headers = {
            "new_component/warning.h": "ProjectWarning",
            "third_party/warning.h": "VendoredWarning",
            "external/warning.h": "ExternalWarning",
            "system/warning.h": "SystemWarning",
            "pp/tools/new_component/warning.h": "ToolWarning",
            "sandbox/execroot/_main/new_component/warning.h": "SandboxWarning",
        }
        for name, symbol in headers.items():
            path = root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(f"#pragma once\nstruct {symbol} {{ {symbol}(int); }};\n")
        spellings = [
            ('"new_component/warning.h"', "ProjectWarning"),
            ('"./new_component/warning.h"', "ProjectWarning"),
            ('"pp/tools/new_component/warning.h"', "ToolWarning"),
            (f'"{root / "new_component/warning.h"}"', "ProjectWarning"),
            (f'"{root / "sandbox/execroot/_main/new_component/warning.h"}"', "SandboxWarning"),
            ("<new_component/warning.h>", "ProjectWarning"),
        ]
        for mode in ("clang_tidy", "clang_tidy_shallow"):
            arguments, include_kind = effective[mode]
            if include_kind != "-I":
                raise RuntimeError(f"{mode} marks CcInfo include directories as {include_kind}, expected -I")
            for source_name in ("owner/production.cpp", "owner/item_tests.cpp", "owner/benchmarks/item.cpp"):
                source = root / source_name
                source.parent.mkdir(parents=True, exist_ok=True)
                for include, expected in spellings:
                    source.write_text(
                        f"#include {include}\n"
                        '#include "third_party/warning.h"\n'
                        '#include "external/warning.h"\n'
                        "#include <warning.h>\n"
                    )
                    for absolute in (False, True):
                        command = ["clang-tidy", *arguments, "--checks=-*,google-explicit-constructor",
                                   str(source if absolute else source.relative_to(root)),
                                   "--", "-std=c++23", include_kind, str(root),
                                   "-isystem", str(root / "system")]
                        check(command, root, expected)
                        cases += 1
        for suffix in ("", "~", "+"):
            module = root / ("external/entrypoint_codegen" + suffix)
            header = module / "facts/warning.h"
            header.parent.mkdir(parents=True, exist_ok=True)
            header.write_text("#pragma once\nstruct CodegenWarning { CodegenWarning(int); };\n")
            for sibling in ("entrypoint_codegen~~libclang_extension~system_libclang",
                            "entrypoint_codegen+libclang_extension+system_libclang"):
                path = root / "external" / sibling / "warning.h"
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text("#pragma once\nstruct LibclangWarning { LibclangWarning(int); };\n")
                source = module / "item_tests.cpp"
                source.write_text('#include <facts/warning.h>\n'
                                  '#include "new_component/warning.h"\n'
                                  f'#include "external/{sibling}/warning.h"\n'
                                  '#include "third_party/warning.h"\n'
                                  '#include <warning.h>\n')
                for absolute in (False, True):
                    command = ["clang-tidy", *effective["clang_tidy_codegen"][0],
                               "--checks=-*,google-explicit-constructor",
                               str(source if absolute else source.relative_to(root)),
                               "--", "-std=c++23", "-I", str(root),
                               "-isystem", str(module), "-isystem", str(root / "system")]
                    check(command, root, "CodegenWarning")
                    cases += 1
    print(f"clang-tidy header filter: {cases} cases passed")


if __name__ == "__main__":
    main()
