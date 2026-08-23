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

echo "deployed instance is on $want"
