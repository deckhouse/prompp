def _path_args(flag, paths):
    return [flag + path for path in paths]

def _entrypoint_fact_checking_impl(ctx):
    output = ctx.actions.declare_file(ctx.label.name + ".json")
    log = ctx.actions.declare_file(ctx.label.name + ".log")

    clang_args = []
    clang_args.extend(_path_args("-I", ctx.attr.includes))
    clang_args.extend(_path_args("-iquote", ctx.attr.quote_includes))
    clang_args.extend(_path_args("-isystem", ctx.attr.system_includes))

    json_args = ctx.actions.args()
    json_args.add("--mode=json")
    json_args.add("--output=" + output.path)
    json_args.add_all(ctx.files.srcs)
    json_args.add("--")
    json_args.add_all(ctx.attr.clang_args)
    json_args.add_all(clang_args)

    lint_args = ctx.actions.args()
    lint_args.add("--mode=lint")
    lint_args.add_all(ctx.files.srcs)
    lint_args.add("--")
    lint_args.add_all(ctx.attr.clang_args)
    lint_args.add_all(clang_args)

    inputs = depset(ctx.files.srcs + ctx.files.inputs)

    ctx.actions.run_shell(
        inputs = inputs,
        tools = [ctx.executable.tool],
        outputs = [
            output,
            log,
        ],
        arguments = [
            ctx.executable.tool.path,
            log.path,
            output.path,
            json_args,
            "--entrypoint_fact_checking_lint_args--",
            lint_args,
        ],
        command = """
tool="$1"
log="$2"
output="$3"
shift 3

json_args=()
while [ "$#" -gt 0 ] && [ "$1" != "--entrypoint_fact_checking_lint_args--" ]; do
  json_args+=("$1")
  shift
done

if [ "$#" -gt 0 ]; then
  shift
fi

lint_args=("$@")

set +e
"${tool}" "${json_args[@]}" >"${log}" 2>&1
status=$?

{
  echo
  echo "entrypoint_fact_checking diagnostics:"
} >>"${log}"
"${tool}" "${lint_args[@]}" >>"${log}" 2>&1
lint_status=$?

if [ "${status}" -eq 0 ]; then
  status="${lint_status}"
fi

if [ -s "${log}" ]; then
  sed 's/^/entrypoint_fact_checking: /' "${log}" >&2
fi

if [ ! -s "${output}" ]; then
  echo "ERROR: entrypoint_fact_checking did not produce ${output}; see ${log}" >&2
  if [ "${status}" -ne 0 ]; then
    exit "${status}"
  fi
  exit 1
fi

if [ "${status}" -ne 0 ]; then
  echo "WARNING: entrypoint_fact_checking reported diagnostics; see ${output} and ${log}" >&2
fi
exit 0
""",
        mnemonic = "EntrypointFactChecking",
        progress_message = "Checking entrypoint facts for %{label}",
    )

    return [
        DefaultInfo(files = depset([
            output,
            log,
        ])),
    ]

entrypoint_fact_checking = rule(
    implementation = _entrypoint_fact_checking_impl,
    attrs = {
        "clang_args": attr.string_list(
            default = ["-std=c++2b"],
        ),
        "includes": attr.string_list(
            default = ["."],
        ),
        "inputs": attr.label_list(
            allow_files = True,
        ),
        "quote_includes": attr.string_list(),
        "system_includes": attr.string_list(),
        "srcs": attr.label_list(
            allow_files = [".cc", ".cpp", ".cxx"],
            mandatory = True,
        ),
        "tool": attr.label(
            default = Label("@entrypoint_codegen//:entrypoint_codegen"),
            executable = True,
            cfg = "exec",
        ),
    },
)
