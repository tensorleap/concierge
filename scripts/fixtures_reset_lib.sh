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

  GIT_AUTHOR_NAME="Concierge Fixture Bot" \
  GIT_AUTHOR_EMAIL="concierge-fixtures@local" \
  GIT_AUTHOR_DATE="2000-01-01T00:00:00Z" \
  GIT_COMMITTER_NAME="Concierge Fixture Bot" \
  GIT_COMMITTER_EMAIL="concierge-fixtures@local" \
  GIT_COMMITTER_DATE="2000-01-01T00:00:00Z" \
    git -C "${repo_dir}" -c commit.gpgsign=false commit --quiet -m "Create pre-integration fixture variant"
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
