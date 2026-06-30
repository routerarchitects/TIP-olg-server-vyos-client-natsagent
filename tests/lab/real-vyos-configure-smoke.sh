#!/usr/bin/env bash
set -euo pipefail

# Real-VyOS configure smoke helper.
#
# This script is lab-only. It assumes NATS and vyos-nats-agent are already
# running and focuses on configure submission, verification, and evidence
# collection.

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
cd "$ROOT_DIR"

usage() {
  cat <<'EOF'
Usage:
  ./tests/lab/real-vyos-configure-smoke.sh
  ./tests/lab/real-vyos-configure-smoke.sh --help

Purpose:
  Run the Phase 9 real VyOS configure smoke through the real NATS + JetStream
  path, verify VyOS config/state evidence, and optionally resubmit the same UUID
  to prove the already-in-sync path.

Manual prerequisites:
  1. Start NATS manually on the Ubuntu host, for example:
       nats-server -js -p 4222
  2. Install/start vyos-nats-agent manually inside the VyOS VM, for example:
       sudo install -m 0755 ~/vyos-nats-agent /usr/local/bin/vyos-nats-agent
       nohup /usr/local/bin/vyos-nats-agent --config ~/vyos-nats-agent.yaml >/tmp/vyos-nats-agent.log 2>&1 &

Required environment:
  REAL_VYOS_LAB_ACK=I_UNDERSTAND
  NATS_URL=nats://<host>:4222
  VYOS_TARGET=vyos
  VYOS_HOST=<host-or-ip>
  VYOS_USER=vyos
  STATE_PATH=/tmp/vyos-nats-agent/state.json
  Set exactly one of:
    VYOS_PASSWORD=<password>
    VYOS_SSH_KEY=/path/to/private/key

Optional environment:
  DESIRED_CONFIG_FILE=tests/lab/configs/desired-vyos-wan-only-config.json
  ARTIFACT_DIR=tests/lab/artifacts/manual-run
  CONFIG_UUID=cfg-lab-<timestamp>
  RPC_ID=real-vyos-configure-<timestamp>
  TIMEOUT=120s
  RESUBMIT_SAME_UUID=true
  EXPECTED_VYOS_MATCH=OLG_APPLY_SMOKE_TEST
  REMOTE_AGENT_LOG=/tmp/vyos-nats-agent.log
  VYOS_SHOW_CONFIG_COMMAND="show configuration commands"
  KEEP_WORK_DIR=false

Mode:
  Manual dependency mode only:
    Start NATS and vyos-nats-agent yourself, then run the script.

Examples:
  Manual mode:
    export REAL_VYOS_LAB_ACK=I_UNDERSTAND
    export NATS_URL=nats://192.168.76.69:4222
    export VYOS_TARGET=vyos
    export VYOS_HOST=192.168.76.2
    export VYOS_USER=vyos
    export VYOS_PASSWORD=vyos
    export STATE_PATH=/tmp/vyos-nats-agent/state.json
    export REMOTE_AGENT_LOG=/tmp/vyos-nats-agent.log
    export DESIRED_CONFIG_FILE=tests/lab/configs/desired-vyos-wan-only-config.json
    export ARTIFACT_DIR=tests/lab/artifacts/manual-run-001
    export VYOS_SHOW_CONFIG_COMMAND="/opt/vyatta/bin/vyatta-op-cmd-wrapper show configuration commands"
    ./tests/lab/real-vyos-configure-smoke.sh

Artifact outputs:
  phase9-summary.md
  configure-status.jsonl
  configure-result.jsonl
  controller.log
  agent.log
  vyos-before.txt
  vyos-after.txt
  state.json
  commands-run.txt
  environment-summary.txt

Secret safety:
  The script does not enable shell tracing, does not print passwords, and does
  not write secret values to commands-run.txt or environment-summary.txt.
  It copies the remote agent log into agent.log after configure checks finish
  and attempts best-effort log collection on failures too. Review and sanitize
  artifacts before sharing them outside the lab.
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "[FAIL] unknown argument: $1" >&2
      echo >&2
      usage >&2
      exit 1
      ;;
  esac
done

REAL_VYOS_LAB_ACK="${REAL_VYOS_LAB_ACK:-}"
NATS_URL="${NATS_URL:-}"
VYOS_TARGET="${VYOS_TARGET:-vyos}"
TARGET="${TARGET:-${VYOS_TARGET}}"
VYOS_HOST="${VYOS_HOST:-}"
VYOS_USER="${VYOS_USER:-}"
VYOS_PASSWORD="${VYOS_PASSWORD:-}"
VYOS_SSH_KEY="${VYOS_SSH_KEY:-}"
STATE_PATH="${STATE_PATH:-}"
DESIRED_CONFIG_FILE="${DESIRED_CONFIG_FILE:-${ROOT_DIR}/tests/lab/configs/desired-vyos-wan-only-config.json}"
PAYLOAD_FILE="${PAYLOAD_FILE:-${DESIRED_CONFIG_FILE}}"
ARTIFACT_DIR="${ARTIFACT_DIR:-${ROOT_DIR}/tests/lab/artifacts/real-vyos-configure-$(date +%Y%m%dT%H%M%SZ)}"
CONFIG_UUID="${CONFIG_UUID:-cfg-lab-$(date +%s)-$$}"
RPC_ID="${RPC_ID:-real-vyos-configure-$(date +%s)-$$}"
TIMEOUT="${TIMEOUT:-120s}"
RESUBMIT_SAME_UUID="${RESUBMIT_SAME_UUID:-true}"
EXPECTED_VYOS_MATCH="${EXPECTED_VYOS_MATCH:-OLG_APPLY_SMOKE_TEST}"
REMOTE_AGENT_LOG="${REMOTE_AGENT_LOG:-/tmp/vyos-nats-agent.log}"
VYOS_SHOW_CONFIG_COMMAND="${VYOS_SHOW_CONFIG_COMMAND:-show configuration commands}"
KEEP_WORK_DIR="${KEEP_WORK_DIR:-false}"

mkdir -p "${ROOT_DIR}/.tmp"
WORK_DIR="$(mktemp -d "${ROOT_DIR}/.tmp/vyos-nats-agent-real-lab-XXXXXX")"
TMP_CONFIG="${WORK_DIR}/controller-config.yaml"
# Make WORK_DIR path absolute so relative path context is preserved
WORK_DIR="$(cd "${WORK_DIR}" && pwd)"
TMP_CONFIG="${WORK_DIR}/controller-config.yaml"
CONTROLLER_DIR="${WORK_DIR}/controller"
CONTROLLER_LOG="${ARTIFACT_DIR}/controller.log"
AGENT_LOG="${ARTIFACT_DIR}/agent.log"


cleanup() {
  set +e
  if [[ "${KEEP_WORK_DIR}" != "true" ]]; then
    rm -rf "${WORK_DIR}"
  else
    echo "[INFO] kept lab work dir at ${WORK_DIR}"
  fi
}
trap cleanup EXIT

fail() {
  collect_remote_agent_log_best_effort
  echo "[FAIL] $*" >&2
  echo "" >&2
  echo "Artifacts: ${ARTIFACT_DIR}" >&2
  echo "" >&2
  echo "---- controller log ----" >&2
  [[ -f "${CONTROLLER_LOG}" ]] && tail -n 260 "${CONTROLLER_LOG}" >&2 || true
  echo "" >&2
  echo "---- agent log ----" >&2
  [[ -f "${AGENT_LOG}" ]] && tail -n 260 "${AGENT_LOG}" >&2 || true
  exit 1
}

require_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    fail "required command not found: $1"
  fi
}

