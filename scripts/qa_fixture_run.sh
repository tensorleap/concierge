#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
MANIFEST_PATH="${REPO_ROOT}/fixtures/manifest.json"
FIXTURES_ROOT="${REPO_ROOT}/.fixtures"
DOCKERFILE_PATH="${REPO_ROOT}/QA/docker/fixture.Dockerfile"

DEFAULT_CLAUDE_CODE_VERSION="${QA_CLAUDE_CODE_VERSION:-2.1.76}"

usage() {
  cat <<'EOF'
Usage: bash scripts/qa_fixture_run.sh [--repo <fixture-id>] [--image-mode <cold|prewarmed>] [-- <qa_loop args...>]

Reset a prepared built-in fixture, build an isolated Docker image from its clean pre state,
start a fixture container, and run the QA loop against that container.

Options:
  --repo <fixture-id>           Fixture ID from fixtures/manifest.json. Default: ultralytics
  --image-mode <cold|prewarmed> Container image mode. Default: cold
  --help                        Show this help text.
EOF
}

fail() {
  echo "error: $*" >&2
  exit 1
}

log() {
  echo "[qa-run] $*"
}

cleanup() {
  if [[ -n "${container_name:-}" ]]; then
    docker rm -f "${container_name}" >/dev/null 2>&1 || true
  fi
  if [[ -n "${tmpdir:-}" && -d "${tmpdir}" ]]; then
    rm -rf "${tmpdir}"
  fi
}

resolve_python_version() {
  local pyproject_path="$1"
  python3 - "$pyproject_path" <<'PY'
import re
import sys
import tomllib
from pathlib import Path

PATCH_MAP = {
    "3.11": "3.11.11",
    "3.10": "3.10.16",
    "3.9": "3.9.21",
    "3.8": "3.8.20",
}


def parse_version(value: str) -> tuple[int, int, int]:
    parts = [int(part) for part in value.split(".")]
    while len(parts) < 3:
        parts.append(0)
    return tuple(parts[:3])


def satisfies(candidate: tuple[int, int, int], token: str) -> bool:
    match = re.fullmatch(r"\s*(<=|>=|==|!=|<|>)\s*([0-9]+(?:\.[0-9]+){0,2})\s*", token)
    if not match:
        raise SystemExit(f"unsupported python constraint token: {token!r}")
    operator, version_text = match.groups()
    required = parse_version(version_text)
    return {
        "<": candidate < required,
        "<=": candidate <= required,
        ">": candidate > required,
        ">=": candidate >= required,
        "==": candidate == required,
        "!=": candidate != required,
    }[operator]


pyproject_path = Path(sys.argv[1])
data = tomllib.loads(pyproject_path.read_text(encoding="utf-8"))
constraint = str(data["tool"]["poetry"]["dependencies"]["python"]).strip()
tokens = [token.strip() for token in constraint.split(",") if token.strip()]
for minor in sorted(PATCH_MAP.keys(), reverse=True):
    exact = PATCH_MAP[minor]
    candidate = parse_version(exact)
    if all(satisfies(candidate, token) for token in tokens):
        print(exact)
        raise SystemExit(0)
raise SystemExit(f"no curated Python image satisfies constraint {constraint!r}")
PY
}

resolve_poetry_version() {
  local python_version="$1"
  local python_minor="${python_version%.*}"
  if [[ -n "${QA_POETRY_VERSION:-}" ]]; then
    printf '%s\n' "${QA_POETRY_VERSION}"
    return 0
  fi
  case "${python_minor}" in
    3.8)
      printf '1.8.5\n'
      ;;
    3.9|3.10|3.11)
      printf '2.2.1\n'
      ;;
    *)
      fail "no default Poetry version configured for Python ${python_minor}"
      ;;
  esac
}

