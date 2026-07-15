#!/bin/bash
set -euo pipefail

IMAGE="ghcr.io/wollomatic/socket-proxy:1.12.3@sha256:9e781fbe79315355d08901832f639119aa332ac27ee6157fc7f2fab5193c8600"

if ! command -v docker >/dev/null 2>&1 || ! docker info >/dev/null 2>&1; then
  echo "Docker is unavailable; skipping socket proxy smoke test."
  exit 0
fi
if ! command -v curl >/dev/null 2>&1; then
  echo "curl is unavailable; skipping socket proxy smoke test."
  exit 0
fi

directory="$(mktemp -d)"
container="binnacle-socket-proxy-smoke-$$"
docker_gid="$(stat -c '%g' /var/run/docker.sock)"
# Match the named-volume access used in production. Rootful Docker preserves
# the host directory owner, and the proxy has no DAC override capability.
chgrp "$docker_gid" "$directory"
chmod 0770 "$directory"
cleanup() {
  docker rm -f "$container" >/dev/null 2>&1 || true
  rm -rf "$directory"
}
trap cleanup EXIT

docker run -d --name "$container" --read-only --cap-drop ALL --security-opt no-new-privileges:true \
  --user "0:$docker_gid" \
  -v /var/run/docker.sock:/var/run/docker.sock:ro \
  -v "$directory:/var/run/binnacle-docker" \
  "$IMAGE" \
  -loglevel=warn \
  -allowHEAD=/_ping \
  '-allowGET=/v1\.[0-9]+/version' \
  '-allowGET=/v1\.[0-9]+/containers/json' \
  '-allowGET=/v1\.[0-9]+/containers/[^/]+/(json|stats|logs)' \
  '-allowGET=/v1\.[0-9]+/events' \
  -proxysocketendpoint=/var/run/binnacle-docker/docker.sock \
  -proxysocketendpointfilemode=0660 >/dev/null

socket="$directory/docker.sock"
for _ in $(seq 1 30); do
  test -S "$socket" && break
  sleep 0.2
done
test -S "$socket"

status() {
  curl --silent --show-error --unix-socket "$socket" --output /dev/null --write-out '%{http_code}' "$@"
}

assert_status() {
  local expected="$1"
  local actual="$2"
  local request="$3"
  if [[ "$actual" != "$expected" ]]; then
    echo "$request returned HTTP $actual; expected $expected." >&2
    exit 1
  fi
}

assert_denied() {
  local actual="$1"
  local request="$2"
  case "$actual" in
    403 | 405) ;;
    *)
      echo "$request was not denied (HTTP $actual)." >&2
      exit 1
      ;;
  esac
}

assert_status 200 "$(status --head http://localhost/_ping)" "HEAD /_ping"
version_status="$(status http://localhost/v1.55/version)"
if [[ "$version_status" == "403" || "$version_status" == "405" ]]; then
  echo "GET /version was denied (HTTP $version_status)." >&2
  exit 1
fi
assert_denied "$(status --request POST http://localhost/v1.55/containers/create)" "POST /containers/create"
assert_status 403 "$(status http://localhost/v1.55/containers/example/archive)" "GET /containers/example/archive"
assert_status 403 "$(status http://localhost/v1.55/images/json)" "GET /images/json"

echo "Socket proxy allowlist smoke test passed."
