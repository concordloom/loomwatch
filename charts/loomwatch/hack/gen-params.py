#!/usr/bin/env python3
"""Regenerate the parameter tables in README.md from values.yaml.

The tables are derived from the `## @section` / `## @param` annotations in
values.yaml, the same convention Bitnami's readme-generator uses. They are
generated rather than hand-written because there are ~180 of them: kept by
hand, the documented default and the actual default drift apart silently, and
a values table that lies is worse than no table.

The script rewrites everything between the `## Parameters` heading and the
next `## ` heading, leaving the surrounding prose alone.

    python3 charts/loomwatch/hack/gen-params.py

Exits non-zero with --check if the README is out of date, which is what CI
would call.
"""

from __future__ import annotations

import argparse
import pathlib
import re
import sys

import yaml

CHART_DIR = pathlib.Path(__file__).resolve().parent.parent
VALUES = CHART_DIR / "values.yaml"
README = CHART_DIR / "README.md"

SECTION_RE = re.compile(r"^##\s*@section\s+(.*)$")
# The optional [...] is the type hint readme-generator uses; it is not part of
# the description.
PARAM_RE = re.compile(r"^##\s*@param\s+(\S+)\s*(?:\[[^\]]*\])?\s*(.*)$")


def resolve(values: dict, dotted: str):
    """Follow a dotted path, returning None when any segment is missing."""
    cur = values
    for part in dotted.split("."):
        if isinstance(cur, dict) and part in cur:
            cur = cur[part]
        else:
            return None
    return cur


def render(value) -> str:
    if value is None:
        return "`nil`"
    if isinstance(value, bool):
        return f"`{str(value).lower()}`"
    if isinstance(value, str):
        return f"`{value}`" if value else '`""`'
    if isinstance(value, (list, dict)) and not value:
        return "`[]`" if isinstance(value, list) else "`{}`"
    return f"`{value}`"


def build_tables() -> str:
    values = yaml.safe_load(VALUES.read_text(encoding="utf-8"))
    out: list[str] = []
    section = None
    for line in VALUES.read_text(encoding="utf-8").splitlines():
        stripped = line.strip()
        if m := SECTION_RE.match(stripped):
            if section:
                out.append("")
            section = m.group(1).strip()
            out += [f"### {section}", "", "| Name | Description | Value |", "| ---- | ----------- | ----- |"]
            continue
        if (m := PARAM_RE.match(stripped)) and section:
            name, desc = m.group(1), m.group(2).strip()
            out.append(f"| `{name}` | {desc} | {render(resolve(values, name))} |")
    return "\n".join(out)


def splice(readme: str, tables: str) -> str:
    lines = readme.splitlines()
    try:
        start = next(i for i, l in enumerate(lines) if l.strip() == "## Parameters")
    except StopIteration:
        sys.exit("README.md has no '## Parameters' heading to splice into")
    end = next(
        (i for i in range(start + 1, len(lines)) if lines[i].startswith("## ")),
        len(lines),
    )
    return "\n".join(lines[: start + 1] + ["", tables, ""] + lines[end:]) + "\n"


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--check", action="store_true", help="fail if README.md is stale")
    args = ap.parse_args()

    current = README.read_text(encoding="utf-8")
    updated = splice(current, build_tables())

    if args.check:
        if current != updated:
            print("README.md parameter tables are stale; run hack/gen-params.py", file=sys.stderr)
            return 1
        print("README.md parameter tables are up to date")
        return 0

    README.write_text(updated, encoding="utf-8")
    documented = sum(1 for l in updated.splitlines() if l.startswith("| `"))
    print(f"README.md updated ({documented} parameters)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
