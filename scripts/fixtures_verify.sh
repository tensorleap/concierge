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

assert_clean_git_tree() {
  local dir="$1"
  local label="$2"
  if [[ -n "$(git -C "${dir}" status --porcelain)" ]]; then
    fail "${label} is not a clean git tree: ${dir}"
  fi
}

require_cmd git
require_cmd jq
require_cmd rg
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

  log "  Checking strip_for_pre expectations across post/pre variants"
  for rel_path in "${strip_files[@]}"; do
    [[ -e "${post_dir}/${rel_path}" ]] || fail "fixture '${id}': '${rel_path}' missing in post variant"
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
  while IFS= read -r abs_path; do
    [[ -n "${abs_path}" ]] || continue
    post_tensorleap_files+=("${abs_path#"${post_dir}/"}")
  done < <(rg -n --ignore-case --files-with-matches "tensorleap" "${post_dir}" || true)
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
  while IFS= read -r abs_path; do
    [[ -n "${abs_path}" ]] || continue
    pre_tensorleap_files+=("${abs_path#"${pre_dir}/"}")
  done < <(rg -n --ignore-case --files-with-matches "tensorleap" "${pre_dir}" || true)
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

  log "Verified fixture '${id}'"
done < <(jq -c '.fixtures[]' "${MANIFEST_PATH}")

log "Fixture verification complete"
