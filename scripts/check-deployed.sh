#!/usr/bin/env bash
# Verify the deployed instance is running the revision in VERSION.
#
# Reaches the service through a port-forward rather than a public address. The
# panel used to be published through an ingress with a DNS record and a
# certificate, which meant a human interface with one password and no second
# factor was answering the internet so that a handful of administrative actions
# a year would be convenient. The interface is still there - provider accounts
# can only be created through it - it simply is not reachable from outside the
# cluster any more, and neither is this check.
#
# Reads from gopnik.local.env, which is machine-local and gitignored:
#
#   LOOMWATCH_NAMESPACE      namespace of the deployment      (default observability)
#   LOOMWATCH_SERVICE        service name                     (default onwatch)
#   LOOMWATCH_PORT           service port                     (default 9211)
#   LOOMWATCH_LOCAL_PORT     local port for the forward       (default 19211)
#   LOOMWATCH_ADMIN_USER     panel login, for the UI check
#   LOOMWATCH_ADMIN_PASS     panel password, for the UI check
#   ONWATCH_METRICS_TOKEN    bearer token for /metrics
#
# The dashboard check is skipped unless a Grafana password is configured, so a
# deployment that does not run Grafana is not failed for not running it:
#
#   LOOMWATCH_GRAFANA_SERVICE     grafana service name      (default grafana-service)
#   LOOMWATCH_GRAFANA_PORT        grafana service port      (default 3000)
#   LOOMWATCH_GRAFANA_LOCAL_PORT  local port for the forward (default 13000)
#   LOOMWATCH_GRAFANA_USER        grafana login             (default admin)
#   LOOMWATCH_GRAFANA_PASS        grafana password
#   LOOMWATCH_GRAFANA_UID         dashboard uid             (default: the chart\'s own)
set -euo pipefail

set -a
. ./gopnik.local.env
set +a

ns="${LOOMWATCH_NAMESPACE:-observability}"
svc="${LOOMWATCH_SERVICE:-onwatch}"
port="${LOOMWATCH_PORT:-9211}"
local_port="${LOOMWATCH_LOCAL_PORT:-19211}"
want="$(cat VERSION)"

kubectl port-forward -n "$ns" "svc/$svc" "$local_port:$port" >/dev/null 2>&1 &
pf=$!
trap 'kill "$pf" 2>/dev/null || true' EXIT

# Wait for the forward rather than sleeping a guessed number of seconds: a fixed
# sleep is either slower than it needs to be or occasionally too short, and the
# too-short case fails as though the deployment were broken.
for _ in $(seq 1 40); do
  curl -fsS -o /dev/null "http://127.0.0.1:$local_port/login" 2>/dev/null && break
  sleep 1
done

export LOOMWATCH_STAND_URL="http://127.0.0.1:$local_port"

echo "--- the login page reports $want"
curl -fsS "$LOOMWATCH_STAND_URL/login" | grep -qF "$want"

echo "--- onwatch_build_info reports $want"
curl -fsS -H "Authorization: Bearer $ONWATCH_METRICS_TOKEN" "$LOOMWATCH_STAND_URL/metrics" \
  | grep "^onwatch_build_info" | grep -qF "$want"

echo "--- the panel renders, and refuses a wrong password"
tests/e2e/.venv/bin/python scripts/check-deployed-ui.py

# The dashboard, as Grafana draws it.
#
# Everything above this line inspects numbers: the login page, the metrics
# endpoint, the panel served by loomwatch itself. The quota dashboard is drawn
# by a different program from data this one exports, and the defects that live
# there survive every check that stops at the query. Three of them shipped on
# 24 August with promtool, helm unittest and live PromQL all green - reset times
# rendered in 1970, rows missing a name, and the safe state painted in alarm
# red. A browser is the only instrument that sees a formatter.
if [ -n "${LOOMWATCH_GRAFANA_PASS:-}" ]; then
  gsvc="${LOOMWATCH_GRAFANA_SERVICE:-grafana-service}"
  gport="${LOOMWATCH_GRAFANA_PORT:-3000}"
  glocal="${LOOMWATCH_GRAFANA_LOCAL_PORT:-13000}"

  kubectl port-forward -n "$ns" "svc/$gsvc" "$glocal:$gport" >/dev/null 2>&1 &
  gpf=$!
  trap 'kill "$pf" "$gpf" 2>/dev/null || true' EXIT

  for _ in $(seq 1 40); do
    curl -fsS -o /dev/null "http://127.0.0.1:$glocal/api/health" 2>/dev/null && break
    sleep 1
  done

  echo "--- the quota dashboard renders, and renders correctly"
  export LOOMWATCH_GRAFANA_URL="http://127.0.0.1:$glocal"
  tests/e2e/.venv/bin/python scripts/check-deployed-grafana.py
else
  echo "--- the quota dashboard: skipped, LOOMWATCH_GRAFANA_PASS is not set"
fi

echo "deployed instance is on $want"
