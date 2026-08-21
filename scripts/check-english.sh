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
      if grep -lIP "$CYRILLIC" "$f" >/dev/null 2>&1; then
        printf 'Cyrillic in %s:\n' "$f" >&2
        grep -nIP "$CYRILLIC" "$f" | head -20 >&2
        status=1
      fi
    done
    [ "$status" -eq 0 ] || { fail; exit 1; }
    ;;
  *)
    hits=$(git ls-files -z | xargs -0 grep -lIP "$CYRILLIC" 2>/dev/null || true)
    if [ -n "$hits" ]; then
      printf 'Cyrillic in tracked files:\n' >&2
      printf '%s\n' "$hits" >&2
      fail
      exit 1
    fi
    ;;
esac

printf 'English-only check passed.\n'
