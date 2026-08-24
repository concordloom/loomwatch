#!/usr/bin/env python3
"""Every relative link and image in the documentation points at a tracked file.

Written after the README shipped to the default branch with two broken images.

The cause is worth stating exactly, because the obvious guard would not have
caught it. `.gitignore` carries a blanket `*.png` with a short list of
exceptions, one of which is `docs/screenshots/`. The images were written to
`docs/img/`, `git add -A` skipped them without a word, and the commit went
through with the README referencing files that existed on the author's disk and
nowhere else.

A link checker that asks "does this path exist" passes in exactly that
situation, because the path does exist - locally. The question that had to be
asked is whether the file is IN THE REPOSITORY, which is `git ls-files`.

Anchors are checked too, against the headings of the target file, because a
renamed section leaves a link that is not broken enough to notice.
"""
import re
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

# Markdown link and image targets: [text](target) and ![alt](target).
LINK = re.compile(r"!?\[[^\]]*\]\(([^)\s]+)(?:\s+\"[^\"]*\")?\)")

# GitHub's anchor rule, enough of it: lower case, drop anything that is not a
# word character, a space or a hyphen, then spaces become hyphens.
def anchor_of(heading: str) -> str:
    text = heading.strip().lstrip("#").strip()
    text = re.sub(r"`([^`]*)`", r"\1", text)
    text = re.sub(r"[^\w\s-]", "", text, flags=re.UNICODE).strip().lower()
    return re.sub(r"[\s]+", "-", text)


def tracked_files() -> set:
    out = subprocess.run(
        ["git", "ls-files", "-z"], cwd=ROOT, capture_output=True, text=True, check=True
    ).stdout
    return {p for p in out.split("\0") if p}


def main() -> int:
    tracked = tracked_files()
    docs = sorted(p for p in tracked if p.endswith(".md"))
    anchors: dict = {}
    failures = []
    checked = 0

    for doc in docs:
        body = (ROOT / doc).read_text(encoding="utf-8")
        for match in LINK.finditer(body):
            target = match.group(1)
            if target.startswith(("http://", "https://", "mailto:", "#")):
                continue
            checked += 1
            path, _, anchor = target.partition("#")
            if not path:
                continue
            resolved = str((Path(doc).parent / path).resolve().relative_to(ROOT)) \
                if not path.startswith("/") else path.lstrip("/")

            if resolved not in tracked:
                on_disk = (ROOT / resolved).exists()
                why = (
                    "exists on disk but is NOT tracked - check .gitignore, "
                    "a blanket *.png with exceptions is why this file exists"
                    if on_disk else "does not exist"
                )
                failures.append(f"{doc} -> {target}: {why}")
                continue

            if anchor and resolved.endswith(".md"):
                if resolved not in anchors:
                    anchors[resolved] = {
                        anchor_of(line)
                        for line in (ROOT / resolved).read_text(encoding="utf-8").splitlines()
                        if line.startswith("#")
                    }
                if anchor.lower() not in anchors[resolved]:
                    failures.append(f"{doc} -> {target}: no heading makes that anchor")

    if not checked:
        print("no relative links found at all; this check proved nothing", file=sys.stderr)
        return 1

    if failures:
        print(f"documentation links FAILED ({len(failures)}):", file=sys.stderr)
        for failure in failures:
            print(f"  - {failure}", file=sys.stderr)
        return 1

    print(f"{checked} relative links across {len(docs)} documents resolve to tracked files")
    return 0


if __name__ == "__main__":
    sys.exit(main())
