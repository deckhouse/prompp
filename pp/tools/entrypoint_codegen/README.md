# entrypoint_codegen

`entrypoint_codegen` is a libclang-based scanner for Prom++ entrypoint
translation units. It can run as a standalone Bazel module from this directory,
and pp also exposes a lightweight report target at `//:entrypoint_fact_checking`.

The pp report target is intentionally not wired into `//:entrypoint` builds yet.
It is a manual diagnostic/reporting surface, not a build gate.

## What It Extracts

- `prompp_*` entrypoint function definitions.
- Bridge kind from `annotate("prompp.entrypoint.cgo")` or
  `annotate("prompp.entrypoint.fastcgo")`.
- Function return type, linkage, parameter count, parameter names and parameter
  types.
- Local layout declarations:
  - `struct Arguments { ... }`
  - `struct Result { ... }`
  - `using Arguments = struct { ... };`
  - `using Result = struct { ... };`
- Structured diagnostics:
  - missing `extern "C"` linkage
  - missing entrypoint annotation
  - unsupported return type
  - unsupported parameter count or parameter type
  - `args`/`res` layout mismatches
  - libclang parse diagnostics

## Standalone Build

From `tools/entrypoint_codegen`:

```bash
bazel build //:entrypoint_codegen
```

The standalone module expects libclang to be provided by the local environment.
The default configuration in `MODULE.bazel` currently expects LLVM/libclang
`21.1.8` under `/usr/lib/llvm-21`:

```starlark
libclang = use_extension("//:libclang_repository.bzl", "libclang_extension")
libclang.configure(
    expected_version = "21.1.8",
    llvm_root = "/usr/lib/llvm-21",
)
use_repo(libclang, "system_libclang")
```

You can override the location from the command line:

```bash
bazel build \
  --repo_env=LIBCLANG_LLVM_ROOT=/usr/lib/llvm-21 \
  //:entrypoint_codegen
```

If headers and libraries are installed separately:

```bash
bazel build \
  --repo_env=LIBCLANG_INCLUDE_DIR=/opt/llvm/include \
  --repo_env=LIBCLANG_LIB_DIR=/opt/llvm/lib \
  --repo_env=LIBCLANG_LIBRARY=clang \
  //:entrypoint_codegen
```

## Standalone Run

Generate JSON:

```bash
bazel run //:entrypoint_codegen -- \
  --output=/tmp/entrypoint_facts.json \
  /workspaces/prompp/pp/entrypoint/common.cpp \
  -- \
  -std=c++2b \
  -I/workspaces/prompp/pp
```

Print compiler-style diagnostics instead of JSON:

```bash
bazel run //:entrypoint_codegen -- \
  --mode=lint \
  /workspaces/prompp/pp/entrypoint/common.cpp \
  -- \
  -std=c++2b \
  -I/workspaces/prompp/pp
```

Inputs are explicit `.cpp`, `.cc`, or `.cxx` source files.

## pp Report Target

From the pp workspace root:

```bash
bazel build //:entrypoint_fact_checking
```

This produces:

```text
bazel-bin/entrypoint_fact_checking.json
bazel-bin/entrypoint_fact_checking.log
```

The target is deliberately simple. Its `srcs` are the entrypoint `.cpp` files to
scan. Its `inputs` are local headers that Bazel should make available to the
action sandbox. It does not depend on `//:entrypoint`, and it does not enumerate
the full entrypoint dependency graph.

Because the pp target is currently a report target, diagnostics do not fail the
Bazel action as long as JSON was produced. The diagnostics are available in both
the JSON output and the text log.

## Output Modes

- `--mode=json`: write structured facts and diagnostics to JSON.
- `--mode=lint`: print compiler-style diagnostics to stdout.

Useful options:

- `--output=PATH`: JSON output path. Required when `--mode=json`.
- `--runtime-debug`: append tool-owned runtime diagnostics.
- `--`: treat remaining arguments as libclang parser arguments.

## Exit Codes

For the standalone binary:

- `0`: no error diagnostics were found.
- `1`: invalid invocation, infrastructure failure, or at least one error
  diagnostic was found.

The pp `//:entrypoint_fact_checking` target wraps the standalone binary and
keeps diagnostics report-only for now.
