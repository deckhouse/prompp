"""Builds a static library for several -march values in a single bazel invocation.

Each variant transitions its target to its own //bazel/toolchain:march value, so all flavors get
separate output directories and bazel schedules their actions together instead of one invocation
per flavor.
"""

_MARCH_FLAG = "//bazel/toolchain:march"

def _march_transition_impl(_settings, attr):
    return {_MARCH_FLAG: attr.march}

_march_transition = transition(
    implementation = _march_transition_impl,
    inputs = [],
    outputs = [_MARCH_FLAG],
)

def _march_variant_impl(ctx):
    archives = [f for f in ctx.attr.target[0][DefaultInfo].files.to_list() if f.extension == "a"]
    if len(archives) != 1:
        fail("{} must provide exactly one .a file, got: {}".format(ctx.attr.target[0].label, archives))

    output = ctx.actions.declare_file("{}.a".format(ctx.label.name))
    ctx.actions.symlink(output = output, target_file = archives[0])
    return [DefaultInfo(files = depset([output]))]

march_variant = rule(
    implementation = _march_variant_impl,
    attrs = {
        "march": attr.string(mandatory = True),
        "target": attr.label(mandatory = True, cfg = _march_transition),
    },
)

def march_escape(march):
    """Replaces every non-alphanumeric character with '_' (same as `escape` in entrypoint/Makefile)."""
    return "".join([c if c.isalnum() else "_" for c in march.elems()])

def march_variants(name, target, marches):
    """Declares `<name>_<escaped march>` march_variant targets producing `<name>_<escaped march>.a`."""
    for march in marches:
        march_variant(
            name = "{}_{}".format(name, march_escape(march)),
            march = march,
            target = target,
        )