record_command() {
  printf '%s\n' "$*" >> "${ARTIFACT_DIR}/commands-run.txt"
}

write_environment_summary() {
  {
    echo "real_vyos_lab_ack_set=$([[ "${REAL_VYOS_LAB_ACK}" == "I_UNDERSTAND" ]] && echo true || echo false)"
    echo "nats_url_set=$([[ -n "${NATS_URL}" ]] && echo true || echo false)"
    echo "vyos_target=${TARGET}"
    echo "vyos_host_set=$([[ -n "${VYOS_HOST}" ]] && echo true || echo false)"
    echo "vyos_user_set=$([[ -n "${VYOS_USER}" ]] && echo true || echo false)"
    echo "vyos_password_set=$([[ -n "${VYOS_PASSWORD}" ]] && echo true || echo false)"
    echo "vyos_ssh_key_set=$([[ -n "${VYOS_SSH_KEY}" ]] && echo true || echo false)"
    echo "state_path=${STATE_PATH}"
    echo "desired_config_file=${PAYLOAD_FILE}"
    echo "artifact_dir=${ARTIFACT_DIR}"
    echo "config_uuid=${CONFIG_UUID}"
    echo "rpc_id=${RPC_ID}"
    echo "resubmit_same_uuid=${RESUBMIT_SAME_UUID}"
    echo "expected_vyos_match=${EXPECTED_VYOS_MATCH}"
    echo "remote_agent_log=${REMOTE_AGENT_LOG}"
  } > "${ARTIFACT_DIR}/environment-summary.txt"
}

