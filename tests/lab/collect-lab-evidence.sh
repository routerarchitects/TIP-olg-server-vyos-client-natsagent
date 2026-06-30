#!/usr/bin/env bash
set -euo pipefail

# Collect and normalize Phase 9 real-lab evidence artifacts.
# This script copies available lab outputs into ARTIFACT_DIR without collecting
# passwords, tokens, private keys, or raw secret environment values.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

ARTIFACT_DIR="${ARTIFACT_DIR:-${ROOT_DIR}/tests/lab/artifacts/manual-$(date +%Y%m%dT%H%M%SZ)}"
SOURCE_DIR="${SOURCE_DIR:-${ARTIFACT_DIR}}"

mkdir -p "${ARTIFACT_DIR}"

copy_if_present() {
  local name="$1"
  local src="${SOURCE_DIR}/${name}"
  local dst="${ARTIFACT_DIR}/${name}"

  if [[ -f "${src}" && "${src}" != "${dst}" ]]; then
    cp "${src}" "${dst}"
  fi
  if [[ ! -f "${dst}" ]]; then
    : > "${dst}"
  fi
}

copy_if_present "configure-status.jsonl"
copy_if_present "configure-result.jsonl"
copy_if_present "action-status.jsonl"
copy_if_present "action-result.jsonl"
copy_if_present "agent.log"
copy_if_present "vyos-before.txt"
copy_if_present "vyos-after.txt"
copy_if_present "state.json"
copy_if_present "commands-run.txt"

{
  echo "artifact_dir=${ARTIFACT_DIR}"
  echo "source_dir=${SOURCE_DIR}"
  echo "nats_url_set=$([[ -n "${NATS_URL:-}" ]] && echo true || echo false)"
  echo "vyos_target=${VYOS_TARGET:-${TARGET:-vyos}}"
  echo "vyos_host_set=$([[ -n "${VYOS_HOST:-}" ]] && echo true || echo false)"
  echo "vyos_user_set=$([[ -n "${VYOS_USER:-}" ]] && echo true || echo false)"
  echo "vyos_password_set=$([[ -n "${VYOS_PASSWORD:-}" ]] && echo true || echo false)"
  echo "vyos_ssh_key_set=$([[ -n "${VYOS_SSH_KEY:-}" ]] && echo true || echo false)"
  echo "state_path=${STATE_PATH:-}"
  echo "desired_config_file=${DESIRED_CONFIG_FILE:-}"
} > "${ARTIFACT_DIR}/environment-summary.txt"

if [[ ! -s "${ARTIFACT_DIR}/phase9-summary.md" ]]; then
  cat > "${ARTIFACT_DIR}/phase9-summary.md" <<EOF
# Real VyOS Lab Evidence Summary

Evidence directory: ${ARTIFACT_DIR}

Attach this directory, or an archived copy of it, to the PR or release notes
after a manual lab run. Review all files before upload and remove any
environment-specific data that should not leave the lab.
EOF
fi

echo "[PASS] Collected lab evidence artifacts in ${ARTIFACT_DIR}"
