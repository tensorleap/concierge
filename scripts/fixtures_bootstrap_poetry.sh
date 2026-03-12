#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
FIXTURES_ROOT="${REPO_ROOT}/.fixtures"
CASE_MANIFEST_PATH="${REPO_ROOT}/fixtures/cases/manifest.json"
RESET_LIB_PATH="${REPO_ROOT}/scripts/fixtures_reset_lib.sh"

DEFAULT_PYTHON_VERSION="${FIXTURE_BOOTSTRAP_PYTHON:-3.10.14}"

# shellcheck source=./fixtures_reset_lib.sh
source "${RESET_LIB_PATH}"

usage() {
  cat <<'EOF'
Usage: bash scripts/fixtures_bootstrap_poetry.sh [options]

Bootstrap Poetry environments for prepared fixture repositories.
This script is fixture/dev-only and must not be used by Concierge product runtime.

Options:
  --fixture ID         Limit bootstrap to one fixture ID from fixtures/manifest.json.
  --case ID            Limit bootstrap to one generated case ID from fixtures/cases/manifest.json.
  --variant KIND       One of pre, post, cases, or all (default: all).
  --python VERSION     Python version to install/use via pyenv (default: 3.10.14).
  --help               Show this help text.
EOF
}

fail() {
  echo "error: $*" >&2
  exit 1
}

log() {
  echo "[fixtures_bootstrap_poetry] $*"
}

require_cmd() {
  local cmd="$1"
  command -v "${cmd}" >/dev/null 2>&1 || fail "required command '${cmd}' not found"
}

fixture_id=""
case_id=""
variant="all"
python_version="${DEFAULT_PYTHON_VERSION}"

while (($# > 0)); do
  case "$1" in
    --fixture)
      shift
      [[ $# -gt 0 ]] || fail "--fixture requires a value"
      fixture_id="$1"
      ;;
    --case)
      shift
      [[ $# -gt 0 ]] || fail "--case requires a value"
      case_id="$1"
      ;;
    --variant)
      shift
      [[ $# -gt 0 ]] || fail "--variant requires a value"
      variant="$1"
      ;;
    --python)
      shift
      [[ $# -gt 0 ]] || fail "--python requires a value"
      python_version="$1"
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

case "${variant}" in
  pre|post|cases|all)
    ;;
  *)
    fail "unsupported --variant value '${variant}'"
    ;;
esac

require_cmd git
require_cmd jq
require_cmd poetry
require_cmd pyenv
require_cmd python3
[[ -f "${RESET_LIB_PATH}" ]] || fail "fixture reset library not found: ${RESET_LIB_PATH}"
[[ -d "${FIXTURES_ROOT}" ]] || fail "fixture output root not found: ${FIXTURES_ROOT} (run fixtures_prepare.sh first)"

log "Ensuring Python ${python_version} is available via pyenv"
pyenv install -s "${python_version}" >/dev/null
python_executable="$(pyenv prefix "${python_version}")/bin/python"
[[ -x "${python_executable}" ]] || fail "resolved python executable not found: ${python_executable}"

bootstrap_repo() {
  local repo_dir="$1"
  local label="$2"

  [[ -f "${repo_dir}/pyproject.toml" ]] || {
    log "Skipping ${repo_dir}: no pyproject.toml"
    return 0
  }

  log "Bootstrapping ${repo_dir}"
  fixture_assert_min_code_loader_pin "${repo_dir}" "${label}"
  (
    cd "${repo_dir}"
    POETRY_VIRTUALENVS_IN_PROJECT=true poetry env use "${python_executable}" >/dev/null
    POETRY_VIRTUALENVS_IN_PROJECT=true poetry install --no-root >/dev/null
  )
  fixture_assert_min_installed_code_loader_version "${repo_dir}" "${label}"
}

collect_fixture_dirs() {
  local kind="$1"
  local id

  if [[ -n "${fixture_id}" ]]; then
    printf '%s\n' "${FIXTURES_ROOT}/${fixture_id}/${kind}"
    return 0
  fi

  jq -r '.fixtures[].id' "${REPO_ROOT}/fixtures/manifest.json" \
    | while IFS= read -r id; do
        [[ -n "${id}" ]] || continue
        printf '%s\n' "${FIXTURES_ROOT}/${id}/${kind}"
      done
}

collect_case_dirs() {
  local id

  [[ -f "${CASE_MANIFEST_PATH}" ]] || return 0

  if [[ -n "${case_id}" ]]; then
    printf '%s\n' "${FIXTURES_ROOT}/cases/${case_id}"
    return 0
  fi

  if [[ -n "${fixture_id}" ]]; then
    jq -r --arg fixture_id "${fixture_id}" '.cases[] | select(.source_fixture_id == $fixture_id) | .id' "${CASE_MANIFEST_PATH}" \
      | while IFS= read -r id; do
          [[ -n "${id}" ]] || continue
          printf '%s\n' "${FIXTURES_ROOT}/cases/${id}"
        done
    return 0
  fi

  jq -r '.cases[].id' "${CASE_MANIFEST_PATH}" \
    | while IFS= read -r id; do
        [[ -n "${id}" ]] || continue
        printf '%s\n' "${FIXTURES_ROOT}/cases/${id}"
      done
}

declare -A seen_dirs=()
bootstrap_targets=()
bootstrap_labels=()

add_target() {
  local dir="$1"
  local label="$2"
  [[ -d "${dir}/.git" ]] || return 0
  if [[ -z "${seen_dirs["${dir}"]+x}" ]]; then
    seen_dirs["${dir}"]=1
    bootstrap_targets+=("${dir}")
    bootstrap_labels+=("${label}")
  fi
}

if [[ "${variant}" == "pre" || "${variant}" == "all" ]]; then
  while IFS= read -r dir; do
    [[ -n "${dir}" ]] || continue
    add_target "${dir}" "fixture repo ${dir}"
  done < <(collect_fixture_dirs "pre")
fi

if [[ "${variant}" == "post" || "${variant}" == "all" ]]; then
  while IFS= read -r dir; do
    [[ -n "${dir}" ]] || continue
    add_target "${dir}" "fixture repo ${dir}"
  done < <(collect_fixture_dirs "post")
fi

if [[ "${variant}" == "cases" || "${variant}" == "all" ]]; then
  while IFS= read -r dir; do
    [[ -n "${dir}" ]] || continue
    add_target "${dir}" "fixture repo ${dir}"
  done < <(collect_case_dirs)
fi

((${#bootstrap_targets[@]} > 0)) || fail "no matching fixture repositories found for bootstrap"

for i in "${!bootstrap_targets[@]}"; do
  bootstrap_repo "${bootstrap_targets[$i]}" "${bootstrap_labels[$i]}"
done

log "Fixture Poetry bootstrap complete"
