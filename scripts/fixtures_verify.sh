#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
MANIFEST_PATH="${REPO_ROOT}/fixtures/manifest.json"
FIXTURES_ROOT="${REPO_ROOT}/.fixtures"

fail() {
  echo "error: $*" >&2
  exit 1
}

log() {
  echo "[fixtures_verify] $*"
}

require_cmd() {
  local cmd="$1"
  command -v "${cmd}" >/dev/null 2>&1 || fail "required command '${cmd}' not found"
}

assert_clean_git_tree() {
  local dir="$1"
  local label="$2"
  if [[ -n "$(git -C "${dir}" status --porcelain)" ]]; then
    fail "${label} is not a clean git tree: ${dir}"
  fi
}

require_cmd git
require_cmd jq
[[ -f "${MANIFEST_PATH}" ]] || fail "manifest not found: ${MANIFEST_PATH}"

jq -e '.fixtures and (.fixtures | type == "array")' "${MANIFEST_PATH}" >/dev/null \
  || fail "invalid manifest schema in ${MANIFEST_PATH}"
log "Manifest: ${MANIFEST_PATH}"
log "Fixture output root: ${FIXTURES_ROOT}"

while IFS= read -r fixture_json; do
  id="$(jq -r '.id // empty' <<<"${fixture_json}")"
  post_ref="$(jq -r '.post_ref // empty' <<<"${fixture_json}")"
  [[ -n "${id}" ]] || fail "fixture id is missing in manifest entry: ${fixture_json}"
  [[ -n "${post_ref}" ]] || fail "fixture post_ref is missing for fixture '${id}'"

  strip_files=()
  while IFS= read -r rel_path; do
    strip_files+=("${rel_path}")
  done < <(jq -r '.strip_for_pre[]?' <<<"${fixture_json}")
  ((${#strip_files[@]} > 0)) || fail "fixture '${id}' has empty strip_for_pre list"

  fixture_root="${FIXTURES_ROOT}/${id}"
  post_dir="${fixture_root}/post"
  pre_dir="${fixture_root}/pre"

  log "Verifying fixture '${id}'"
  [[ -d "${post_dir}/.git" ]] || fail "post variant missing git repo for fixture '${id}'"
  [[ -d "${pre_dir}/.git" ]] || fail "pre variant missing git repo for fixture '${id}'"

  log "  Checking strip_for_pre expectations across post/pre variants"
  for rel_path in "${strip_files[@]}"; do
    [[ -e "${post_dir}/${rel_path}" ]] || fail "fixture '${id}': '${rel_path}' missing in post variant"
    [[ ! -e "${pre_dir}/${rel_path}" ]] || fail "fixture '${id}': '${rel_path}' still exists in pre variant"
  done

  log "  Validating post_ref presence and pre ancestry"
  git -C "${post_dir}" rev-parse --verify "${post_ref}^{commit}" >/dev/null 2>&1 \
    || fail "fixture '${id}': post_ref '${post_ref}' is missing in post variant"
  git -C "${pre_dir}" merge-base --is-ancestor "${post_ref}" HEAD >/dev/null 2>&1 \
    || fail "fixture '${id}': pre variant does not derive from post_ref '${post_ref}'"

  log "  Validating both repos are clean"
  assert_clean_git_tree "${post_dir}" "post variant for fixture '${id}'"
  assert_clean_git_tree "${pre_dir}" "pre variant for fixture '${id}'"

  log "Verified fixture '${id}'"
done < <(jq -c '.fixtures[]' "${MANIFEST_PATH}")

log "Fixture verification complete"
