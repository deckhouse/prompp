#!/usr/bin/env python3
"""Normalize association-table types after adapting gperf output for constexpr use."""

import argparse
from pathlib import Path
import re


def normalize(source):
    pattern = re.compile(r"\b(?:unsigned\s+short|uint16_t)(\s+asso_values\s*\[\s*\]\s*=\s*\{)([\d\s,]+)(\};)")
    matches = list(pattern.finditer(source))
    if len(matches) != 1:
        raise ValueError("expected exactly one numeric association table")
    values = [int(value) for value in re.findall(r"\d+", matches[0][2])]
    if not values or max(values) > 65535:
        raise ValueError("association values do not fit uint16_t")
    result = pattern.sub(lambda match: "uint16_t" + "".join(match.groups()), source)
    if "#include <cstdint>" not in result:
        first_include = re.search(r"^#include[^\n]*\n", result, re.MULTILINE)
        if first_include:
            result = result[:first_include.end()] + "#include <cstdint>\n" + result[first_include.end():]
        else:
            result = "#include <cstdint>\n\n" + result
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("header", nargs="?", type=Path,
                        default=Path(__file__).resolve().parents[1] / "prometheus/promql/function_names_hash.h")
    parser.add_argument("--check", action="store_true", help="Check normalization without writing the header.")
    args = parser.parse_args()
    source = args.header.read_text()
    try:
        result = normalize(source)
    except ValueError as error:
        parser.error(str(error))
    if args.check:
        if result != source:
            parser.exit(1, "association table needs normalization\n")
    else:
        args.header.write_text(result)


if __name__ == "__main__":
    main()
