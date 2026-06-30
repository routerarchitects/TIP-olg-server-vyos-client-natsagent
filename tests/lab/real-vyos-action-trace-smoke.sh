#!/usr/bin/env bash
set -euo pipefail

# Manual real-VyOS trace action smoke placeholder.
#
# The agent currently implements trace with the placeholder executor only.
# This script intentionally refuses to produce fake real-VyOS trace evidence.
# When a real platform trace executor exists, replace this guard with the
# NATS action submission and validation flow.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

ARTIFACT_DIR="${ARTIFACT_DIR:-${ROOT_DIR}/tests/lab/artifacts/real-vyos-action-trace-$(date +%Y%m%dT%H%M%SZ)}"
mkdir -p "${ARTIFACT_DIR}"

cat > "${ARTIFACT_DIR}/phase9-summary.md" <<'EOF'
# Real VyOS Action Trace Smoke

Status: Deferred

The real platform trace executor is not implemented yet. Placeholder trace
workflow is covered by unit and mocked integration tests, but that is not real
VyOS trace evidence.

Future work:

1. Implement a real VyOS trace executor.
2. Submit trace through the real NATS action path.
3. Capture action status/result JSONL artifacts.
4. Validate the returned trace evidence from the VyOS VM/device.
EOF

: > "${ARTIFACT_DIR}/action-status.jsonl"
: > "${ARTIFACT_DIR}/action-result.jsonl"
: > "${ARTIFACT_DIR}/commands-run.txt"

echo "[DEFERRED] Real VyOS trace action is not implemented yet."
echo "[INFO] Evidence placeholder written to ${ARTIFACT_DIR}"
exit 2
