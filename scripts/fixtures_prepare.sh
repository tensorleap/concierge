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

warn() {
  echo "[fixtures_prepare] warning: $*" >&2
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

ensure_relevant_model_lfs_hydrated() {
  local repo_dir="$1"
  local label="$2"
  local strict_lfs="${STRICT_FIXTURE_LFS:-0}"

  local relevant_model_files=()
  while IFS= read -r rel_path; do
    [[ -n "${rel_path}" ]] || continue
    relevant_model_files+=("${rel_path}")
  done < <(collect_relevant_model_files "${repo_dir}")

  ((${#relevant_model_files[@]} > 0)) || return 0

  local lfs_pointer_files=()
  local rel_path
  for rel_path in "${relevant_model_files[@]}"; do
    if is_lfs_pointer_file "${repo_dir}/${rel_path}"; then
      lfs_pointer_files+=("${rel_path}")
    fi
  done

  ((${#lfs_pointer_files[@]} > 0)) || return 0

  git lfs version >/dev/null 2>&1 \
    || fail "${label}: found LFS pointer model files but git-lfs is unavailable: ${lfs_pointer_files[*]}"

  local include_csv
  include_csv="$(printf '%s,' "${lfs_pointer_files[@]}")"
  include_csv="${include_csv%,}"

  log "  Hydrating ${#lfs_pointer_files[@]} LFS model file(s) in ${label}"
  if ! git -C "${repo_dir}" lfs pull --include "${include_csv}" --exclude ""; then
    if [[ "${strict_lfs}" == "1" ]]; then
      fail "${label}: git lfs pull failed for model file(s): ${lfs_pointer_files[*]}"
    fi
    warn "${label}: git lfs pull failed in best-effort mode for model file(s): ${lfs_pointer_files[*]}"
  fi

  local unresolved_lfs=()
  for rel_path in "${lfs_pointer_files[@]}"; do
    if is_lfs_pointer_file "${repo_dir}/${rel_path}"; then
      unresolved_lfs+=("${rel_path}")
    fi
  done

  if ((${#unresolved_lfs[@]} > 0)); then
    if [[ "${strict_lfs}" == "1" ]]; then
      fail "${label}: model file(s) still unresolved after git lfs pull: ${unresolved_lfs[*]}"
    fi
    warn "${label}: model file(s) remained as LFS pointers after pull (best-effort mode): ${unresolved_lfs[*]}"
  fi

  return 0
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
  ensure_relevant_model_lfs_hydrated "${post_dir}" "post variant for fixture '${id}'"

  log "  Verifying required integration files exist in post variant"
  for rel_path in "${strip_files[@]}"; do
    [[ -e "${post_dir}/${rel_path}" ]] || fail "fixture '${id}' missing '${rel_path}' in post variant"
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

  assert_clean_git_tree "${post_dir}" "post variant for fixture '${id}'"

  log "  Creating pre variant from source repository"
  git_fixture clone --quiet --no-checkout --filter=blob:none "${repo}" "${pre_dir}"
  git_fixture -C "${pre_dir}" checkout --quiet "${post_ref}"
  ensure_relevant_model_lfs_hydrated "${pre_dir}" "pre variant for fixture '${id}'"

  log "  Stripping pre-integration files from pre variant"
  for rel_path in "${strip_files[@]}"; do
    rm -rf -- "${pre_dir:?}/${rel_path}"
  done

  # Remove compiled artifacts that can leak stripped integration semantics.
  stripped_py_basenames=()
  for rel_path in "${strip_files[@]}"; do
    if [[ "${rel_path}" == *.py ]]; then
      stripped_py_basenames+=("$(basename "${rel_path}" .py)")
    fi
  done

  while IFS= read -r abs_path; do
    [[ -n "${abs_path}" ]] || continue
    rel_path="${abs_path#"${pre_dir}/"}"
    rm -f -- "${pre_dir:?}/${rel_path}"
  done < <(find "${pre_dir}" -type f -path '*/__pycache__/leap*.pyc' | sort)

  for base_name in "${stripped_py_basenames[@]}"; do
    while IFS= read -r abs_path; do
      [[ -n "${abs_path}" ]] || continue
      rel_path="${abs_path#"${pre_dir}/"}"
      rm -f -- "${pre_dir:?}/${rel_path}"
    done < <(find "${pre_dir}" -type f -path "*/__pycache__/${base_name}*.pyc" | sort)
  done

  find "${pre_dir}" -type d -name '__pycache__' -empty -delete

  remaining_pre_root_leap_files=()
  while IFS= read -r rel_path; do
    [[ -n "${rel_path}" ]] && remaining_pre_root_leap_files+=("${rel_path}")
  done < <(find "${pre_dir}" -maxdepth 1 -type f -name 'leap*' -exec basename {} \; | sort)

  ((${#remaining_pre_root_leap_files[@]} == 0)) \
    || fail "fixture '${id}': pre variant still has root leap files after stripping: ${remaining_pre_root_leap_files[*]}"

  pre_leap_pyc_files=()
  while IFS= read -r abs_path; do
    [[ -n "${abs_path}" ]] || continue
    pre_leap_pyc_files+=("${abs_path#"${pre_dir}/"}")
  done < <(find "${pre_dir}" -type f -path '*/__pycache__/leap*.pyc' | sort)
  ((${#pre_leap_pyc_files[@]} == 0)) \
    || fail "fixture '${id}': pre variant still has compiled leap artifacts after stripping: ${pre_leap_pyc_files[*]}"

  for base_name in "${stripped_py_basenames[@]}"; do
    pre_compiled_matches=()
    while IFS= read -r abs_path; do
      [[ -n "${abs_path}" ]] || continue
      pre_compiled_matches+=("${abs_path#"${pre_dir}/"}")
    done < <(find "${pre_dir}" -type f -path "*/__pycache__/${base_name}*.pyc" | sort)
    ((${#pre_compiled_matches[@]} == 0)) \
      || fail "fixture '${id}': pre variant still has compiled artifacts for stripped '${base_name}.py': ${pre_compiled_matches[*]}"
  done

  pre_code_loader_python_files=()
  while IFS= read -r rel_path; do
    [[ -n "${rel_path}" ]] || continue
    pre_code_loader_python_files+=("${rel_path}")
  done < <(collect_python_code_loader_files "${pre_dir}")
  ((${#pre_code_loader_python_files[@]} == 0)) \
    || fail "fixture '${id}': pre variant still contains Python files importing code_loader: ${pre_code_loader_python_files[*]}"

  pre_tensorleap_dirs=()
  while IFS= read -r rel_path; do
    [[ -n "${rel_path}" ]] || continue
    pre_tensorleap_dirs+=("${rel_path}")
  done < <(collect_tensorleap_folder_dirs "${pre_dir}")
  ((${#pre_tensorleap_dirs[@]} == 0)) \
    || fail "fixture '${id}': pre variant still contains Tensorleap directories: ${pre_tensorleap_dirs[*]}"

  pre_tensorleap_mapping_files=()
  while IFS= read -r rel_path; do
    [[ -n "${rel_path}" ]] || continue
    pre_tensorleap_mapping_files+=("${rel_path}")
  done < <(collect_tensorleap_mapping_files "${pre_dir}")
  ((${#pre_tensorleap_mapping_files[@]} == 0)) \
    || fail "fixture '${id}': pre variant still contains Tensorleap mapping files: ${pre_tensorleap_mapping_files[*]}"

  pre_tensorleap_files=()
  while IFS= read -r abs_path; do
    [[ -n "${abs_path}" ]] || continue
    pre_tensorleap_files+=("${abs_path#"${pre_dir}/"}")
  done < <(rg -n --ignore-case --files-with-matches "tensorleap" "${pre_dir}" || true)
  ((${#pre_tensorleap_files[@]} == 0)) \
    || fail "fixture '${id}': pre variant still contains files with 'tensorleap': ${pre_tensorleap_files[*]}"

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
