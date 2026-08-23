#!/usr/bin/env python3
"""Syntax-check every PromQL query in the shipped Grafana dashboard.

The dashboard's queries have the same problem the alerting rules had: nothing
ever parsed them. A typo in a panel expression produces an empty panel, and an
empty panel is indistinguishable from a quiet system.

Grafana template variables are not PromQL, so each `$var` is replaced with a
permissive matcher before parsing. That checks the shape of the query, which is
what breaks; it cannot check what the variable will expand to.

Usage: check-dashboard-queries.py <dashboard.json> <promtool>
"""
import json
import re
import subprocess
import sys
import tempfile


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
            {"record": f"dashboard:q{i}", "expr": re.sub(r"\$\w+", ".+", expr)}
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
