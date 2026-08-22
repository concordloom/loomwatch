#!/usr/bin/env bash
# Refuse Cyrillic anywhere that lands in git.
#
# loomWatch is an open-source product. A contributor who does not read Russian
# has to be able to follow the history and the code, so commit messages, code
# comments, docs and UI strings are English. The rule was written down in
# CLAUDE.md first and was still broken 34 commits in a row, which is why it is
# also checked mechanically here.
#
#   scripts/check-english.sh              # every tracked text file
#   scripts/check-english.sh --files a b  # only these paths
#   scripts/check-english.sh --message F  # a commit message file (used by the hook)
#
# Only Cyrillic is rejected, not non-ASCII in general: box drawing, arrows and
# typographic dashes are legitimate and already in the codebase.
set -euo pipefail

CYRILLIC='[\x{0400}-\x{04FF}]'
status=0

# Vendored agent skills are exempt. `.claude/skills/` holds third-party tooling
# that its own installer writes and overwrites on every update - the Gopnik
# bundle ships an English SKILL.md beside a Russian translation of the same
# document. The rule exists so that a contributor who does not read Russian can
# follow THIS repository's history and code; a vendored tool's translated copy
# does not stand in the way of that, and editing files an update replaces would
# only break the check again on the next one. Nothing this project writes is
# covered by the exemption.
EXEMPT='.claude/skills/'

exempt() {
  case "${1#./}" in
    "$EXEMPT"*) return 0 ;;
    *) return 1 ;;
  esac
}

fail() {
  printf '\n\033[0;31mEnglish-only check failed.\033[0m\n' >&2
  printf 'Everything committed to this repository is written in English.\n' >&2
  printf 'See the Style section of CLAUDE.md.\n' >&2
}

case "${1:-}" in
  --message)
    file="${2:?--message needs a file}"
    if grep -nP "$CYRILLIC" "$file" >/dev/null 2>&1; then
      printf 'Cyrillic in the commit message:\n' >&2
      grep -nP "$CYRILLIC" "$file" >&2
      fail
      exit 1
    fi
    ;;
  --files)
    shift
    for f in "$@"; do
      [ -f "$f" ] || continue
      exempt "$f" && continue
      if grep -lIP "$CYRILLIC" "$f" >/dev/null 2>&1; then
        printf 'Cyrillic in %s:\n' "$f" >&2
        grep -nIP "$CYRILLIC" "$f" | head -20 >&2
        status=1
      fi
    done
    [ "$status" -eq 0 ] || { fail; exit 1; }
    ;;
  *)
    hits=$(git ls-files -z -- ":(exclude)$EXEMPT" | xargs -0 grep -lIP "$CYRILLIC" 2>/dev/null || true)
    if [ -n "$hits" ]; then
      printf 'Cyrillic in tracked files:\n' >&2
      printf '%s\n' "$hits" >&2
      fail
      exit 1
    fi
    ;;
esac

printf 'English-only check passed.\n'
