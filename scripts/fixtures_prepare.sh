#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
MANIFEST_PATH="${REPO_ROOT}/fixtures/manifest.json"
FIXTURES_ROOT="${REPO_ROOT}/.fixtures"
RESET_LIB_PATH="${REPO_ROOT}/scripts/fixtures_reset_lib.sh"
BOOTSTRAP_SCRIPT_PATH="${REPO_ROOT}/scripts/fixtures_bootstrap_poetry.sh"

# shellcheck source=./fixtures_reset_lib.sh
source "${RESET_LIB_PATH}"

usage() {
  cat <<'EOF'
Usage: bash scripts/fixtures_prepare.sh [--fixture <id>] [--bootstrap-poetry]

Prepare the pinned pre/post fixture repositories in .fixtures/.

Options:
  --fixture ID         Limit preparation to one fixture ID from fixtures/manifest.json.
  --bootstrap-poetry   After preparing fixtures, bootstrap Poetry environments explicitly.
  --help               Show this help text.
EOF
}

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

bootstrap_poetry=0
fixture_id=""
while (($# > 0)); do
  case "$1" in
    --fixture)
      shift
      [[ $# -gt 0 ]] || fail "--fixture requires a value"
      fixture_id="$1"
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

assert_guide_native_post_variant() {
  local repo_dir="$1"
  local label="$2"
  local leap_yaml_path="${repo_dir}/leap.yaml"
  local entry_file

  [[ -f "${leap_yaml_path}" ]] || fail "${label} is missing leap.yaml"
  entry_file="$(fixture_extract_leap_yaml_entry_file "${leap_yaml_path}")"
  [[ "${entry_file}" == "leap_integration.py" ]] \
    || fail "${label} must use leap_integration.py as leap.yaml entryFile, found '${entry_file:-<empty>}'"
  [[ -f "${repo_dir}/leap_integration.py" ]] || fail "${label} is missing leap_integration.py"
}

write_fixture_reset_script() {
  local repo_dir="$1"
  local variant_kind="$2"
  local post_ref="$3"
  shift 3

  local strip_files=("$@")
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
    printf 'POST_REF=%q\n' "${post_ref}"
    echo

    if [[ "${variant_kind}" == "pre" ]]; then
      echo 'STRIP_FILES=('
      local rel_path
      for rel_path in "${strip_files[@]}"; do
        printf '  %q\n' "${rel_path}"
      done
      echo ')'
      echo
      echo 'fixture_reset_pre_variant "${SCRIPT_DIR}" "${POST_REF}" "${STRIP_FILES[@]}"'
    else
      echo 'fixture_reset_post_variant "${SCRIPT_DIR}" "${POST_REF}"'
    fi
  } >"${script_path}"

  chmod +x "${script_path}"
  fixture_add_local_exclude "${repo_dir}" "/.fixture_reset.sh"
}

reset_fixture_dir() {
  local dir="$1"
  [[ -e "${dir}" ]] || return 0
  chmod -R u+w "${dir}" 2>/dev/null || true
  find "${dir}" -depth -delete
  [[ ! -e "${dir}" ]] || fail "failed to reset fixture directory: ${dir}"
}

require_cmd git
require_cmd jq
require_cmd poetry
require_cmd rg
require_cmd python3
[[ -f "${MANIFEST_PATH}" ]] || fail "manifest not found: ${MANIFEST_PATH}"
[[ -f "${RESET_LIB_PATH}" ]] || fail "fixture reset library not found: ${RESET_LIB_PATH}"
[[ -f "${BOOTSTRAP_SCRIPT_PATH}" ]] || fail "fixture bootstrap script not found: ${BOOTSTRAP_SCRIPT_PATH}"

jq -e '.fixtures and (.fixtures | type == "array")' "${MANIFEST_PATH}" >/dev/null \
  || fail "invalid manifest schema in ${MANIFEST_PATH}"

if [[ -n "${fixture_id}" ]]; then
  jq -e --arg id "${fixture_id}" '.fixtures[] | select(.id == $id)' "${MANIFEST_PATH}" >/dev/null \
    || fail "unknown fixture id '${fixture_id}' (see ${MANIFEST_PATH})"
fi

git_fixture() {
  GIT_LFS_SKIP_SMUDGE=1 git "$@"
}

mkdir -p "${FIXTURES_ROOT}"
log "Manifest: ${MANIFEST_PATH}"
log "Fixture output root: ${FIXTURES_ROOT}"
if [[ -n "${fixture_id}" ]]; then
  log "Target fixture: ${fixture_id}"
fi

manifest_filter='.fixtures[]'
if [[ -n "${fixture_id}" ]]; then
  manifest_filter='.fixtures[] | select(.id == $id)'
fi

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

  stripped_py_basenames=()
  while IFS= read -r base_name; do
    [[ -n "${base_name}" ]] || continue
    stripped_py_basenames+=("${base_name}")
  done < <(fixture_list_stripped_python_basenames "${strip_files[@]}")

  fixture_root="${FIXTURES_ROOT}/${id}"
  post_dir="${fixture_root}/post"
  pre_dir="${fixture_root}/pre"

  log "Preparing fixture '${id}'"
  log "  Source repository: ${repo}"
  log "  Pinned post_ref: ${post_ref}"
  log "  Resetting existing fixture directories"
  reset_fixture_dir "${post_dir}"
  reset_fixture_dir "${pre_dir}"
  mkdir -p "${fixture_root}"

  log "  Cloning post variant and checking out pinned commit"
  git_fixture clone --quiet --no-checkout --filter=blob:none "${repo}" "${post_dir}"
  git_fixture -C "${post_dir}" checkout --quiet "${post_ref}"
  ensure_relevant_model_lfs_hydrated "${post_dir}" "post variant for fixture '${id}'"
  assert_guide_native_post_variant "${post_dir}" "post variant for fixture '${id}'"
  post_pin_info="$(fixture_detect_code_loader_pin "${post_dir}")"
  post_pin_version="${post_pin_info#*|}"
  if ! fixture_version_at_least "${post_pin_version}" "$(fixture_min_code_loader_version)"; then
    log "  Refreshing local code-loader pin for post variant"
    fixture_prepare_local_code_loader_pin "${post_dir}" "post variant for fixture '${id}'"
  fi
  fixture_assert_min_code_loader_pin "${post_dir}" "post variant for fixture '${id}'"
  prepared_post_ref="$(git -C "${post_dir}" rev-parse HEAD)"
  write_fixture_reset_script "${post_dir}" post "${prepared_post_ref}"

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

  assert_clean_git_tree "${post_dir}" "post variant for fixture '${id}'"

  log "  Creating pre variant from source repository"
  git_fixture clone --quiet --no-checkout --filter=blob:none "${repo}" "${pre_dir}"
  git_fixture -C "${pre_dir}" checkout --quiet "${post_ref}"
  fixture_prepare_local_code_loader_pin "${pre_dir}" "pre variant source for fixture '${id}'"
  pre_source_ref="$(git -C "${pre_dir}" rev-parse HEAD)"
  [[ "${pre_source_ref}" == "${prepared_post_ref}" ]] \
    || fail "fixture '${id}': prepared pre source ref '${pre_source_ref}' does not match prepared post ref '${prepared_post_ref}'"
  write_fixture_reset_script "${pre_dir}" pre "${prepared_post_ref}" "${strip_files[@]}"

  log "  Stripping pre-integration files from pre variant"
  "${pre_dir}/.fixture_reset.sh"

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

  local_base_name=""
  for local_base_name in "${stripped_py_basenames[@]}"; do
    pre_compiled_matches=()
    while IFS= read -r abs_path; do
      [[ -n "${abs_path}" ]] || continue
      pre_compiled_matches+=("${abs_path#"${pre_dir}/"}")
    done < <(find "${pre_dir}" -type f -path "*/__pycache__/${local_base_name}*.pyc" | sort)
    ((${#pre_compiled_matches[@]} == 0)) \
      || fail "fixture '${id}': pre variant still has compiled artifacts for stripped '${local_base_name}.py': ${pre_compiled_matches[*]}"
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
  while IFS= read -r rel_path; do
    [[ -n "${rel_path}" ]] || continue
    pre_tensorleap_files+=("${rel_path}")
  done < <(collect_tensorleap_text_files "${pre_dir}")
  ((${#pre_tensorleap_files[@]} == 0)) \
    || fail "fixture '${id}': pre variant still contains files with 'tensorleap': ${pre_tensorleap_files[*]}"

  assert_clean_git_tree "${pre_dir}" "pre variant for fixture '${id}'"
  log "Prepared fixture '${id}' at ${fixture_root}"
done < <(jq -c --arg id "${fixture_id}" "${manifest_filter}" "${MANIFEST_PATH}")

if [[ "${bootstrap_poetry}" == "1" ]]; then
  log "Bootstrapping Poetry environments for prepared fixtures"
  bootstrap_args=(--variant all)
  if [[ -n "${fixture_id}" ]]; then
    bootstrap_args+=(--fixture "${fixture_id}")
  fi
  bash "${BOOTSTRAP_SCRIPT_PATH}" "${bootstrap_args[@]}"
fi

log "Fixture preparation complete"