ssh_cmd() {
  local remote_cmd="$1"
  local -a base_args=(-o StrictHostKeyChecking=accept-new -o UserKnownHostsFile="${ARTIFACT_DIR}/known_hosts")

  record_command "ssh ${VYOS_USER}@${VYOS_HOST} '${remote_cmd}'"

  if [[ -n "${VYOS_SSH_KEY}" ]]; then
    ssh "${base_args[@]}" -i "${VYOS_SSH_KEY}" "${VYOS_USER}@${VYOS_HOST}" "${remote_cmd}"
    return
  fi

  SSHPASS="${VYOS_PASSWORD}" sshpass -e ssh "${base_args[@]}" "${VYOS_USER}@${VYOS_HOST}" "${remote_cmd}"
}

validate_no_single_quotes() {
  local name="$1"
  local value="$2"
  if [[ "${value}" == *"'"* ]]; then
    fail "${name} must not contain single quotes"
  fi
}

copy_state_artifact() {
  local out="$1"
  if ! ssh_cmd "cat '${STATE_PATH}'" > "${out}" 2>>"${ARTIFACT_DIR}/ssh-errors.log"; then
    fail "could not read state file from VyOS host at STATE_PATH"
  fi
}

collect_remote_agent_log_best_effort() {
  mkdir -p "${ARTIFACT_DIR}" >/dev/null 2>&1 || true
  if ! ssh_cmd "cat '${REMOTE_AGENT_LOG}'" > "${AGENT_LOG}" 2>>"${ARTIFACT_DIR}/ssh-errors.log"; then
    echo "[WARN] could not collect remote agent log from ${REMOTE_AGENT_LOG}" >&2
  fi
}

if [[ "${REAL_VYOS_LAB_ACK}" != "I_UNDERSTAND" ]]; then
  fail "refusing to run real VyOS apply smoke without REAL_VYOS_LAB_ACK=I_UNDERSTAND"
fi
if [[ -z "${NATS_URL}" ]]; then
  fail "NATS_URL is required"
fi
if [[ -z "${VYOS_HOST}" ]]; then
  fail "VYOS_HOST is required"
fi
if [[ -z "${VYOS_USER}" ]]; then
  fail "VYOS_USER is required"
fi
if [[ -z "${VYOS_PASSWORD}" && -z "${VYOS_SSH_KEY}" ]]; then
  fail "VYOS_PASSWORD or VYOS_SSH_KEY is required"
fi
if [[ -n "${VYOS_PASSWORD}" && -n "${VYOS_SSH_KEY}" ]]; then
  fail "set only one of VYOS_PASSWORD or VYOS_SSH_KEY"
fi
if [[ -z "${STATE_PATH}" ]]; then
  fail "STATE_PATH is required"
fi
if [[ ! -f "${PAYLOAD_FILE}" ]]; then
  fail "DESIRED_CONFIG_FILE not found: ${PAYLOAD_FILE}"
fi

validate_no_single_quotes "STATE_PATH" "${STATE_PATH}"
validate_no_single_quotes "REMOTE_AGENT_LOG" "${REMOTE_AGENT_LOG}"

require_cmd go
require_cmd ssh
if [[ -n "${VYOS_PASSWORD}" ]]; then
  require_cmd sshpass
fi

mkdir -p "${ARTIFACT_DIR}" "${CONTROLLER_DIR}"
: > "${ARTIFACT_DIR}/commands-run.txt"
: > "${AGENT_LOG}"
write_environment_summary

echo "[INFO] artifacts will be written to ${ARTIFACT_DIR}"
echo "[INFO] using desired config fixture ${PAYLOAD_FILE}"
echo "[INFO] target=${TARGET} rpc_id=${RPC_ID} uuid=${CONFIG_UUID}"
echo "[INFO] assuming NATS server is already running at ${NATS_URL}"
echo "[INFO] assuming vyos-nats-agent is already running on ${VYOS_HOST}"

record_command "ssh show before config"
if ! ssh_cmd "${VYOS_SHOW_CONFIG_COMMAND}" > "${ARTIFACT_DIR}/vyos-before.txt" 2>>"${ARTIFACT_DIR}/ssh-errors.log"; then
  fail "could not collect pre-apply VyOS configuration"
fi

echo "[INFO] preparing controller config"
sed \
  -e "s#nats://127.0.0.1:4222#${NATS_URL}#g" \
  -e "s#target: vyos#target: ${TARGET}#g" \
  config.example.yaml > "${TMP_CONFIG}"