hash_file() {
  local target_path="$1"
  python3 - "$target_path" <<'PY'
import hashlib
import sys
from pathlib import Path

path = Path(sys.argv[1])
digest = hashlib.sha256()
with path.open("rb") as handle:
    for chunk in iter(lambda: handle.read(1024 * 1024), b""):
        digest.update(chunk)
print(digest.hexdigest())
PY
}

compute_image_key() {
  python3 - "$@" <<'PY'
import hashlib
import json
import sys

payload = {
    "fixture_id": sys.argv[1],
    "fixture_ref": sys.argv[2],
    "python_version": sys.argv[3],
    "poetry_version": sys.argv[4],
    "claude_version": sys.argv[5],
    "concierge_sha": sys.argv[6],
    "image_mode": sys.argv[7],
    "dockerfile_sha": sys.argv[8],
    "runner_sha": sys.argv[9],
}
encoded = json.dumps(payload, sort_keys=True).encode("utf-8")
print(hashlib.sha256(encoded).hexdigest()[:16])
PY
}

fixture_id="${REPO:-ultralytics}"
image_mode="${QA_IMAGE_MODE:-cold}"
while (($# > 0)); do
  case "$1" in
    --repo)
      (($# >= 2)) || fail "--repo requires a fixture id"
      fixture_id="$2"
      shift 2
      ;;
    --image-mode)
      (($# >= 2)) || fail "--image-mode requires cold or prewarmed"
      image_mode="$2"
      shift 2
      ;;
    --help|-h)
      usage
      exit 0
      ;;
    --)
      shift
      break
      ;;
    *)
      break
      ;;
  esac
done

qa_args=("$@")

[[ "${image_mode}" == "cold" || "${image_mode}" == "prewarmed" ]] || fail "unsupported image mode '${image_mode}'"
[[ -n "${ANTHROPIC_API_KEY:-}" ]] || fail "ANTHROPIC_API_KEY is required for Docker QA runs"

[[ -f "${MANIFEST_PATH}" ]] || fail "fixture manifest not found: ${MANIFEST_PATH}"
[[ -f "${DOCKERFILE_PATH}" ]] || fail "fixture Dockerfile not found: ${DOCKERFILE_PATH}"

command -v docker >/dev/null 2>&1 || fail "required command 'docker' not found"
command -v git >/dev/null 2>&1 || fail "required command 'git' not found"
command -v go >/dev/null 2>&1 || fail "required command 'go' not found"
command -v jq >/dev/null 2>&1 || fail "required command 'jq' not found"
command -v python3 >/dev/null 2>&1 || fail "required command 'python3' not found"

jq -e --arg id "${fixture_id}" '.fixtures[] | select(.id == $id)' "${MANIFEST_PATH}" >/dev/null \
  || fail "unknown fixture id '${fixture_id}' (see ${MANIFEST_PATH})"

fixture_root="${FIXTURES_ROOT}/${fixture_id}"
pre_dir="${fixture_root}/pre"
post_dir="${fixture_root}/post"

if [[ ! -x "${pre_dir}/.fixture_reset.sh" || ! -x "${post_dir}/.fixture_reset.sh" ]]; then
  log "Preparing fixtures because '${fixture_id}' is not available locally yet"
  bash "${REPO_ROOT}/scripts/fixtures_prepare.sh"
fi

[[ -x "${pre_dir}/.fixture_reset.sh" ]] || fail "missing pre reset script for fixture '${fixture_id}': ${pre_dir}/.fixture_reset.sh"
[[ -x "${post_dir}/.fixture_reset.sh" ]] || fail "missing post reset script for fixture '${fixture_id}': ${post_dir}/.fixture_reset.sh"

log "Resetting fixture '${fixture_id}' post variant"
"${post_dir}/.fixture_reset.sh"
log "Resetting fixture '${fixture_id}' pre variant"
"${pre_dir}/.fixture_reset.sh"

if [[ -n "$(git -C "${pre_dir}" status --porcelain)" ]]; then
  fail "fixture pre variant is not clean after reset: ${pre_dir}"
fi
if [[ -n "$(git -C "${post_dir}" status --porcelain)" ]]; then
  fail "fixture post variant is not clean after reset: ${post_dir}"
fi

[[ -f "${pre_dir}/pyproject.toml" ]] || fail "fixture pre variant is missing pyproject.toml: ${pre_dir}"

python_version="$(resolve_python_version "${pre_dir}/pyproject.toml")"
poetry_version="$(resolve_poetry_version "${python_version}")"
docker_arch="$(docker version --format '{{.Server.Arch}}')"
case "${docker_arch}" in
  amd64|arm64)
    go_arch="${docker_arch}"
    ;;
  *)
    fail "unsupported Docker server arch '${docker_arch}'"
    ;;
