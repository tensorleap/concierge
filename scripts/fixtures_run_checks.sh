#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

cd "${REPO_ROOT}"

bash scripts/fixtures_prepare.sh
bash scripts/fixtures_mutate_cases.sh
bash scripts/fixtures_verify.sh

go test ./internal/e2e/fixtures -v
