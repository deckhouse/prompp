def entrypoint_codegen_copts():
    include_root = native.package_name()
    if include_root == "":
        include_root = "."
    return [
        "-std=c++2b",
        "-I" + include_root,
    ]
