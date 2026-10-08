#!/usr/bin/env bash
set -euo pipefail

sdk_go_version="$(go env GOVERSION)"
echo "Build toolchain: ${sdk_go_version}"
if [[ ! "${sdk_go_version}" =~ ^go([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
  echo "A patched, stable Go release is required" >&2
  exit 1
fi
sdk_go_major="${BASH_REMATCH[1]}"
sdk_go_minor="${BASH_REMATCH[2]}"
sdk_go_patch="${BASH_REMATCH[3]}"
if (( sdk_go_major != 1 || sdk_go_minor < 25 || (sdk_go_minor == 25 && sdk_go_patch < 13) || (sdk_go_minor == 26 && sdk_go_patch < 6) )); then
  echo "Go 1.25.13+, 1.26.6+, or a newer stable release is required" >&2
  exit 1
fi
