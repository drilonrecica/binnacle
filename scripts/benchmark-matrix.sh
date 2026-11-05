#!/bin/bash
# SPDX-License-Identifier: AGPL-3.0-only
# Short-scenario benchmark matrix for 10/30/50/100 synthetic containers and
# 10/50/100 checks. The release gate uses the long-run scenario instead
# (make benchmark); the matrix is for comparing commits locally.
set -euo pipefail

DURATION="${BENCHMARK_DURATION:-60}"
OUTPUT_DIR="${BENCHMARK_OUTPUT_DIR:-benchmark-reports}"

mkdir -p "$OUTPUT_DIR"
failed=0

for containers in 10 30 50 100; do
  for checks in 10 50 100; do
  echo "==> Running benchmark: $containers containers, $checks checks for ${DURATION}s"
  if ! python3 scripts/benchmark.py \
    --scenario short \
    --containers "$containers" \
    --checks "$checks" \
    --duration "$DURATION" \
    --output "$OUTPUT_DIR/containers-${containers}-checks-${checks}.json" >/dev/null; then
    failed=1
  fi
  done
done

echo "==> Reports written to $OUTPUT_DIR"
if [[ "$failed" -ne 0 ]]; then
  echo "==> At least one run missed a target" >&2
  exit 1
fi
