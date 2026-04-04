#!/usr/bin/env bash
set -euo pipefail

fixture_fail() {
  echo "error: $*" >&2
  exit 1
}

fixture_add_local_exclude() {
  local repo_dir="$1"
  local pattern="$2"
  local exclude_file="${repo_dir}/.git/info/exclude"

  [[ -d "${repo_dir}/.git" ]] || fixture_fail "fixture repo is missing .git: ${repo_dir}"

  mkdir -p "$(dirname "${exclude_file}")"
  touch "${exclude_file}"
  grep -qxF "${pattern}" "${exclude_file}" || printf '%s\n' "${pattern}" >>"${exclude_file}"
}

fixture_list_stripped_python_basenames() {
  local rel_path
  for rel_path in "$@"; do
    if [[ "${rel_path}" == *.py ]]; then
      basename "${rel_path}" .py
    fi
  done
}

fixture_collect_declared_readme_files() {
  local repo_dir="$1"
  local pyproject_path="${repo_dir}/pyproject.toml"

  [[ -f "${pyproject_path}" ]] || return 0

  sed -nE 's/^[[:space:]]*readme[[:space:]]*=[[:space:]]*"([^"]+)".*/\1/p' "${pyproject_path}"
}

fixture_restore_declared_readmes() {
  local repo_dir="$1"
  local rel_path

  while IFS= read -r rel_path; do
    [[ -n "${rel_path}" ]] || continue
    [[ -e "${repo_dir}/${rel_path}" ]] && continue

    mkdir -p "$(dirname "${repo_dir}/${rel_path}")"
    cat >"${repo_dir}/${rel_path}" <<'EOF'
# Fixture Placeholder

This placeholder README keeps Poetry metadata valid for Concierge fixture runtime checks.
EOF
  done < <(fixture_collect_declared_readme_files "${repo_dir}")
}

fixture_commit_with_fixed_metadata() {
  local repo_dir="$1"
  local message="$2"

  GIT_AUTHOR_NAME="Concierge Fixture Bot" \
  GIT_AUTHOR_EMAIL="concierge-fixtures@local" \
  GIT_AUTHOR_DATE="2000-01-01T00:00:00Z" \
  GIT_COMMITTER_NAME="Concierge Fixture Bot" \
  GIT_COMMITTER_EMAIL="concierge-fixtures@local" \
  GIT_COMMITTER_DATE="2000-01-01T00:00:00Z" \
    git -C "${repo_dir}" -c commit.gpgsign=false commit --quiet -m "${message}"
}

fixture_strip_pre_variant_files() {
  local repo_dir="$1"
  shift

  local strip_files=("$@")
  local rel_path
  for rel_path in "${strip_files[@]}"; do
    rm -rf -- "${repo_dir:?}/${rel_path}"
  done

  local stripped_py_basenames=()
  while IFS= read -r base_name; do
    [[ -n "${base_name}" ]] || continue
    stripped_py_basenames+=("${base_name}")
  done < <(fixture_list_stripped_python_basenames "${strip_files[@]}")

  while IFS= read -r abs_path; do
    [[ -n "${abs_path}" ]] || continue
    rm -f -- "${abs_path}"
  done < <(find "${repo_dir}" -type f -path '*/__pycache__/leap*.pyc' | sort)

  local base_name
  for base_name in "${stripped_py_basenames[@]}"; do
    while IFS= read -r abs_path; do
      [[ -n "${abs_path}" ]] || continue
      rm -f -- "${abs_path}"
    done < <(find "${repo_dir}" -type f -path "*/__pycache__/${base_name}*.pyc" | sort)
  done

  find "${repo_dir}" -type d -name '__pycache__' -empty -delete
  fixture_restore_declared_readmes "${repo_dir}"
}

fixture_commit_pre_variant() {
  local repo_dir="$1"

  git -C "${repo_dir}" add -A
  git -C "${repo_dir}" diff --cached --quiet \
    && fixture_fail "fixture pre variant had no changes after stripping files: ${repo_dir}"

  fixture_commit_with_fixed_metadata "${repo_dir}" "Create pre-integration fixture variant"
}

