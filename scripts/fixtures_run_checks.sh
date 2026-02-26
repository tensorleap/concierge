#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

cd "${REPO_ROOT}"

export CONCIERGE_RUN_FIXTURE_E2E=1

bash scripts/fixtures_prepare.sh

go test ./internal/e2e/fixtures -v
