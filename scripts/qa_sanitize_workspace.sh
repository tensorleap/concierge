#!/usr/bin/env bash
set -euo pipefail

usage() {
  cat <<'EOF'
Usage: bash scripts/qa_sanitize_workspace.sh <source-repo> <output-dir>

Copy the tracked working tree from <source-repo> into <output-dir>, then
reinitialize it as a single-commit detached git repo with no remotes.
EOF
}

fail() {
  echo "error: $*" >&2
  exit 1
}

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  usage
  exit 0
fi

(($# == 2)) || fail "expected <source-repo> and <output-dir>"

source_repo="$(cd -- "$1" && pwd)"
output_dir="$2"

git -C "${source_repo}" rev-parse --verify HEAD >/dev/null 2>&1 \
  || fail "source repo is not a git checkout with a valid HEAD: ${source_repo}"

rm -rf "${output_dir}"
mkdir -p "${output_dir}"

git -C "${source_repo}" archive --format=tar HEAD | tar -xf - -C "${output_dir}"

git -C "${output_dir}" init --quiet
git -C "${output_dir}" add -A

GIT_AUTHOR_NAME="Concierge QA" \
GIT_AUTHOR_EMAIL="qa@example.com" \
GIT_AUTHOR_DATE="2000-01-01T00:00:00Z" \
GIT_COMMITTER_NAME="Concierge QA" \
GIT_COMMITTER_EMAIL="qa@example.com" \
GIT_COMMITTER_DATE="2000-01-01T00:00:00Z" \
  git -C "${output_dir}" -c commit.gpgsign=false commit --quiet -m "Create sanitized QA workspace baseline"

git -C "${output_dir}" checkout --quiet --detach HEAD
