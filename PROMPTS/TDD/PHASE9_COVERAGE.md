# Phase 9 Coverage: Real VyOS Lab Smoke and Evidence

Phase 9 reorganizes the smoke/lab scripts into non-phase operational paths and
adds manual real-VyOS evidence collection. Real VyOS lab scripts are not run in
normal PR CI.

## Renamed Paths

| Old path | New path |
|---|---|
| `tests/scripts/phase3-real-nats-configure-smoke.sh` | `tests/smoke/real-nats-configure-smoke.sh` |
| `tests/scripts/phase4-real-nats-action-smoke.sh` | `tests/smoke/real-nats-action-smoke.sh` |
| `tests/scripts/validate-config.sh` | `tests/smoke/validate-config.sh` |
| `tests/scripts/real-vyos-configure-lab-smoke.sh` | `tests/lab/real-vyos-configure-smoke.sh` |
| `lab/desired-vyos-wan-lan-config.json` | `tests/lab/configs/desired-vyos-wan-lan-config.json` |
| `lab/desired-vyos-wan-only-config.json` | `tests/lab/configs/desired-vyos-wan-only-config.json` |

## CI Smoke Updates

| CI job | Step | Command | Timeout |
|---|---|---|---|
| `Smoke Tests` | `Validate config script` | `./tests/smoke/validate-config.sh` | job timeout |
| `Smoke Tests` | `Real-NATS configure smoke` | `NATS_PORT=4223 ./tests/smoke/real-nats-configure-smoke.sh` | 5 minutes |
| `Smoke Tests` | `Real-NATS action smoke` | `NATS_PORT=4224 ./tests/smoke/real-nats-action-smoke.sh` | 5 minutes |

## LAB Coverage

| ID | Status | Script/workflow | Command | Notes | Evidence artifacts | Prerequisites |
|---|---|---|---|---|---|---|
| LAB-001 | Covered by script / Manual | `tests/lab/real-vyos-configure-smoke.sh` | `REAL_VYOS_LAB_ACK=I_UNDERSTAND ... ./tests/lab/real-vyos-configure-smoke.sh` | Submits configure through real NATS/KV and verifies the expected marker appears in VyOS configuration output. Not executed in this implementation pass. | `configure-status.jsonl`, `configure-result.jsonl`, `vyos-before.txt`, `vyos-after.txt`, `phase9-summary.md` | Real NATS, reachable VyOS SSH, agent running in real configure mode, safe desired config fixture |
| LAB-002 | Covered by script / Manual | `tests/lab/real-vyos-configure-smoke.sh` | same as LAB-001 | Copies `STATE_PATH` from the VyOS host and checks it contains the submitted UUID. Not executed in this implementation pass. | `state.json`, `phase9-summary.md` | `STATE_PATH` must point to the agent state file on the VyOS host |
| LAB-003 | Covered by script / Manual | `tests/lab/real-vyos-configure-smoke.sh` | `RESUBMIT_SAME_UUID=true ... ./tests/lab/real-vyos-configure-smoke.sh` | Resubmits the same UUID and expects the already-in-sync success result, proving no duplicate apply loop for the same UUID. Not executed in this implementation pass. | `configure-status.jsonl`, `configure-result.jsonl`, `commands-run.txt` | Same as LAB-001 |
| LAB-004 | Deferred / Partially Covered | `tests/lab/real-vyos-action-trace-smoke.sh` | `./tests/lab/real-vyos-action-trace-smoke.sh` | Real platform trace executor is not implemented yet. Placeholder trace workflow is covered by Phases 5 and 7. Future work: implement real VyOS trace executor and enable real trace lab smoke. | `phase9-summary.md`, empty `action-status.jsonl`, empty `action-result.jsonl` deferral artifacts | Real trace executor implementation |
| LAB-005 | Covered by script / Manual process | `tests/lab/collect-lab-evidence.sh` | `ARTIFACT_DIR=tests/lab/artifacts/<run> ./tests/lab/collect-lab-evidence.sh` | Collects and normalizes evidence for PR/release attachment. It records only boolean secret-presence fields and avoids secret values. | `phase9-summary.md`, `configure-status.jsonl`, `configure-result.jsonl`, `action-status.jsonl`, `action-result.jsonl`, `agent.log`, `vyos-before.txt`, `vyos-after.txt`, `state.json`, `commands-run.txt`, `environment-summary.txt` | Manual review before attaching artifacts |

## Manual Workflow Status

No required PR CI runs real VyOS lab scripts. The optional manual workflow is:

| Workflow | Trigger | Runner | Command | Evidence handling |
|---|---|---|---|---|
| `.github/workflows/vyos-lab-smoke.yml` | `workflow_dispatch` only | `[self-hosted, vyos-lab]` | `./tests/lab/real-vyos-configure-smoke.sh` | Collects evidence locally on the self-hosted runner under `tests/lab/artifacts/github-actions/run-<run_id>-attempt-<run_attempt>` and prints that path at the end of the run. Artifacts are not uploaded automatically and must be reviewed/sanitized before being shared. |

## Current Real VyOS Execution Status

Real VyOS configure smoke was executed manually in a disposable lab and passed for LAB-001 through LAB-003.

Raw evidence artifacts were reviewed locally and intentionally not committed because they contain lab-specific configuration, private IPs, host-key data, and VyOS output. Sanitized excerpts can be provided if requested.

Real trace action remains deferred until a real platform trace/rtty executor exists.

## Local Commands

CI-friendly smoke:

```bash
./tests/smoke/validate-config.sh
NATS_PORT=4223 ./tests/smoke/real-nats-configure-smoke.sh
NATS_PORT=4224 ./tests/smoke/real-nats-action-smoke.sh
```

Manual real VyOS configure:

```bash
REAL_VYOS_LAB_ACK=I_UNDERSTAND \
NATS_URL=nats://<nats-host>:4222 \
VYOS_TARGET=vyos \
VYOS_HOST=<vyos-host> \
VYOS_USER=vyos \
VYOS_SSH_KEY=/path/to/key \
STATE_PATH=/tmp/vyos-nats-agent/state.json \
./tests/lab/real-vyos-configure-smoke.sh
```
