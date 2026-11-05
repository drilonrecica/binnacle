#!/bin/bash
set -euo pipefail

# End-to-end check for SEC-3: claim the administrator and perform a CSRF-guarded
# mutation through a TLS-terminating reverse proxy (Caddy), once with
# BINNACLE_PUBLIC_URL and once with BINNACLE_TRUSTED_PROXY_CIDRS. Both runs must
# answer 2xx, set Secure cookies and send HSTS.
#
# Uses `docker compose` or `podman compose`. Set BINNACLE_IMAGE to reuse a
# built image; otherwise the Dockerfile is built as localhost/binnacle:e2e.

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
STACK_DIR="$ROOT_DIR/scripts/e2e-tls-proxy"
BASE_URL="https://localhost:8443"

if docker compose version >/dev/null 2>&1 && docker info >/dev/null 2>&1; then
  RUNTIME=docker
elif command -v podman >/dev/null 2>&1 && podman compose version >/dev/null 2>&1; then
  RUNTIME=podman
else
  echo "docker compose / podman compose not available; skipping TLS proxy e2e."
  exit 0
fi
compose() { "$RUNTIME" compose -f "$STACK_DIR/compose.yml" "$@"; }

export BINNACLE_IMAGE="${BINNACLE_IMAGE:-localhost/binnacle:e2e}"
if ! "$RUNTIME" image inspect "$BINNACLE_IMAGE" >/dev/null 2>&1; then
  "$RUNTIME" build -f "$ROOT_DIR/packaging/docker/Dockerfile" -t "$BINNACLE_IMAGE" "$ROOT_DIR"
fi

WORK="$(mktemp -d)"
cleanup() {
  compose down -v >/dev/null 2>&1 || true
  rm -rf "$WORK"
}
trap cleanup EXIT

fail() { echo "FAIL: $*" >&2; exit 1; }

# run_scenario <label> <public_url> <trusted_cidrs>
run_scenario() {
  local label="$1" jar="$WORK/$1.jar" headers="$WORK/$1.headers" csrf status
  export BINNACLE_SETUP_TOKEN="$(openssl rand -hex 32)"
  export BINNACLE_PUBLIC_URL="$2" BINNACLE_TRUSTED_PROXY_CIDRS="$3"
  compose down -v >/dev/null 2>&1 || true
  compose up -d --quiet-pull

  for _ in $(seq 1 60); do
    if curl -ksf -o /dev/null "$BASE_URL/healthz"; then break; fi
    sleep 1
  done
  curl -ksf -o /dev/null "$BASE_URL/healthz" || { compose logs; fail "$label: stack did not become reachable"; }

  status="$(curl -ks -o /dev/null -w '%{http_code}' -c "$jar" -D "$headers" \
    -H 'Content-Type: application/json' -H "Origin: $BASE_URL" \
    -d "{\"token\":\"$BINNACLE_SETUP_TOKEN\",\"username\":\"admin\",\"password\":\"correct horse battery staple\"}" \
    "$BASE_URL/api/v1/setup/claim")"
  [ "$status" = "204" ] || { compose logs binnacle; fail "$label: setup claim answered $status"; }
  grep -qi '^strict-transport-security:' "$headers" || fail "$label: HSTS missing on the claim response"
  grep -i '^set-cookie:' "$headers" | grep -qi 'binnacle_session=' || fail "$label: no session cookie"
  if grep -i '^set-cookie:' "$headers" | grep -vqi '; *secure'; then
    fail "$label: a cookie was set without the Secure attribute"
  fi

  csrf="$(awk '$6 == "binnacle_csrf" { print $7 }' "$jar")"
  [ -n "$csrf" ] || fail "$label: CSRF cookie missing from the jar"
  status="$(curl -ks -o "$WORK/$label.put" -w '%{http_code}' -b "$jar" \
    -H 'Content-Type: application/json' -H "Origin: $BASE_URL" -H "X-CSRF-Token: $csrf" \
    -X PUT -d '{"theme":"dark","density":"comfortable","pinnedResources":[],"landingPage":"overview","chartRange":"1h"}' \
    "$BASE_URL/api/v1/preferences")"
  [ "$status" = "200" ] || { cat "$WORK/$label.put"; fail "$label: mutation answered $status"; }
  echo "ok: $label — claim, Secure cookies, HSTS and a CSRF-guarded mutation through the TLS proxy"
}

run_scenario public-url "$BASE_URL" ""
run_scenario trusted-cidr "" "10.77.0.0/24"
echo "TLS proxy e2e passed."
