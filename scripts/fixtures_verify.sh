#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
MANIFEST_PATH="${REPO_ROOT}/fixtures/manifest.json"
FIXTURES_ROOT="${REPO_ROOT}/.fixtures"
RESET_LIB_PATH="${REPO_ROOT}/scripts/fixtures_reset_lib.sh"

# shellcheck source=./fixtures_reset_lib.sh
source "${RESET_LIB_PATH}"

fail() {
  echo "error: $*" >&2
  exit 1
}

log() {
  echo "[fixtures_verify] $*"
}

warn() {
  echo "[fixtures_verify] warning: $*" >&2
}

require_cmd() {
  local cmd="$1"
  command -v "${cmd}" >/dev/null 2>&1 || fail "required command '${cmd}' not found"
}

is_lfs_pointer_file() {
  local path="$1"
  [[ -f "${path}" ]] || return 1
  local first_line
  IFS= read -r first_line <"${path}" || return 1
  [[ "${first_line}" == "version https://git-lfs.github.com/spec/v1" ]]
}

collect_relevant_model_files() {
  local repo_dir="$1"
  while IFS= read -r abs_path; do
    [[ -n "${abs_path}" ]] || continue
    echo "${abs_path#"${repo_dir}/"}"
  done < <(
    find "${repo_dir}" -type f \
      \( -name '*.onnx' \
      -o -name '*.h5' \
      -o -name '*.hdf5' \
      -o -name '*.keras' \
      -o -name '*.pt' \
      -o -name '*.pth' \
      -o -name '*.ckpt' \
      -o -name '*.pb' \
      -o -name '*.tflite' \
      -o -name '*.engine' \) \
      | sort
  )
}

