#!/bin/bash
set -euo pipefail

TAG="${1:-${GITHUB_REF_NAME:-}}"
if [[ -z "$TAG" ]]; then
  echo "usage: validate-version.sh <tag>" >&2
  exit 1
fi

if [[ ! "$TAG" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)(-([a-zA-Z0-9.]+))?(\+([a-zA-Z0-9.]+))?$ ]]; then
  echo "Invalid semantic version tag: $TAG" >&2
  exit 1
fi

MAJOR="${BASH_REMATCH[1]}"
MINOR="${BASH_REMATCH[2]}"
PATCH="${BASH_REMATCH[3]}"
PRERELEASE="${BASH_REMATCH[5]:-}"

CHANNEL="edge"
if [[ -z "$PRERELEASE" ]]; then
  CHANNEL="stable"
elif [[ "$PRERELEASE" == beta* ]]; then
  CHANNEL="beta"
fi

# stable must not point to alpha or beta builds.
if [[ "$CHANNEL" == "stable" && -n "$PRERELEASE" ]]; then
  echo "stable channel cannot target a prerelease: $TAG" >&2
  exit 1
fi

# beta must not point to alpha builds.
if [[ "$CHANNEL" == "beta" && "$PRERELEASE" == alpha* ]]; then
  echo "beta channel cannot target an alpha prerelease: $TAG" >&2
  exit 1
fi

echo "version=$TAG"
echo "channel=$CHANNEL"
echo "major=$MAJOR"
echo "minor=$MINOR"
echo "patch=$PATCH"
echo "prerelease=$PRERELEASE"