esac

fixture_ref="$(git -C "${pre_dir}" rev-parse HEAD)"
safe_fixture_id="$(printf '%s' "${fixture_id}" | tr '[:upper:]_' '[:lower:]-')"

tmpdir="$(mktemp -d)"
trap cleanup EXIT

context_dir="${tmpdir}/context"
mkdir -p "${context_dir}/workspace" "${context_dir}/bin"

log "Copying clean fixture workspace into Docker build context"
cp -a "${pre_dir}/." "${context_dir}/workspace/"

log "Building Linux Concierge binary for ${go_arch}"
(
  cd "${REPO_ROOT}"
  CGO_ENABLED=0 GOOS=linux GOARCH="${go_arch}" go build -o "${context_dir}/bin/concierge" ./cmd/concierge
)
chmod +x "${context_dir}/bin/concierge"

concierge_sha="$(hash_file "${context_dir}/bin/concierge")"
dockerfile_sha="$(hash_file "${DOCKERFILE_PATH}")"
runner_sha="$(hash_file "${SCRIPT_DIR}/qa_fixture_run.sh")"
image_key="$(compute_image_key \
  "${fixture_id}" \
  "${fixture_ref}" \
  "${python_version}" \
  "${poetry_version}" \
  "${DEFAULT_CLAUDE_CODE_VERSION}" \
  "${concierge_sha}" \
  "${image_mode}" \
  "${dockerfile_sha}" \
  "${runner_sha}"
)"
image_ref="concierge-qa-${safe_fixture_id}:${image_mode}-py${python_version//./-}-${image_key}"
build_target="fixture-${image_mode}"

if docker image inspect "${image_ref}" >/dev/null 2>&1; then
  log "Reusing cached Docker image ${image_ref}"
else
  log "Building Docker image ${image_ref} (${build_target})"
  docker build \
    --file "${DOCKERFILE_PATH}" \
    --target "${build_target}" \
    --build-arg "PYTHON_IMAGE_TAG=${python_version}-slim" \
    --build-arg "POETRY_VERSION=${poetry_version}" \
    --build-arg "CLAUDE_CODE_VERSION=${DEFAULT_CLAUDE_CODE_VERSION}" \
    --build-arg "FIXTURE_ID=${fixture_id}" \
    --build-arg "FIXTURE_REF=${fixture_ref}" \
    --tag "${image_ref}" \
    "${context_dir}"
fi

container_name="concierge-qa-${safe_fixture_id}-$(date -u +%Y%m%d%H%M%S)-$$"
log "Starting fixture container ${container_name}"
docker run -d \
  --name "${container_name}" \
  --env "ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}" \
  "${image_ref}" >/dev/null

log "Starting QA loop for fixture '${fixture_id}'"
log "Fixture image: ${image_ref}"
log "Target container: ${container_name}"
log "Post fixture: ${post_dir}"

python3 "${REPO_ROOT}/QA/qa_loop.py" \
  --container-name "${container_name}" \
  --container-workdir /workspace \
  --container-image "${image_ref}" \
  --fixture-post-path "${post_dir}" \
  "${qa_args[@]}"
