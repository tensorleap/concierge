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
  echo "[fixtures_prepare] $*"
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

git_fixture() {
  GIT_LFS_SKIP_SMUDGE=1 git "$@"
}

mkdir -p "${FIXTURES_ROOT}"
log "Manifest: ${MANIFEST_PATH}"
log "Fixture output root: ${FIXTURES_ROOT}"

while IFS= read -r fixture_json; do
  id="$(jq -r '.id // empty' <<<"${fixture_json}")"
  repo="$(jq -r '.repo // empty' <<<"${fixture_json}")"
  post_ref="$(jq -r '.post_ref // empty' <<<"${fixture_json}")"

  [[ -n "${id}" ]] || fail "fixture id is missing in manifest entry: ${fixture_json}"
  [[ -n "${repo}" ]] || fail "fixture repo is missing for fixture '${id}'"
  [[ -n "${post_ref}" ]] || fail "fixture post_ref is missing for fixture '${id}'"

  strip_files=()
  while IFS= read -r rel_path; do
    strip_files+=("${rel_path}")
  done < <(jq -r '.strip_for_pre[]?' <<<"${fixture_json}")
  ((${#strip_files[@]} > 0)) || fail "fixture '${id}' has empty strip_for_pre list"

  fixture_root="${FIXTURES_ROOT}/${id}"
  post_dir="${fixture_root}/post"
  pre_dir="${fixture_root}/pre"

  log "Preparing fixture '${id}'"
  log "  Source repository: ${repo}"
  log "  Pinned post_ref: ${post_ref}"

  log "  Resetting existing fixture directories"
  rm -rf "${post_dir}" "${pre_dir}"
  mkdir -p "${fixture_root}"

  log "  Cloning post variant and checking out pinned commit"
  git_fixture clone --quiet --no-checkout --filter=blob:none "${repo}" "${post_dir}"
  git_fixture -C "${post_dir}" checkout --quiet "${post_ref}"

  log "  Verifying required integration files exist in post variant"
  for rel_path in "${strip_files[@]}"; do
    [[ -e "${post_dir}/${rel_path}" ]] || fail "fixture '${id}' missing '${rel_path}' in post variant"
  done

  assert_clean_git_tree "${post_dir}" "post variant for fixture '${id}'"

  log "  Creating pre variant from source repository"
  git_fixture clone --quiet --no-checkout --filter=blob:none "${repo}" "${pre_dir}"
  git_fixture -C "${pre_dir}" checkout --quiet "${post_ref}"

  log "  Stripping pre-integration files from pre variant"
  for rel_path in "${strip_files[@]}"; do
    rm -f -- "${pre_dir:?}/${rel_path}"
  done

  log "  Committing stripped pre variant so git tree remains clean"
  git -C "${pre_dir}" add -A
  git -C "${pre_dir}" diff --cached --quiet \
    && fail "fixture '${id}' had no changes after stripping files"

  GIT_AUTHOR_NAME="Concierge Fixture Bot" \
  GIT_AUTHOR_EMAIL="concierge-fixtures@local" \
  GIT_AUTHOR_DATE="2000-01-01T00:00:00Z" \
  GIT_COMMITTER_NAME="Concierge Fixture Bot" \
  GIT_COMMITTER_EMAIL="concierge-fixtures@local" \
  GIT_COMMITTER_DATE="2000-01-01T00:00:00Z" \
    git -C "${pre_dir}" commit --quiet -m "Create pre-integration fixture variant"

  assert_clean_git_tree "${pre_dir}" "pre variant for fixture '${id}'"

  log "Prepared fixture '${id}' at ${fixture_root}"
done < <(jq -c '.fixtures[]' "${MANIFEST_PATH}")

log "Fixture preparation complete"
