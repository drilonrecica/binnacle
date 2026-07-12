#!/bin/bash
# SPDX-License-Identifier: AGPL-3.0-only
# Reproducible benchmark matrix for 10/30/50/100 synthetic containers.
set -euo pipefail

DURATION="${BENCHMARK_DURATION:-60}"
OUTPUT_DIR="${BENCHMARK_OUTPUT_DIR:-benchmark-reports}"

mkdir -p "$OUTPUT_DIR"

for containers in 10 30 50 100; do
  for checks in 10 50 100; do
  echo "==> Running benchmark: $containers containers for ${DURATION}s"
  python3 scripts/benchmark.py \
    --containers "$containers" \
    --checks "$checks" \
    --duration "$DURATION" \
    --output "$OUTPUT_DIR/containers-${containers}-checks-${checks}.json"
  done
done

echo "==> Reports written to $OUTPUT_DIR"
