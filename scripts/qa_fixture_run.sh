#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
MANIFEST_PATH="${REPO_ROOT}/fixtures/manifest.json"
FIXTURES_ROOT="${REPO_ROOT}/.fixtures"

usage() {
  cat <<'EOF'
Usage: bash scripts/qa_fixture_run.sh [--repo <fixture-id>] [-- <qa_loop args...>]

Reset a prepared built-in fixture to a clean pinned state and run the QA loop.

Options:
  --repo <fixture-id>   Fixture ID from fixtures/manifest.json. Default: ultralytics
  --help                Show this help text.
EOF
}

fail() {
  echo "error: $*" >&2
  exit 1
}

log() {
  echo "[qa-run] $*"
}

fixture_id="${REPO:-ultralytics}"
while (($# > 0)); do
  case "$1" in
    --repo)
      (($# >= 2)) || fail "--repo requires a fixture id"
      fixture_id="$2"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    --)
      shift
      break
      ;;
    *)
      break
      ;;
  esac
done

qa_args=("$@")

[[ -f "${MANIFEST_PATH}" ]] || fail "fixture manifest not found: ${MANIFEST_PATH}"
command -v jq >/dev/null 2>&1 || fail "required command 'jq' not found"
command -v python3 >/dev/null 2>&1 || fail "required command 'python3' not found"

jq -e --arg id "${fixture_id}" '.fixtures[] | select(.id == $id)' "${MANIFEST_PATH}" >/dev/null \
  || fail "unknown fixture id '${fixture_id}' (see ${MANIFEST_PATH})"

fixture_root="${FIXTURES_ROOT}/${fixture_id}"
pre_dir="${fixture_root}/pre"
post_dir="${fixture_root}/post"

if [[ ! -x "${pre_dir}/.fixture_reset.sh" || ! -x "${post_dir}/.fixture_reset.sh" ]]; then
  log "Preparing fixtures because '${fixture_id}' is not available locally yet"
  bash "${REPO_ROOT}/scripts/fixtures_prepare.sh"
fi

[[ -x "${pre_dir}/.fixture_reset.sh" ]] || fail "missing pre reset script for fixture '${fixture_id}': ${pre_dir}/.fixture_reset.sh"
[[ -x "${post_dir}/.fixture_reset.sh" ]] || fail "missing post reset script for fixture '${fixture_id}': ${post_dir}/.fixture_reset.sh"

log "Resetting fixture '${fixture_id}' post variant"
"${post_dir}/.fixture_reset.sh"
log "Resetting fixture '${fixture_id}' pre variant"
"${pre_dir}/.fixture_reset.sh"

if [[ -n "$(git -C "${pre_dir}" status --porcelain)" ]]; then
  fail "fixture pre variant is not clean after reset: ${pre_dir}"
fi
if [[ -n "$(git -C "${post_dir}" status --porcelain)" ]]; then
  fail "fixture post variant is not clean after reset: ${post_dir}"
fi

log "Starting QA loop for fixture '${fixture_id}'"
log "Pre fixture: ${pre_dir}"
log "Post fixture: ${post_dir}"

exec python3 "${REPO_ROOT}/QA/qa_loop.py" \
  --command-cwd "${pre_dir}" \
  --fixture-post-path "${post_dir}" \
  "${qa_args[@]}"
