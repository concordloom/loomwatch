#!/usr/bin/env python3
"""Syntax-check every PromQL query in the shipped Grafana dashboard.

The dashboard's queries have the same problem the alerting rules had: nothing
ever parsed them. A typo in a panel expression produces an empty panel, and an
empty panel is indistinguishable from a quiet system.

Grafana template variables are not PromQL, so each `$var` is replaced before
parsing. That checks the shape of the query, which is what breaks; it cannot
check what the variable will expand to.

Two kinds of substitution, because one does not fit both. Grafana's duration
macros - $__range and friends - sit inside a range selector, where a label
matcher is a syntax error; they become a literal duration. Everything else is a
label value and becomes a permissive matcher.

Usage: check-dashboard-queries.py <dashboard.json> <promtool>
"""
import json
import re
import subprocess
import sys
import tempfile


# Grafana duration macros. Any valid duration will do: this checks syntax, not
# what the dashboard's time range happens to be when someone opens it.
DURATION_MACROS = ("$__range_s", "$__range_ms", "$__range", "$__rate_interval",
                   "$__interval_ms", "$__interval")


def normalise(expr):
    for macro in DURATION_MACROS:
        expr = expr.replace(macro, "5m")
    return re.sub(r"\$\w+", ".+", expr)


def main(dashboard, promtool):
    doc = json.load(open(dashboard))
    queries = []
    for panel in doc.get("panels", []):
        for target in panel.get("targets", []):
            expr = target.get("expr")
            if expr:
                queries.append((panel["title"], expr))

    if not queries:
        print("    BROKEN no panel queries found - this check verified nothing")
        return 1

    # One throwaway rule per query: promtool parses rule expressions, and that
    # is the only PromQL parser available without a running server.
    rules = {"groups": [{"name": "dashboard", "rules": []}]}
    for i, (title, expr) in enumerate(queries):
        rules["groups"][0]["rules"].append(
            {"record": f"dashboard:q{i}", "expr": normalise(expr)}
        )

    with tempfile.NamedTemporaryFile("w", suffix=".yaml", delete=False) as fh:
        json.dump(rules, fh)
        path = fh.name

    result = subprocess.run(
        [promtool, "check", "rules", path], capture_output=True, text=True
    )
    print(f"    {len(queries)} dashboard queries parsed")
    if result.returncode != 0:
        print(result.stdout.strip())
        print(result.stderr.strip())
        for title, expr in queries:
            print(f"    panel {title!r}: {expr[:100]}")
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1], sys.argv[2]))
