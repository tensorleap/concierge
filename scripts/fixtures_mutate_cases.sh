#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
FIXTURES_ROOT="${REPO_ROOT}/.fixtures"
CASE_MANIFEST_PATH="${REPO_ROOT}/fixtures/cases/manifest.json"
CASE_SCHEMA_PATH="${REPO_ROOT}/fixtures/cases/schema.json"
RESET_LIB_PATH="${REPO_ROOT}/scripts/fixtures_reset_lib.sh"
BOOTSTRAP_SCRIPT_PATH="${REPO_ROOT}/scripts/fixtures_bootstrap_poetry.sh"

# shellcheck source=./fixtures_reset_lib.sh
source "${RESET_LIB_PATH}"

usage() {
  cat <<'EOF'
Usage: bash scripts/fixtures_mutate_cases.sh [--case <case-id>] [--bootstrap-poetry]

Generate deterministic guide-native fixture cases under .fixtures/cases/.

Options:
  --case ID            Generate only the selected case ID from fixtures/cases/manifest.json.
  --bootstrap-poetry   After generating cases, bootstrap Poetry environments explicitly.
  --help               Show this help text.
EOF
}

fail() {
  echo "error: $*" >&2
  exit 1
}

log() {
  echo "[fixtures_mutate_cases] $*"
}

require_cmd() {
  local cmd="$1"
  command -v "${cmd}" >/dev/null 2>&1 || fail "required command '${cmd}' not found"
}

bootstrap_poetry=0
selected_case_id=""
while (($# > 0)); do
  case "$1" in
    --case)
      shift
      [[ $# -gt 0 ]] || fail "--case requires a value"
      selected_case_id="$1"
      ;;
    --bootstrap-poetry)
      bootstrap_poetry=1
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    *)
      fail "unknown argument: $1"
      ;;
  esac
  shift
done

require_cmd git
require_cmd jq
[[ -f "${CASE_MANIFEST_PATH}" ]] || fail "case manifest not found: ${CASE_MANIFEST_PATH}"
[[ -f "${CASE_SCHEMA_PATH}" ]] || fail "case schema not found: ${CASE_SCHEMA_PATH}"
[[ -f "${RESET_LIB_PATH}" ]] || fail "fixture reset library not found: ${RESET_LIB_PATH}"
[[ -f "${BOOTSTRAP_SCRIPT_PATH}" ]] || fail "fixture bootstrap script not found: ${BOOTSTRAP_SCRIPT_PATH}"

jq -e '
  .cases
  and (.cases | type == "array")
  and ([.cases[] |
      (.id | type == "string" and length > 0)
      and (.source_fixture_id | type == "string" and length > 0)
      and (.source_variant == "post")
      and (.family | type == "string" and length > 0)
      and (.patch | type == "string" and length > 0)
      and (.expected_primary_step | type == "string" and length > 0)
      and (.expected_issue_codes | type == "array" and length > 0)
    ] | all)
' "${CASE_MANIFEST_PATH}" >/dev/null || fail "invalid case manifest shape in ${CASE_MANIFEST_PATH}"

mkdir -p "${FIXTURES_ROOT}/cases"
log "Case manifest: ${CASE_MANIFEST_PATH}"
log "Case output root: ${FIXTURES_ROOT}/cases"
if [[ -n "${selected_case_id}" ]]; then
  log "Targeted case generation enabled for case '${selected_case_id}'"
fi

write_case_reset_script() {
  local repo_dir="$1"
  local source_ref="$2"
  local patch_path="$3"
  local commit_message="$4"
  local script_path="${repo_dir}/.fixture_reset.sh"

  {
    echo '#!/usr/bin/env bash'
    echo 'set -euo pipefail'
    echo
    echo 'SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"'
    echo 'CONCIERGE_ROOT="$(cd -- "${SCRIPT_DIR}/../../.." && pwd)"'
    echo
    echo '# shellcheck source=/dev/null'
    echo 'source "${CONCIERGE_ROOT}/scripts/fixtures_reset_lib.sh"'
    printf 'SOURCE_REF=%q\n' "${source_ref}"
    printf 'PATCH_PATH=%q\n' "${patch_path}"
    printf 'COMMIT_MESSAGE=%q\n' "${commit_message}"
    echo
    echo 'fixture_reset_case_variant "${SCRIPT_DIR}" "${SOURCE_REF}" "${PATCH_PATH}" "${COMMIT_MESSAGE}"'
  } >"${script_path}"

  chmod +x "${script_path}"
  fixture_add_local_exclude "${repo_dir}" "/.fixture_reset.sh"
}

reset_case_dir() {
  local dir="$1"
  [[ -e "${dir}" ]] || return 0
  chmod -R u+w "${dir}" 2>/dev/null || true
  find "${dir}" -depth -delete
  [[ ! -e "${dir}" ]] || fail "failed to reset case directory: ${dir}"
}

processed_cases=0
while IFS= read -r case_json; do
  current_case_id="$(jq -r '.id' <<<"${case_json}")"
  source_fixture_id="$(jq -r '.source_fixture_id' <<<"${case_json}")"
  source_variant="$(jq -r '.source_variant' <<<"${case_json}")"
  patch_relpath="$(jq -r '.patch' <<<"${case_json}")"
  patch_path="${REPO_ROOT}/${patch_relpath}"
  commit_message="Create fixture case ${current_case_id}"

  source_dir="${FIXTURES_ROOT}/${source_fixture_id}/${source_variant}"
  case_dir="${FIXTURES_ROOT}/cases/${current_case_id}"

  [[ -d "${source_dir}/.git" ]] || fail "source fixture repo missing for case '${current_case_id}': ${source_dir}"
  [[ -f "${patch_path}" ]] || fail "patch file missing for case '${current_case_id}': ${patch_relpath}"
  source_ref="$(git -C "${source_dir}" rev-parse HEAD)"

  log "Generating case '${current_case_id}' from fixture '${source_fixture_id}'"
  reset_case_dir "${case_dir}"
  cp -R "${source_dir}" "${case_dir}"
  git -C "${case_dir}" checkout --quiet "${source_ref}"
  fixture_apply_case_patch "${case_dir}" "${patch_path}" "${commit_message}"
  write_case_reset_script "${case_dir}" "${source_ref}" "${patch_path}" "${commit_message}"

  if [[ -n "$(git -C "${case_dir}" status --porcelain)" ]]; then
    fail "generated case '${current_case_id}' is not a clean git tree"
  fi
  processed_cases=$((processed_cases + 1))
done < <(
  if [[ -n "${selected_case_id}" ]]; then
    jq -c --arg case_id "${selected_case_id}" '.cases[] | select(.id == $case_id)' "${CASE_MANIFEST_PATH}"
  else
    jq -c '.cases[]' "${CASE_MANIFEST_PATH}"
  fi
)

if [[ "${processed_cases}" == "0" ]]; then
  if [[ -n "${selected_case_id}" ]]; then
    fail "unknown case id '${selected_case_id}' (see ${CASE_MANIFEST_PATH})"
  fi
  fail "no fixture cases found in ${CASE_MANIFEST_PATH}"
fi

if [[ "${bootstrap_poetry}" == "1" ]]; then
  log "Bootstrapping Poetry environments for generated cases"
  bootstrap_args=(--variant cases)
  if [[ -n "${selected_case_id}" ]]; then
    bootstrap_args+=(--case "${selected_case_id}")
  fi
  bash "${BOOTSTRAP_SCRIPT_PATH}" "${bootstrap_args[@]}"
fi

log "Fixture case generation complete"