fixture_reset_post_variant() {
  local repo_dir="$1"
  local post_ref="$2"

  git -C "${repo_dir}" reset --hard "${post_ref}" >/dev/null
  git -C "${repo_dir}" clean -fdx -e .fixture_reset.sh >/dev/null
}

fixture_reset_pre_variant() {
  local repo_dir="$1"
  local post_ref="$2"
  shift 2

  fixture_reset_post_variant "${repo_dir}" "${post_ref}"
  fixture_strip_pre_variant_files "${repo_dir}" "$@"
  fixture_commit_pre_variant "${repo_dir}"
}

fixture_apply_case_patch() {
  local repo_dir="$1"
  local patch_path="$2"
  local commit_message="$3"

  [[ -f "${patch_path}" ]] || fixture_fail "fixture case patch is missing: ${patch_path}"

  git -C "${repo_dir}" apply --whitespace=nowarn "${patch_path}"
  git -C "${repo_dir}" add -A
  git -C "${repo_dir}" diff --cached --quiet \
    && fixture_fail "fixture case patch had no effect: ${patch_path}"
  fixture_commit_with_fixed_metadata "${repo_dir}" "${commit_message}"
}

fixture_patch_sha256() {
  local patch_path="$1"

  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "${patch_path}" | awk '{print $1}'
    return
  fi
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "${patch_path}" | awk '{print $1}'
    return
  fi

  fixture_fail "required command 'shasum' or 'sha256sum' not found"
}

fixture_write_case_state_file() {
  local repo_dir="$1"
  local source_ref="$2"
  local patch_path="$3"
  local patch_sha256

  [[ -d "${repo_dir}" ]] || fixture_fail "fixture case repo missing: ${repo_dir}"
  [[ -f "${patch_path}" ]] || fixture_fail "fixture case patch is missing: ${patch_path}"

  patch_sha256="$(fixture_patch_sha256 "${patch_path}")"
  cat >"${repo_dir}/.fixture_case_state.json" <<EOF
{
  "source_ref": "${source_ref}",
  "patch_sha256": "${patch_sha256}"
}
EOF
}

fixture_reset_case_variant() {
  local repo_dir="$1"
  local source_ref="$2"
  local patch_path="$3"
  local commit_message="$4"

  fixture_reset_post_variant "${repo_dir}" "${source_ref}"
  fixture_write_case_state_file "${repo_dir}" "${source_ref}" "${patch_path}"
  fixture_apply_case_patch "${repo_dir}" "${patch_path}" "${commit_message}"
}

fixture_extract_leap_yaml_entry_file() {
  local leap_yaml_path="$1"

  [[ -f "${leap_yaml_path}" ]] || return 0

  sed -nE 's/^[[:space:]]*entryFile[[:space:]]*:[[:space:]]*"?([^"]+)"?.*/\1/p' "${leap_yaml_path}" | head -n 1
}

fixture_min_code_loader_version() {
  printf '%s\n' "${FIXTURE_MIN_CODE_LOADER_VERSION:-1.0.165}"
}

fixture_compare_versions() {
  local left="$1"
  local right="$2"

  python3 - "$left" "$right" <<'PY'
import re
import sys


def parse_version(text: str):
    match = re.search(r"(\d+)\.(\d+)\.(\d+)(?:[.\-]?dev(\d+))?", text)
    if not match:
        raise ValueError(f"unsupported version: {text}")
    major, minor, patch, dev = match.groups()
    return int(major), int(minor), int(patch), None if dev is None else int(dev)


left = parse_version(sys.argv[1])
right = parse_version(sys.argv[2])

left_core = left[:3]
right_core = right[:3]
if left_core < right_core:
    print(-1)
    raise SystemExit(0)
if left_core > right_core:
    print(1)
    raise SystemExit(0)

left_dev = left[3]
right_dev = right[3]
if left_dev is None and right_dev is None:
    print(0)
elif left_dev is None:
    print(1)
elif right_dev is None:
    print(-1)
elif left_dev < right_dev:
    print(-1)
elif left_dev > right_dev:
    print(1)
else:
    print(0)
PY
}

fixture_version_at_least() {
  local actual="$1"
  local minimum="$2"
  local comparison

  comparison="$(fixture_compare_versions "${actual}" "${minimum}")" \
    || fixture_fail "failed to compare versions '${actual}' and '${minimum}'"
  [[ "${comparison}" == "0" || "${comparison}" == "1" ]]
}

