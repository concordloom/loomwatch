#!/usr/bin/env bash
# Поднять версию форка во всех местах, которые сверяет сборка релиза.
#
# Мест два — VERSION и charts/loomwatch/Chart.yaml (там ещё и две строки:
# version и appVersion) — и сборка проверяет их по очереди. Обновление руками
# дважды подряд заканчивалось упавшим тегом: первый раз забыли VERSION, второй
# раз чарт. Отсюда скрипт.
#
#   scripts/set-version.sh 1.6.2
#
# Тег после этого выпускается как loom-v<версия>: сборка выводит ожидаемую
# версию именно из его имени.
set -euo pipefail

version="${1:-}"
if [ -z "$version" ]; then
  echo "использование: scripts/set-version.sh <версия>, например 1.6.2" >&2
  exit 2
fi
if ! printf '%s' "$version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
  echo "версия должна быть вида X.Y.Z, получено '$version'" >&2
  exit 2
fi

root="$(cd "$(dirname "$0")/.." && pwd)"
chart="$root/charts/loomwatch/Chart.yaml"

printf '%s\n' "$version" > "$root/VERSION"
sed -i -E "s/^version: .*/version: $version/; s/^appVersion: .*/appVersion: $version/" "$chart"

echo "VERSION      $(cat "$root/VERSION")"
awk '/^version:|^appVersion:/ {print "Chart.yaml   " $0}' "$chart"