cat > "${CONTROLLER_DIR}/main.go" <<'GO'
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Telecominfraproject/olg-nats-agent-core/agentcore"
	"github.com/Telecominfraproject/olg-server-vyos-client-natsagent/internal/config"
)

const wireVersion = "1.0"

func main() {
	var configPath string
	var payloadPath string
	var rpcID string
	var configUUID string
	var artifactDir string
	var timeout time.Duration
	var resubmit bool

	flag.StringVar(&configPath, "config", "", "Path to controller YAML config")
	flag.StringVar(&payloadPath, "payload", "", "Path to desired config JSON payload")
	flag.StringVar(&rpcID, "rpc-id", "", "RPC ID")
	flag.StringVar(&configUUID, "uuid", "", "Desired config UUID")
	flag.StringVar(&artifactDir, "artifact-dir", "", "Artifact directory")
	flag.DurationVar(&timeout, "timeout", 120*time.Second, "Timeout")
	flag.BoolVar(&resubmit, "resubmit", true, "Resubmit the same UUID and expect already_in_sync")
	flag.Parse()

	if configPath == "" || payloadPath == "" || rpcID == "" || configUUID == "" || artifactDir == "" {
		fatalf("missing required flags")
	}

	payload, err := os.ReadFile(payloadPath)
	if err != nil {
		fatalf("read payload: %v", err)
	}
	if !json.Valid(payload) {
		fatalf("payload is not valid JSON")
	}

	appCfg, err := config.Load(configPath)
	if err != nil {
		fatalf("load config: %v", err)
	}
	coreCfg, err := appCfg.ToAgentCoreConfig()
	if err != nil {
		fatalf("convert config: %v", err)
	}
	coreCfg.AgentName = "vyos-nats-agent-real-lab-controller"
	coreCfg.NATS.ClientName = "vyos-nats-agent-real-lab-controller"

	client, err := agentcore.New(coreCfg)
	if err != nil {
		fatalf("create agentcore client: %v", err)
	}

	statusFile, err := os.OpenFile(filepath.Join(artifactDir, "configure-status.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		fatalf("open status artifact: %v", err)
	}
	defer statusFile.Close()

	resultFile, err := os.OpenFile(filepath.Join(artifactDir, "configure-result.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		fatalf("open result artifact: %v", err)
	}
	defer resultFile.Close()

	target := appCfg.Agent.Target
	statusCh := make(chan agentcore.StatusEnvelope, 64)
	resultCh := make(chan agentcore.ResultEnvelope, 32)

	if err := client.RegisterStatusHandler(target, func(ctx context.Context, msg agentcore.StatusEnvelope) error {
		_ = json.NewEncoder(statusFile).Encode(msg)
		fmt.Printf("[CONTROLLER] status target=%s rpc_id=%s uuid=%s status=%s stage=%s message=%q\n",
			msg.Target, msg.RPCID, msg.UUID, msg.Status, msg.Stage, msg.Message)
		select {
		case statusCh <- msg:
		default:
		}
		return nil
	}); err != nil {
		fatalf("register status handler: %v", err)
	}

	if err := client.RegisterResultHandler(target, func(ctx context.Context, msg agentcore.ResultEnvelope) error {
		_ = json.NewEncoder(resultFile).Encode(msg)
		fmt.Printf("[CONTROLLER] result target=%s rpc_id=%s uuid=%s result=%s error_code=%s message=%q\n",
			msg.Target, msg.RPCID, msg.UUID, msg.Result, msg.ErrorCode, msg.Message)
		select {
		case resultCh <- msg:
		default:
		}
		return nil
	}); err != nil {
		fatalf("register result handler: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := client.Start(ctx); err != nil {
		fatalf("start controller client: %v", err)
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		if err := client.Close(closeCtx); err != nil {
			fmt.Fprintf(os.Stderr, "[CONTROLLER] close client: %v\n", err)
		}
	}()

	submitAndWait(ctx, client, statusCh, resultCh, target, rpcID, configUUID, payload, false)
	if resubmit {
		submitAndWait(ctx, client, statusCh, resultCh, target, rpcID+"-retry", configUUID, payload, true)
	}
}

func submitAndWait(ctx context.Context, client *agentcore.Client, statusCh <-chan agentcore.StatusEnvelope, resultCh <-chan agentcore.ResultEnvelope, target, rpcID, configUUID string, payload json.RawMessage, expectAlreadyInSync bool) {
	ack, err := client.SubmitConfigure(ctx, agentcore.ConfigureCommand{
		Version:   wireVersion,
		RPCID:     rpcID,
		Target:    target,
		UUID:      configUUID,
		Payload:   payload,
		Timestamp: time.Now().UTC(),
	})
	if err != nil {
		fatalf("submit configure: %v", err)
	}
	fmt.Printf("[CONTROLLER] submitted configure accepted=%v rpc_id=%s uuid=%s kv_bucket=%s kv_key=%s kv_revision=%d\n",
		ack.Accepted, rpcID, configUUID, ack.KVBucket, ack.KVKey, ack.KVRevision)

	for {
		select {
		case <-ctx.Done():
			fatalf("timed out waiting for configure result rpc_id=%s uuid=%s: %v", rpcID, configUUID, ctx.Err())
		case msg := <-statusCh:
			if msg.RPCID == rpcID && msg.UUID == configUUID && msg.Status == "failure" {
				fatalf("agent reported failure status at stage=%s message=%q", msg.Stage, msg.Message)
			}
			if expectAlreadyInSync && msg.RPCID == rpcID && msg.UUID == configUUID && msg.Stage == "already_in_sync" && msg.Status == "success" {
				fmt.Printf("[CONTROLLER] already_in_sync status observed rpc_id=%s uuid=%s\n", rpcID, configUUID)
			}
		case msg := <-resultCh:
			if msg.RPCID != rpcID || msg.UUID != configUUID || msg.CommandType != "configure" {
				continue
			}
			if msg.Result != "success" {
				fatalf("configure failed: error_code=%s message=%q", msg.ErrorCode, msg.Message)
			}
			if expectAlreadyInSync && msg.Message != "desired config already applied" {
				fatalf("expected already_in_sync result message, got %q", msg.Message)
			}
			fmt.Printf("[CONTROLLER] configure success rpc_id=%s uuid=%s message=%q\n", msg.RPCID, msg.UUID, msg.Message)
			return
		}
	}
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "[CONTROLLER][FAIL] "+format+"\n", args...)
	os.Exit(1)
}
GO

echo "[INFO] submitting real configure through NATS/KV"
record_command "go run controller --config <tmp> --payload ${PAYLOAD_FILE} --rpc-id ${RPC_ID} --uuid ${CONFIG_UUID}"
if ! go run "${CONTROLLER_DIR}" \
  --config "${TMP_CONFIG}" \
  --payload "${PAYLOAD_FILE}" \
  --rpc-id "${RPC_ID}" \
  --uuid "${CONFIG_UUID}" \
  --artifact-dir "${ARTIFACT_DIR}" \
  --timeout "${TIMEOUT}" \
  --resubmit="${RESUBMIT_SAME_UUID}" >"${CONTROLLER_LOG}" 2>&1; then
  fail "real VyOS lab configure smoke failed"
fi

echo "[INFO] collecting post-apply VyOS configuration"
if ! ssh_cmd "${VYOS_SHOW_CONFIG_COMMAND}" > "${ARTIFACT_DIR}/vyos-after.txt" 2>>"${ARTIFACT_DIR}/ssh-errors.log"; then
  fail "could not collect post-apply VyOS configuration"
fi
if ! grep -q "${EXPECTED_VYOS_MATCH}" "${ARTIFACT_DIR}/vyos-after.txt"; then
  fail "expected VyOS config marker not found in vyos-after.txt: ${EXPECTED_VYOS_MATCH}"
fi

echo "[INFO] collecting state file from VyOS host"
copy_state_artifact "${ARTIFACT_DIR}/state.json"
if ! grep -q "${CONFIG_UUID}" "${ARTIFACT_DIR}/state.json"; then
  fail "state artifact does not contain submitted UUID"
fi

echo "[INFO] collecting remote agent log from ${REMOTE_AGENT_LOG}"
collect_remote_agent_log_best_effort

cat > "${ARTIFACT_DIR}/phase9-summary.md" <<EOF
# Real VyOS Configure Smoke Summary

- Target: ${TARGET}
- RPC ID: ${RPC_ID}
- Config UUID: ${CONFIG_UUID}
- Desired config: ${PAYLOAD_FILE}
- State path: ${STATE_PATH}
- Same UUID resubmitted: ${RESUBMIT_SAME_UUID}
- Expected VyOS marker: ${EXPECTED_VYOS_MATCH}
- Remote agent log: ${REMOTE_AGENT_LOG}
- Result: passed

Rollback / revert notes:

Use the lab's normal VyOS rollback process or submit a known-good desired
configuration through the same NATS/KV path. Review \`vyos-before.txt\` and
\`vyos-after.txt\` before applying any rollback.
EOF

echo "[PASS] Real VyOS configure lab smoke passed"
echo "[INFO] evidence artifacts: ${ARTIFACT_DIR}"
