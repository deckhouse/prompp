def _prefixed_args(flag, values):
    return [flag + value for value in values]

def _path_args(flag, paths):
    return _prefixed_args(flag, [path for path in paths])

def _compilation_args(ctx):
    target = ctx.attr.target
    compilation_context = target[CcInfo].compilation_context
    args = []
    args.extend(_prefixed_args("-D", compilation_context.defines.to_list()))
    args.extend(_path_args("-I", compilation_context.includes.to_list()))
    args.extend(_path_args("-iquote", compilation_context.quote_includes.to_list()))
    args.extend(_path_args("-isystem", compilation_context.system_includes.to_list()))
    args.append("-iquote.")
    args.append("-iquote" + ctx.bin_dir.path)
    return args

def _entrypoint_fact_checking_impl(ctx):
    output = ctx.actions.declare_file(ctx.label.name + ".json")
    log = ctx.actions.declare_file(ctx.label.name + ".log")

    compilation_context = ctx.attr.target[CcInfo].compilation_context
    clang_args = list(ctx.attr.clang_args)
    clang_args.extend(_compilation_args(ctx))

    json_args = ctx.actions.args()
    json_args.add("--mode=json")
    json_args.add("--output=" + output.path)
    json_args.add_all(ctx.files.srcs)
    json_args.add("--")
    json_args.add_all(clang_args)

    lint_args = ctx.actions.args()
    lint_args.add("--mode=lint")
    lint_args.add_all(ctx.files.srcs)
    lint_args.add("--")
    lint_args.add_all(clang_args)

    inputs = depset(
        direct = ctx.files.srcs,
        transitive = [
            compilation_context.headers,
            ctx.attr.target[DefaultInfo].files,
        ],
    )

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
        "srcs": attr.label_list(
            allow_files = [".cc", ".cpp", ".cxx"],
            mandatory = True,
        ),
        "target": attr.label(
            mandatory = True,
            providers = [CcInfo],
        ),
        "tool": attr.label(
            default = Label("@entrypoint_codegen//:entrypoint_codegen"),
            executable = True,
            cfg = "exec",
        ),
    },
)
