#!/usr/bin/env bash
# Render the chart's PrometheusRule and check it with promtool.
#
# helm lint and helm unittest compare rendered text; neither of them knows what
# PromQL means. A rule with a broken expression passes both and then fails
# silently in Prometheus, where a rule that cannot be evaluated does not fire
# and nothing says so. This is the only check in the repository that runs the
# rules through a real engine.
set -euo pipefail

CHART_DIR="${CHART_DIR:-charts/loomwatch}"
TEST_DIR="$CHART_DIR/tests/promtool"
PROMTOOL="${PROMTOOL:-promtool}"

if ! command -v "$PROMTOOL" >/dev/null 2>&1; then
  echo "promtool not found; set PROMTOOL to its path" >&2
  exit 2
fi

work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# The values used here must match the fixtures in tests/promtool/*.yaml.
helm template rules-check "$CHART_DIR" \
  --set metrics.prometheusRule.enabled=true \
  --set metrics.prometheusRule.teams[0].provider=zai \
  --set metrics.prometheusRule.teams[0].accountId=1 \
  --set metrics.prometheusRule.teams[0].team=platform \
  --set metrics.prometheusRule.teams[1].provider=anthropic \
  --set metrics.prometheusRule.teams[1].accountId=default \
  --set metrics.prometheusRule.teams[1].team=research \
  -s templates/prometheusrule.yaml > "$work/rendered.yaml"

# Strip the Kubernetes wrapper: promtool wants a bare `groups:` document.
python3 - "$work/rendered.yaml" "$work/rules.yaml" <<'PY'
import sys, yaml
doc = yaml.safe_load(open(sys.argv[1]))
yaml.safe_dump({"groups": doc["spec"]["groups"]}, open(sys.argv[2], "w"),
               default_flow_style=False, sort_keys=False, width=10000)
PY

echo "--- promtool check rules"
"$PROMTOOL" check rules "$work/rules.yaml"

echo "--- promtool test rules"
for t in "$TEST_DIR"/*_test.yaml; do
  [ -e "$t" ] || continue
  cp "$t" "$work/$(basename "$t")"
  ( cd "$work" && "$PROMTOOL" test rules "$(basename "$t")" )
done
