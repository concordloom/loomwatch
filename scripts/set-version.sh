#!/usr/bin/env bash
# Bump the fork version in every place the release build cross-checks.
#
# There are three places now - VERSION, charts/loomwatch/Chart.yaml (which
# itself holds two lines: version and appVersion) and the artifacthub.io/images
# annotation in the same file - and the build checks each of them in turn.
# Updating by hand ended in a failed tag twice in a row: the first time VERSION
# was forgotten, the second time the chart. The annotation is the third, and it
# is the easiest to forget because nothing renders it: it drifted eight minor
# releases before a check was written for it. Hence this script.
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

# The image inside artifacthub.io/images. Anchored to the leading two spaces of
# the annotation's folded block so it cannot match the chart's own image values.
sed -i -E "s|^      image: ghcr\\.io/madduck-tech/loomwatch:.*|      image: ghcr.io/madduck-tech/loomwatch:$version|" "$chart"

echo "VERSION      $(cat "$root/VERSION")"
awk '/^version:|^appVersion:/ {print "Chart.yaml   " $0}' "$chart"
awk '/image: ghcr\.io\/madduck-tech\/loomwatch:/ {gsub(/^ +/, ""); print "annotation   " $0}' "$chart"