assert_relevant_model_files_hydrated() {
  local repo_dir="$1"
  local label="$2"
  local strict_lfs="${STRICT_FIXTURE_LFS:-0}"

  local relevant_model_files=()
  while IFS= read -r rel_path; do
    [[ -n "${rel_path}" ]] || continue
    relevant_model_files+=("${rel_path}")
  done < <(collect_relevant_model_files "${repo_dir}")

  ((${#relevant_model_files[@]} > 0)) || return 0

  local unresolved_lfs=()
  local rel_path
  for rel_path in "${relevant_model_files[@]}"; do
    if is_lfs_pointer_file "${repo_dir}/${rel_path}"; then
      unresolved_lfs+=("${rel_path}")
    fi
  done

  if ((${#unresolved_lfs[@]} > 0)); then
    if [[ "${strict_lfs}" == "1" ]]; then
      fail "${label}: relevant model files are still LFS pointers: ${unresolved_lfs[*]}"
    fi
    warn "${label}: relevant model files are still LFS pointers (best-effort mode): ${unresolved_lfs[*]}"
  fi
}

strip_entry_covers_path() {
  local needle="$1"
  shift
  local item
  for item in "$@"; do
    [[ "${item}" == "${needle}" ]] && return 0
    [[ "${needle}" == "${item}/"* ]] && return 0
  done
  return 1
}

collect_python_code_loader_files() {
  local repo_dir="$1"
  while IFS= read -r abs_path; do
    [[ -n "${abs_path}" ]] || continue
    echo "${abs_path#"${repo_dir}/"}"
  done < <(rg -l --glob '*.py' 'code_loader|inner_leap_binder|leapbinder_decorators' "${repo_dir}" | sort || true)
}

collect_tensorleap_folder_dirs() {
  local repo_dir="$1"
  while IFS= read -r abs_path; do
    [[ -n "${abs_path}" ]] || continue
    echo "${abs_path#"${repo_dir}/"}"
  done < <(find "${repo_dir}" -type d \( -name 'tensorleap_folder' -o -name '.tensorleap' \) | sort)
}

collect_tensorleap_mapping_files() {
  local repo_dir="$1"
  while IFS= read -r abs_path; do
    [[ -n "${abs_path}" ]] || continue
    echo "${abs_path#"${repo_dir}/"}"
  done < <(find "${repo_dir}" -type f \( -name 'leap_mapping*.yaml' -o -name 'leap_mapping*.yml' \) | sort)
}

should_ignore_tensorleap_text_match() {
  local rel_path="$1"
  [[ "$(basename "${rel_path}")" == "pyproject.toml" ]]
}

collect_tensorleap_text_files() {
  local repo_dir="$1"
  while IFS= read -r abs_path; do
    [[ -n "${abs_path}" ]] || continue
    local rel_path="${abs_path#"${repo_dir}/"}"
    should_ignore_tensorleap_text_match "${rel_path}" && continue
    echo "${rel_path}"
  done < <(rg -n --ignore-case --files-with-matches "tensorleap" "${repo_dir}" || true)
}

assert_clean_git_tree() {
  local dir="$1"
  local label="$2"
  if [[ -n "$(git -C "${dir}" status --porcelain)" ]]; then
    fail "${label} is not a clean git tree: ${dir}"
  fi
}

path_is_listed() {
  local needle="$1"
  shift
  local item
  for item in "$@"; do
    [[ "${item}" == "${needle}" ]] && return 0
  done
  return 1
}

assert_placeholder_readme() {
  local path="$1"
  local label="$2"

  [[ -f "${path}" ]] || fail "${label} expected placeholder README at ${path}"
  grep -q '^# Fixture Placeholder$' "${path}" \
    || fail "${label} placeholder README missing marker line: ${path}"
}

assert_fixture_reset_script() {
  local repo_dir="$1"
  local label="$2"
  local script_path="${repo_dir}/.fixture_reset.sh"

  [[ -f "${script_path}" ]] || fail "${label} is missing .fixture_reset.sh"
  [[ -x "${script_path}" ]] || fail "${label} has a non-executable .fixture_reset.sh"
}

choose_reset_probe_file() {
  local repo_dir="$1"
  local rel_path

  rel_path="$(git -C "${repo_dir}" ls-files -- '*.md' '*.py' '*.yaml' '*.yml' '*.txt' | head -n 1)"
  if [[ -z "${rel_path}" ]]; then
    rel_path="$(git -C "${repo_dir}" ls-files | head -n 1)"
  fi

  [[ -n "${rel_path}" ]] || fail "could not find a tracked file to dirty in ${repo_dir}"
  printf '%s\n' "${rel_path}"
}

dirty_repo_for_reset_check() {
  local repo_dir="$1"
  local label="$2"
  local probe_file
  probe_file="$(choose_reset_probe_file "${repo_dir}")"

  printf '\n# fixture reset probe\n' >>"${repo_dir}/${probe_file}"
  touch "${repo_dir}/.fixture_reset_probe.tmp"

  if [[ -z "$(git -C "${repo_dir}" status --porcelain)" ]]; then
    fail "${label} did not become dirty during reset-script verification"
  fi
}

exercise_fixture_reset_script() {
  local repo_dir="$1"
  local label="$2"
  local expected_head="$3"
  local script_path="${repo_dir}/.fixture_reset.sh"

  assert_fixture_reset_script "${repo_dir}" "${label}"
  dirty_repo_for_reset_check "${repo_dir}" "${label}"
  "${script_path}"

  local actual_head
  actual_head="$(git -C "${repo_dir}" rev-parse HEAD)"
  [[ "${actual_head}" == "${expected_head}" ]] \
    || fail "${label} reset script restored unexpected HEAD: expected ${expected_head}, got ${actual_head}"

  assert_clean_git_tree "${repo_dir}" "${label}"
}

require_cmd git
require_cmd jq
require_cmd rg
[[ -f "${MANIFEST_PATH}" ]] || fail "manifest not found: ${MANIFEST_PATH}"
[[ -f "${RESET_LIB_PATH}" ]] || fail "fixture reset library not found: ${RESET_LIB_PATH}"

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

  stripped_py_basenames=()
  for rel_path in "${strip_files[@]}"; do
    if [[ "${rel_path}" == *.py ]]; then
      stripped_py_basenames+=("$(basename "${rel_path}" .py)")
    fi
  done

  fixture_root="${FIXTURES_ROOT}/${id}"
  post_dir="${fixture_root}/post"
  pre_dir="${fixture_root}/pre"

  log "Verifying fixture '${id}'"
  [[ -d "${post_dir}/.git" ]] || fail "post variant missing git repo for fixture '${id}'"
  [[ -d "${pre_dir}/.git" ]] || fail "pre variant missing git repo for fixture '${id}'"
  assert_fixture_reset_script "${post_dir}" "post variant for fixture '${id}'"
  assert_fixture_reset_script "${pre_dir}" "pre variant for fixture '${id}'"

  declared_pre_readmes=()
  while IFS= read -r rel_path; do
    [[ -n "${rel_path}" ]] || continue
    declared_pre_readmes+=("${rel_path}")
  done < <(fixture_collect_declared_readme_files "${pre_dir}")

  log "  Checking strip_for_pre expectations across post/pre variants"
  for rel_path in "${strip_files[@]}"; do
    [[ -e "${post_dir}/${rel_path}" ]] || fail "fixture '${id}': '${rel_path}' missing in post variant"
    if path_is_listed "${rel_path}" "${declared_pre_readmes[@]}"; then
      assert_placeholder_readme "${pre_dir}/${rel_path}" "fixture '${id}'"
      continue
    fi
    [[ ! -e "${pre_dir}/${rel_path}" ]] || fail "fixture '${id}': '${rel_path}' still exists in pre variant"
  done

  post_root_leap_files=()
  while IFS= read -r rel_path; do
    [[ -n "${rel_path}" ]] && post_root_leap_files+=("${rel_path}")
  done < <(find "${post_dir}" -maxdepth 1 -type f -name 'leap*' -exec basename {} \; | sort)
  for rel_path in "${post_root_leap_files[@]}"; do
    strip_entry_covers_path "${rel_path}" "${strip_files[@]}" \
      || fail "fixture '${id}': root leap file '${rel_path}' is present in post but missing from strip_for_pre"
  done

  post_tensorleap_files=()
  while IFS= read -r rel_path; do
    [[ -n "${rel_path}" ]] || continue
    post_tensorleap_files+=("${rel_path}")
  done < <(collect_tensorleap_text_files "${post_dir}")
  for rel_path in "${post_tensorleap_files[@]}"; do
    strip_entry_covers_path "${rel_path}" "${strip_files[@]}" \
      || fail "fixture '${id}': post file '${rel_path}' contains 'tensorleap' but is missing from strip_for_pre"
  done

  post_code_loader_python_files=()
  while IFS= read -r rel_path; do
    [[ -n "${rel_path}" ]] || continue
    post_code_loader_python_files+=("${rel_path}")
  done < <(collect_python_code_loader_files "${post_dir}")
  for rel_path in "${post_code_loader_python_files[@]}"; do
    strip_entry_covers_path "${rel_path}" "${strip_files[@]}" \
      || fail "fixture '${id}': post Python file '${rel_path}' imports code_loader but is missing from strip_for_pre"
  done

  post_tensorleap_dirs=()
  while IFS= read -r rel_path; do
    [[ -n "${rel_path}" ]] || continue
    post_tensorleap_dirs+=("${rel_path}")
  done < <(collect_tensorleap_folder_dirs "${post_dir}")
  for rel_path in "${post_tensorleap_dirs[@]}"; do
    strip_entry_covers_path "${rel_path}" "${strip_files[@]}" \
      || fail "fixture '${id}': post directory '${rel_path}' is Tensorleap-only content but is missing from strip_for_pre"
  done

  post_tensorleap_mapping_files=()
  while IFS= read -r rel_path; do
    [[ -n "${rel_path}" ]] || continue
    post_tensorleap_mapping_files+=("${rel_path}")
  done < <(collect_tensorleap_mapping_files "${post_dir}")
  for rel_path in "${post_tensorleap_mapping_files[@]}"; do
    strip_entry_covers_path "${rel_path}" "${strip_files[@]}" \
      || fail "fixture '${id}': post mapping file '${rel_path}' is Tensorleap-only content but is missing from strip_for_pre"
  done

  pre_root_leap_files=()
  while IFS= read -r rel_path; do
    [[ -n "${rel_path}" ]] && pre_root_leap_files+=("${rel_path}")
  done < <(find "${pre_dir}" -maxdepth 1 -type f -name 'leap*' -exec basename {} \; | sort)
  ((${#pre_root_leap_files[@]} == 0)) \
    || fail "fixture '${id}': pre variant still has root leap files: ${pre_root_leap_files[*]}"

  pre_leap_pyc_files=()
  while IFS= read -r abs_path; do
    [[ -n "${abs_path}" ]] || continue
    pre_leap_pyc_files+=("${abs_path#"${pre_dir}/"}")
  done < <(find "${pre_dir}" -type f -path '*/__pycache__/leap*.pyc' | sort)
  ((${#pre_leap_pyc_files[@]} == 0)) \
    || fail "fixture '${id}': pre variant has compiled leap artifacts: ${pre_leap_pyc_files[*]}"

  for base_name in "${stripped_py_basenames[@]}"; do
    pre_compiled_matches=()
    while IFS= read -r abs_path; do
      [[ -n "${abs_path}" ]] || continue
      pre_compiled_matches+=("${abs_path#"${pre_dir}/"}")
    done < <(find "${pre_dir}" -type f -path "*/__pycache__/${base_name}*.pyc" | sort)
    ((${#pre_compiled_matches[@]} == 0)) \
      || fail "fixture '${id}': pre variant has compiled artifacts for stripped '${base_name}.py': ${pre_compiled_matches[*]}"
  done

  pre_code_loader_python_files=()
  while IFS= read -r rel_path; do
    [[ -n "${rel_path}" ]] || continue
    pre_code_loader_python_files+=("${rel_path}")
  done < <(collect_python_code_loader_files "${pre_dir}")
  ((${#pre_code_loader_python_files[@]} == 0)) \
    || fail "fixture '${id}': pre variant contains Python files importing code_loader: ${pre_code_loader_python_files[*]}"

  pre_tensorleap_dirs=()
  while IFS= read -r rel_path; do
    [[ -n "${rel_path}" ]] || continue
    pre_tensorleap_dirs+=("${rel_path}")
  done < <(collect_tensorleap_folder_dirs "${pre_dir}")
  ((${#pre_tensorleap_dirs[@]} == 0)) \
    || fail "fixture '${id}': pre variant contains Tensorleap directories: ${pre_tensorleap_dirs[*]}"

  pre_tensorleap_mapping_files=()
  while IFS= read -r rel_path; do
    [[ -n "${rel_path}" ]] || continue
    pre_tensorleap_mapping_files+=("${rel_path}")
  done < <(collect_tensorleap_mapping_files "${pre_dir}")
  ((${#pre_tensorleap_mapping_files[@]} == 0)) \
    || fail "fixture '${id}': pre variant contains Tensorleap mapping files: ${pre_tensorleap_mapping_files[*]}"

  pre_tensorleap_files=()
  while IFS= read -r rel_path; do
    [[ -n "${rel_path}" ]] || continue
    pre_tensorleap_files+=("${rel_path}")
  done < <(collect_tensorleap_text_files "${pre_dir}")
  ((${#pre_tensorleap_files[@]} == 0)) \
    || fail "fixture '${id}': pre variant contains files with 'tensorleap': ${pre_tensorleap_files[*]}"

  log "  Validating post_ref presence and pre ancestry"
  git -C "${post_dir}" rev-parse --verify "${post_ref}^{commit}" >/dev/null 2>&1 \
    || fail "fixture '${id}': post_ref '${post_ref}' is missing in post variant"
  git -C "${pre_dir}" merge-base --is-ancestor "${post_ref}" HEAD >/dev/null 2>&1 \
    || fail "fixture '${id}': pre variant does not derive from post_ref '${post_ref}'"

  log "  Validating relevant model files are hydrated (not LFS pointers)"
  assert_relevant_model_files_hydrated "${post_dir}" "post variant for fixture '${id}'"

  log "  Validating both repos are clean"
  assert_clean_git_tree "${post_dir}" "post variant for fixture '${id}'"
  assert_clean_git_tree "${pre_dir}" "pre variant for fixture '${id}'"

  log "  Exercising generated reset scripts"
  exercise_fixture_reset_script "${post_dir}" "post variant for fixture '${id}'" "${post_ref}"
  exercise_fixture_reset_script \
    "${pre_dir}" \
    "pre variant for fixture '${id}'" \
    "$(git -C "${pre_dir}" rev-parse HEAD)"

  log "Verified fixture '${id}'"
done < <(jq -c '.fixtures[]' "${MANIFEST_PATH}")

log "Fixture verification complete"
