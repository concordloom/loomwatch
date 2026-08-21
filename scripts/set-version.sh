#!/usr/bin/env bash
# Bump the fork version in every place the release build cross-checks.
#
# There are two places - VERSION and charts/loomwatch/Chart.yaml (which itself
# holds two lines: version and appVersion) - and the build checks them in turn.
# Updating by hand ended in a failed tag twice in a row: the first time VERSION
# was forgotten, the second time the chart. Hence this script.
#
#   scripts/set-version.sh 1.6.2
#
# The tag is then released as loom-v<version>: the build derives the expected
# version from that name.
set -euo pipefail

version="${1:-}"
if [ -z "$version" ]; then
  echo "usage: scripts/set-version.sh <version>, for example 1.6.2" >&2
  exit 2
fi
if ! printf '%s' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "version must look like X.Y.Z, got '$version'" >&2
  exit 2
fi

root="$(cd "$(dirname "$0")/.." && pwd)"
chart="$root/charts/loomwatch/Chart.yaml"

printf '%s\n' "$version" > "$root/VERSION"
sed -i -E "s/^version: .*/version: $version/; s/^appVersion: .*/appVersion: $version/" "$chart"

echo "VERSION      $(cat "$root/VERSION")"
awk '/^version:|^appVersion:/ {print "Chart.yaml   " $0}' "$chart"
