#!/usr/bin/env python3
"""Verify every alert carries a runbook_url whose anchor exists.

A runbook_url pointing at an anchor that does not exist is worse than no
runbook at all: it costs the reader a click and their remaining trust in the
link, at the moment they have the least of both.

Usage: check-runbook-links.py <rendered-prometheusrule.yaml> <runbook.md>
"""
import re
import sys

import yaml


def anchors_of(path):
    """GitHub's heading-to-anchor rule, for the subset we generate."""
    found = set()
    for line in open(path):
        m = re.match(r"^#{1,6}\s+(.+?)\s*$", line)
        if m:
            slug = re.sub(r"[^a-z0-9 -]", "", m.group(1).lower()).replace(" ", "-")
            found.add(slug)
    return found


def main(rendered, runbook):
    doc = yaml.safe_load(open(rendered))
    anchors = anchors_of(runbook)
    problems = []
    checked = 0

    for group in doc["spec"]["groups"]:
        for rule in group["rules"]:
            if "alert" not in rule:
                continue
            url = rule.get("annotations", {}).get("runbook_url")
            if not url:
                problems.append(f"{rule['alert']}: no runbook_url")
                continue
            checked += 1
            anchor = url.split("#", 1)[1] if "#" in url else ""
            if anchor not in anchors:
                problems.append(
                    f"{rule['alert']}: #{anchor} is not a heading in {runbook}"
                )

    if not checked and not problems:
        problems.append("no alerts were rendered - this check verified nothing")

    print(f"    {checked} runbook links checked against {len(anchors)} headings")
    for p in problems:
        print(f"    BROKEN {p}")
    return 1 if problems else 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1], sys.argv[2]))