fixture_extract_poetry_lock_code_loader_version() {
  local repo_dir="$1"
  local poetry_lock_path="${repo_dir}/poetry.lock"

  [[ -f "${poetry_lock_path}" ]] || return 0

  awk '
    /^\[\[package\]\]$/ {
      in_package = 1
      package_name = ""
      next
    }
    in_package && /^name = "(code-loader|code_loader)"$/ {
      package_name = "code-loader"
      next
    }
    in_package && package_name == "code-loader" && /^version = "/ {
      gsub(/^version = "/, "", $0)
      gsub(/"$/, "", $0)
      print
      exit
    }
  ' "${poetry_lock_path}"
}

fixture_extract_requirements_code_loader_version() {
  local repo_dir="$1"
  local requirements_path
  local line

  while IFS= read -r requirements_path; do
    [[ -f "${requirements_path}" ]] || continue
    while IFS= read -r line; do
      if [[ "${line}" =~ ^[[:space:]]*code-loader[[:space:]]*==[[:space:]]*([A-Za-z0-9._-]+) ]]; then
        printf '%s\n' "${BASH_REMATCH[1]}"
        return 0
      fi
    done <"${requirements_path}"
  done < <(find "${repo_dir}" -maxdepth 2 -type f \( -name 'requirements.txt' -o -name 'requirements-*.txt' \) | sort)
}

fixture_extract_pyproject_code_loader_constraint() {
  local repo_dir="$1"
  local pyproject_path="${repo_dir}/pyproject.toml"

  [[ -f "${pyproject_path}" ]] || return 0

  python3 - "${pyproject_path}" <<'PY'
import re
import sys

path = sys.argv[1]
with open(path, "r", encoding="utf-8") as handle:
    lines = handle.readlines()

in_poetry_deps = False
for raw_line in lines:
    line = raw_line.strip()
    if line.startswith("["):
        in_poetry_deps = line == "[tool.poetry.dependencies]"
        continue
    if not in_poetry_deps:
        continue
    match = re.match(r'code-loader\s*=\s*"([^"]+)"', line)
    if match:
        print(match.group(1))
        raise SystemExit(0)
    match = re.match(r'code-loader\s*=\s*\{(.+)\}', line)
    if match:
        version_match = re.search(r'version\s*=\s*"([^"]+)"', match.group(1))
        if version_match:
            print(version_match.group(1))
            raise SystemExit(0)

for raw_line in lines:
    line = raw_line.strip()
    match = re.match(r'code-loader\s*=\s*"([^"]+)"', line)
    if match:
        print(match.group(1))
        raise SystemExit(0)
PY
}

fixture_extract_code_loader_version_token() {
  local text="$1"

  python3 - "${text}" <<'PY'
import re
import sys

match = re.search(r"(\d+\.\d+\.\d+(?:[.\-]?dev\d+)?)", sys.argv[1])
if match:
    print(match.group(1))
PY
}

fixture_update_pyproject_code_loader_constraint() {
  local repo_dir="$1"
  local new_constraint="$2"
  local pyproject_path="${repo_dir}/pyproject.toml"

  [[ -f "${pyproject_path}" ]] || fixture_fail "pyproject.toml not found for fixture repo: ${repo_dir}"

  python3 - "${pyproject_path}" "${new_constraint}" <<'PY'
from pathlib import Path
import re
import sys

path = Path(sys.argv[1])
constraint = sys.argv[2]
original = path.read_text(encoding="utf-8")
updated = re.sub(
    r'(^\s*code-loader\s*=\s*")([^"]+)(".*$)',
    rf'\g<1>{constraint}\g<3>',
    original,
    count=1,
    flags=re.MULTILINE,
)
if updated == original:
    updated = re.sub(
        r'(^\s*code-loader\s*=\s*\{[^}]*version\s*=\s*")([^"]+)(")',
        rf'\g<1>{constraint}\g<3>',
        original,
        count=1,
        flags=re.MULTILINE,
    )
if updated == original:
    raise SystemExit("failed to update code-loader constraint in pyproject.toml")
path.write_text(updated, encoding="utf-8")
PY
}

fixture_detect_code_loader_pin() {
  local repo_dir="$1"
  local version=""
  local source=""
  local constraint=""

  version="$(fixture_extract_poetry_lock_code_loader_version "${repo_dir}")"
  if [[ -n "${version}" ]]; then
    printf '%s|%s\n' "poetry.lock" "${version}"
    return 0
  fi

  version="$(fixture_extract_requirements_code_loader_version "${repo_dir}")"
  if [[ -n "${version}" ]]; then
    printf '%s|%s\n' "requirements.txt" "${version}"
    return 0
  fi

  constraint="$(fixture_extract_pyproject_code_loader_constraint "${repo_dir}")"
  if [[ -n "${constraint}" ]]; then
    version="$(fixture_extract_code_loader_version_token "${constraint}")"
    if [[ -n "${version}" ]]; then
      printf '%s|%s\n' "pyproject.toml" "${version}"
      return 0
    fi
  fi

  return 0
}

fixture_prepare_local_code_loader_pin() {
  local repo_dir="$1"
  local label="$2"
  local minimum_version
  local pin_info
  local version
  local target_constraint

  minimum_version="$(fixture_min_code_loader_version)"
  pin_info="$(fixture_detect_code_loader_pin "${repo_dir}")"
  [[ -n "${pin_info}" ]] \
    || fixture_fail "${label}: could not find a pinned code-loader version in poetry.lock, requirements*.txt, or pyproject.toml"

  version="${pin_info#*|}"
  if fixture_version_at_least "${version}" "${minimum_version}"; then
    return 0
  fi

  target_constraint="^${minimum_version}"
  fixture_update_pyproject_code_loader_constraint "${repo_dir}" "${target_constraint}"

  (
    cd "${repo_dir}"
    poetry lock >/dev/null
  ) || fixture_fail "${label}: failed to refresh poetry.lock after updating code-loader to ${target_constraint}"

  fixture_assert_min_code_loader_pin "${repo_dir}" "${label}"

  if [[ -n "$(git -C "${repo_dir}" status --porcelain)" ]]; then
    git -C "${repo_dir}" add pyproject.toml poetry.lock
    git -C "${repo_dir}" diff --cached --quiet || fixture_commit_with_fixed_metadata \
      "${repo_dir}" \
      "Prepare post-integration fixture variant"
  fi
}

fixture_assert_min_code_loader_pin() {
  local repo_dir="$1"
  local label="$2"
  local minimum_version
  local pin_info
  local source
  local version

  minimum_version="$(fixture_min_code_loader_version)"
  pin_info="$(fixture_detect_code_loader_pin "${repo_dir}")"
  [[ -n "${pin_info}" ]] \
    || fixture_fail "${label}: could not find a pinned code-loader version in poetry.lock, requirements*.txt, or pyproject.toml"

  source="${pin_info%%|*}"
  version="${pin_info#*|}"

  fixture_version_at_least "${version}" "${minimum_version}" || fixture_fail \
    "${label}: pinned code-loader ${version} from ${source} is below the minimum required ${minimum_version}; fixture prep requires code-loader validation output when leap_integration.py is executed as a Python script"
}

fixture_assert_min_installed_code_loader_version() {
  local repo_dir="$1"
  local label="$2"
  local minimum_version
  local installed_version

  minimum_version="$(fixture_min_code_loader_version)"
  installed_version="$(
    cd "${repo_dir}" && \
      POETRY_VIRTUALENVS_IN_PROJECT=true poetry run python -c \
        'from importlib import metadata as importlib_metadata; print(importlib_metadata.version("code-loader"))'
  )" || fixture_fail "${label}: failed to resolve installed code-loader version from Poetry environment"

  installed_version="$(printf '%s' "${installed_version}" | tr -d '[:space:]')"
  [[ -n "${installed_version}" ]] || fixture_fail "${label}: Poetry environment did not report an installed code-loader version"

  fixture_version_at_least "${installed_version}" "${minimum_version}" || fixture_fail \
    "${label}: installed code-loader ${installed_version} is below the minimum required ${minimum_version}; fixture bootstrap requires the validator-output-capable code-loader line"
}
