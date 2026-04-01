#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
MANIFEST_PATH="${REPO_ROOT}/fixtures/manifest.json"
CHECKPOINT_MANIFEST_PATH="${REPO_ROOT}/fixtures/checkpoints/manifest.json"
FIXTURES_ROOT="${REPO_ROOT}/.fixtures"
DOCKERFILE_PATH="${REPO_ROOT}/QA/docker/fixture.Dockerfile"
CHECKPOINT_RESOLVER_PATH="${REPO_ROOT}/scripts/qa_checkpoint_resolver.py"
SANITIZER_PATH="${REPO_ROOT}/scripts/qa_sanitize_workspace.sh"

DEFAULT_CLAUDE_CODE_VERSION="${QA_CLAUDE_CODE_VERSION:-2.1.76}"

usage() {
  cat <<'EOF'
Usage: bash scripts/qa_fixture_run.sh [--repo <fixture-id>] [--step <checkpoint-step>] [--require-explicit-setup] [-- <qa_loop args...>]

Resolve a deterministic fixture checkpoint, build an isolated Docker image from that clean state,
start a fixture container, and run the QA loop against that container.

Options:
  --repo <fixture-id>           Fixture ID from fixtures/manifest.json.
  --step <checkpoint-step>      Guide-native checkpoint step selector, or 'pre' for the clean pre repo.
  --require-explicit-setup      Fail fast instead of preparing fixture/case repos implicitly.
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
  python3 "${REPO_ROOT}/scripts/qa_python_runtime.py" resolve-python-version --pyproject "${pyproject_path}"
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

resolve_checkpoint_json() {
  local fixture_id="$1"
  local step="$2"
  local build_mode_override="${3:-}"
  local args=(
    "${CHECKPOINT_RESOLVER_PATH}"
    resolve
    --repo-root "${REPO_ROOT}"
    --fixture-id "${fixture_id}"
    --step "${step}"
  )
  if [[ -n "${build_mode_override}" ]]; then
    args+=(--build-mode-override "${build_mode_override}")
  fi
  python3 "${args[@]}"
}

stage_runtime_prerequisites_json() {
  local fixture_id="$1"
  local stage_root="$2"
  local backend="$3"
  local args=(
    "${CHECKPOINT_RESOLVER_PATH}"
    stage-runtime-prerequisites
    --repo-root "${REPO_ROOT}"
    --fixture-id "${fixture_id}"
    --stage-root "${stage_root}"
    --backend "${backend}"
  )
  python3 "${args[@]}"
}

compute_image_key() {
  python3 "${CHECKPOINT_RESOLVER_PATH}" image-key "$@"
}

select_runner_json() {
  local interactive_flag="$1"
  local args=(
    "${CHECKPOINT_RESOLVER_PATH}"
    select-runner
    --repo-root "${REPO_ROOT}"
  )
  if [[ -n "${fixture_id}" ]]; then
    args+=(--fixture-id "${fixture_id}")
  fi
  if [[ -n "${step}" ]]; then
    args+=(--step "${step}")
  fi
  if [[ "${interactive_flag}" == "true" ]]; then
    args+=(--interactive)
  fi
  python3 "${args[@]}"
}

fixture_id="${REPO:-}"
step="${QA_STEP:-}"
image_mode_override="${QA_IMAGE_MODE:-}"
require_explicit_setup="${QA_REQUIRE_EXPLICIT_SETUP:-0}"
while (($# > 0)); do
  case "$1" in
    --repo)
      (($# >= 2)) || fail "--repo requires a fixture id"
      fixture_id="$2"
      shift 2
      ;;
    --step)
      (($# >= 2)) || fail "--step requires a checkpoint step"
      step="$2"
      shift 2
      ;;
    --image-mode)
      (($# >= 2)) || fail "--image-mode requires cold or prewarmed"
      image_mode_override="$2"
      shift 2
      ;;
    --require-explicit-setup)
      require_explicit_setup=1
      shift
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

[[ -f "${MANIFEST_PATH}" ]] || fail "fixture manifest not found: ${MANIFEST_PATH}"
[[ -f "${CHECKPOINT_MANIFEST_PATH}" ]] || fail "checkpoint manifest not found: ${CHECKPOINT_MANIFEST_PATH}"
[[ -f "${DOCKERFILE_PATH}" ]] || fail "fixture Dockerfile not found: ${DOCKERFILE_PATH}"
[[ -f "${CHECKPOINT_RESOLVER_PATH}" ]] || fail "checkpoint resolver not found: ${CHECKPOINT_RESOLVER_PATH}"
[[ -f "${SANITIZER_PATH}" ]] || fail "workspace sanitizer not found: ${SANITIZER_PATH}"

command -v python3 >/dev/null 2>&1 || fail "required command 'python3' not found"
command -v jq >/dev/null 2>&1 || fail "required command 'jq' not found"

interactive_selection="false"
if [[ ( -z "${fixture_id}" || -z "${step}" ) && -t 0 && -t 1 ]]; then
  interactive_selection="true"
fi
selection_json="$(select_runner_json "${interactive_selection}")"
fixture_id="$(jq -r '.fixture_id' <<<"${selection_json}")"
step="$(jq -r '.step' <<<"${selection_json}")"

if [[ -n "${image_mode_override}" && "${image_mode_override}" != "cold" && "${image_mode_override}" != "prewarmed" ]]; then
  fail "unsupported image mode override '${image_mode_override}'"
fi

jq -e --arg id "${fixture_id}" '.fixtures[] | select(.id == $id)' "${MANIFEST_PATH}" >/dev/null \
  || fail "unknown fixture id '${fixture_id}' (see ${MANIFEST_PATH})"

fixture_root="${FIXTURES_ROOT}/${fixture_id}"
pre_dir="${fixture_root}/pre"
post_dir="${fixture_root}/post"
resolution_json="$(resolve_checkpoint_json "${fixture_id}" "${step}" "${image_mode_override}")"
selected_repo_dir="$(jq -r '.repo_path' <<<"${resolution_json}")"
selected_source_kind="$(jq -r '.source_kind' <<<"${resolution_json}")"
selected_source_id="$(jq -r '.source_id' <<<"${resolution_json}")"
selected_build_mode="$(jq -r '.build_mode' <<<"${resolution_json}")"
selected_warmup_script="$(jq -r '.warmup_script // empty' <<<"${resolution_json}")"
selected_prepare_case_id="$(jq -r '.prepare_case_id // empty' <<<"${resolution_json}")"
checkpoint_key="$(jq -r '.checkpoint_key' <<<"${resolution_json}")"

if [[ "${require_explicit_setup}" == "1" ]]; then
  if [[ ! -x "${pre_dir}/.fixture_reset.sh" || ! -x "${post_dir}/.fixture_reset.sh" ]]; then
    fail "explicit QA setup required: selected fixture '${fixture_id}' is not prepared. Run: bash scripts/fixtures_prepare.sh --fixture \"${fixture_id}\""
  fi

  if [[ -n "${selected_prepare_case_id}" && ! -x "${selected_repo_dir}/.fixture_reset.sh" ]]; then
    fail "explicit QA setup required: selected checkpoint '${checkpoint_key}' is not prepared. Run: bash scripts/fixtures_mutate_cases.sh --case \"${selected_prepare_case_id}\""
  fi
else
  if [[ ! -x "${pre_dir}/.fixture_reset.sh" || ! -x "${post_dir}/.fixture_reset.sh" ]]; then
    log "Preparing fixtures because '${fixture_id}' is not available locally yet"
    bash "${REPO_ROOT}/scripts/fixtures_prepare.sh" --fixture "${fixture_id}"
  fi

  if [[ -n "${selected_prepare_case_id}" && ! -x "${selected_repo_dir}/.fixture_reset.sh" ]]; then
    log "Generating fixture cases because checkpoint '${checkpoint_key}' is not available locally yet"
    bash "${REPO_ROOT}/scripts/fixtures_mutate_cases.sh" --case "${selected_prepare_case_id}"
  fi
fi

[[ -x "${pre_dir}/.fixture_reset.sh" ]] || fail "missing pre reset script for fixture '${fixture_id}': ${pre_dir}/.fixture_reset.sh"
[[ -x "${post_dir}/.fixture_reset.sh" ]] || fail "missing post reset script for fixture '${fixture_id}': ${post_dir}/.fixture_reset.sh"
[[ -x "${selected_repo_dir}/.fixture_reset.sh" ]] || fail "missing reset script for checkpoint '${checkpoint_key}': ${selected_repo_dir}/.fixture_reset.sh"

[[ -n "${ANTHROPIC_API_KEY:-}" ]] || fail "ANTHROPIC_API_KEY is required for Docker QA runs"

command -v docker >/dev/null 2>&1 || fail "required command 'docker' not found"
command -v git >/dev/null 2>&1 || fail "required command 'git' not found"
command -v go >/dev/null 2>&1 || fail "required command 'go' not found"

log "Resetting fixture '${fixture_id}' post variant"
"${post_dir}/.fixture_reset.sh"
log "Resetting checkpoint '${checkpoint_key}' from ${selected_source_kind}:${selected_source_id}"
"${selected_repo_dir}/.fixture_reset.sh"

if [[ -n "$(git -C "${selected_repo_dir}" status --porcelain)" ]]; then
  fail "selected checkpoint repo is not clean after reset: ${selected_repo_dir}"
fi
if [[ -n "$(git -C "${post_dir}" status --porcelain)" ]]; then
  fail "fixture post variant is not clean after reset: ${post_dir}"
fi

[[ -f "${selected_repo_dir}/pyproject.toml" ]] || fail "selected checkpoint repo is missing pyproject.toml: ${selected_repo_dir}"

python_version="$(resolve_python_version "${selected_repo_dir}/pyproject.toml")"
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

fixture_ref="$(git -C "${selected_repo_dir}" rev-parse HEAD)"
concierge_git_sha="$(git -C "${REPO_ROOT}" rev-parse HEAD)"
concierge_branch="$(git -C "${REPO_ROOT}" rev-parse --abbrev-ref HEAD)"
qa_ref_under_test="${concierge_git_sha}"
if [[ "${concierge_branch}" != "HEAD" ]]; then
  qa_ref_under_test="${concierge_branch}@${concierge_git_sha:0:12}"
fi
safe_fixture_id="$(printf '%s' "${fixture_id}" | tr '[:upper:]_' '[:lower:]-')"

tmpdir="$(mktemp -d)"
trap cleanup EXIT

context_dir="${tmpdir}/context"
mkdir -p "${context_dir}/workspace" "${context_dir}/bin"
runtime_prereq_stage_root="${tmpdir}/runtime-prerequisites"

runtime_prereq_backend="local"
if [[ "${GITHUB_ACTIONS:-}" == "true" ]]; then
  runtime_prereq_backend="github_actions"
fi
staged_runtime_prereqs_json="$(stage_runtime_prerequisites_json "${fixture_id}" "${runtime_prereq_stage_root}" "${runtime_prereq_backend}")"
selected_runtime_prereqs_json="$(jq -c '.runtime_prerequisites // []' <<<"${staged_runtime_prereqs_json}")"
runtime_prereq_count="$(jq -r '.runtime_prerequisites | length' <<<"${staged_runtime_prereqs_json}")"
runtime_prereq_mount_source="$(jq -r '.stage_root' <<<"${staged_runtime_prereqs_json}")"
runtime_prereq_mount_target="$(jq -r '.docker_mount_target' <<<"${staged_runtime_prereqs_json}")"
if (( runtime_prereq_count > 0 )); then
  log "Staged ${runtime_prereq_count} runtime prerequisite(s) for backend '${runtime_prereq_backend}'"
  while IFS= read -r line; do
    [[ -n "${line}" ]] || continue
    log "  ${line}"
  done < <(
    jq -r '.runtime_prerequisites[] | "\(.id) -> \(.mount_path) [\(.resolution_source)]"' \
      <<<"${staged_runtime_prereqs_json}"
  )
fi

log "Copying checkpoint workspace into Docker build context"
bash "${SANITIZER_PATH}" "${selected_repo_dir}" "${context_dir}/workspace"
warmup_sha=""
if [[ -n "${selected_warmup_script}" ]]; then
  [[ -f "${selected_warmup_script}" ]] || fail "checkpoint warmup script not found: ${selected_warmup_script}"
  cp "${selected_warmup_script}" "${context_dir}/workspace/.checkpoint_warmup.sh"
  chmod +x "${context_dir}/workspace/.checkpoint_warmup.sh"
  warmup_sha="$(hash_file "${selected_warmup_script}")"
fi

log "Building Linux Concierge binary for ${go_arch}"
(
  cd "${REPO_ROOT}"
  CGO_ENABLED=0 GOOS=linux GOARCH="${go_arch}" go build -o "${context_dir}/bin/concierge" ./cmd/concierge
)
chmod +x "${context_dir}/bin/concierge"

concierge_sha="$(hash_file "${context_dir}/bin/concierge")"
dockerfile_sha="$(hash_file "${DOCKERFILE_PATH}")"
runner_sha="$(hash_file "${SCRIPT_DIR}/qa_fixture_run.sh")"
sanitizer_sha="$(hash_file "${SANITIZER_PATH}")"
resolver_sha="$(hash_file "${CHECKPOINT_RESOLVER_PATH}")"
image_key="$(compute_image_key \
  --fixture-id "${fixture_id}" \
  --checkpoint-key "${checkpoint_key}" \
  --requested-step "${step}" \
  --source-kind "${selected_source_kind}" \
  --source-id "${selected_source_id}" \
  --fixture-ref "${fixture_ref}" \
  --python-version "${python_version}" \
  --poetry-version "${poetry_version}" \
  --claude-version "${DEFAULT_CLAUDE_CODE_VERSION}" \
  --concierge-sha "${concierge_sha}" \
  --build-mode "${selected_build_mode}" \
  --dockerfile-sha "${dockerfile_sha}" \
  --runner-sha "${runner_sha}" \
  --sanitizer-sha "${sanitizer_sha}" \
  --resolver-sha "${resolver_sha}" \
  --warmup-sha "${warmup_sha}"
)"
image_ref="concierge-qa-${safe_fixture_id}:${selected_build_mode}-py${python_version//./-}-${image_key}"
build_target="fixture-${selected_build_mode}"

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
docker_run_args=(
  docker run -d
  --name "${container_name}"
  --env "ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}"
)
if (( runtime_prereq_count > 0 )); then
  docker_run_args+=(
    --mount
    "type=bind,src=${runtime_prereq_mount_source},dst=${runtime_prereq_mount_target},readonly"
  )
fi
docker_run_args+=("${image_ref}")
"${docker_run_args[@]}" >/dev/null

log "Starting QA loop for fixture '${fixture_id}'"
log "Requested step: ${step}"
log "Resolved checkpoint: ${checkpoint_key} -> ${selected_source_kind}:${selected_source_id}"
log "Fixture image: ${image_ref}"
log "Target container: ${container_name}"
log "Post fixture: ${post_dir}"

python3 "${REPO_ROOT}/QA/qa_loop.py" \
  --container-name "${container_name}" \
  --container-workdir /workspace \
  --container-image "${image_ref}" \
  --fixture-post-path "${post_dir}" \
  --fixture-id "${fixture_id}" \
  --guide-step "${step}" \
  --ref-under-test "${qa_ref_under_test}" \
  --checkpoint-key "${checkpoint_key}" \
  --source-kind "${selected_source_kind}" \
  --source-id "${selected_source_id}" \
  --runtime-prerequisites-json "${selected_runtime_prereqs_json}" \
  "${qa_args[@]}"
