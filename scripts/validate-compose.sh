#!/bin/bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
COMPOSE_FILE="$ROOT_DIR/packaging/docker/docker-compose.yml"

if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
  DOCKER_GID="${DOCKER_GID:-$(stat -c '%g' /var/run/docker.sock 2>/dev/null || id -g)}"
  export DOCKER_GID
  BINNACLE_SETUP_TOKEN=dummy docker compose -f "$COMPOSE_FILE" config | grep -F 'image: ghcr.io/drilonrecica/binnacle:stable' >/dev/null
  echo "Compose file is valid."
else
  echo "docker compose not available; skipping live validation."
fi
