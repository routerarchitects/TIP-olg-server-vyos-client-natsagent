# Phase 9: Real VyOS Lab Smoke, Script Reorganization, and Evidence

## Purpose

Read these files/directories before making changes:

1. `TDD_SPEC.md`
2. `PROMPTS/TDD/PHASE7_COVERAGE.md`
3. `PROMPTS/TDD/PHASE8_COVERAGE.md`
4. `.github/workflows/ci.yml`
5. `tests/scripts/phase3-real-nats-configure-smoke.sh`
6. `tests/scripts/phase4-real-nats-action-smoke.sh`
7. `tests/scripts/real-vyos-configure-lab-smoke.sh`
8. `tests/scripts/validate-config.sh`
9. `lab/desired-vyos-wan-lan-config.json`
10. `lab/desired-vyos-wan-only-config.json`
11. `tests/integration/*`
12. `internal/agent/*`
13. `internal/renderervyos/*`
14. `internal/applyvyos/*`
15. `internal/configure/*`
16. `internal/actions/*`

We are implementing ONLY Phase 9 from `TDD_SPEC.md`:

```text
Phase 9: Real VyOS Lab Smoke Tests / Release Evidence
```

This phase should not blindly add duplicate scripts.

There are already useful scripts and lab configs:

```text
tests/scripts/phase3-real-nats-configure-smoke.sh
tests/scripts/phase4-real-nats-action-smoke.sh
tests/scripts/real-vyos-configure-lab-smoke.sh
tests/scripts/validate-config.sh

lab/desired-vyos-wan-lan-config.json
lab/desired-vyos-wan-only-config.json
```

Use, rename, reorganize, and update these existing files.

## Phase 9 Goal

Phase 9 proves final real-device confidence:

```text
real NATS
+ real VyOS NATS client
+ real renderer/apply path
+ real VyOS VM/device
+ collected evidence
```

The required TDD items are:

```text
LAB-001 Real VyOS configure applies config
LAB-002 Real VyOS state updated after apply
LAB-003 No unexpected retries / duplicate apply loop
LAB-004 Real VyOS action trace runs
LAB-005 Logs/evidence attached to PR/release notes
```

## Important CI/CD Rule

Do NOT make real VyOS tests run on every PR.

Normal PR CI should continue to run Phases 1-8:

```text
go test ./...
go test -race ./...
go build ./...
go test -tags=integration ./...
real-NATS smoke scripts that do not need VyOS
```

Phase 9 real VyOS lab tests must be:

```text
manual,
or workflow_dispatch,
or self-hosted lab-runner only.
```

Reason:

```text
GitHub-hosted runners usually cannot reach a private VyOS VM/device.
Real VyOS lab smoke requires real lab topology and credentials.
```

## Required Final Directory Structure

Reorganize the current test assets into clearer non-phase-specific names.

Target structure:

```text
tests/
  integration/
    mocked_agent_flow_integration_test.go

  smoke/
    validate-config.sh
    real-nats-configure-smoke.sh
    real-nats-action-smoke.sh

  lab/
    README.md
    real-vyos-configure-smoke.sh
    real-vyos-action-trace-smoke.sh
    collect-lab-evidence.sh

    configs/
      desired-vyos-wan-lan-config.json
      desired-vyos-wan-only-config.json

    artifacts/
      .gitkeep
```

## Required Renames / Moves

Rename and move:

```text
tests/scripts/phase3-real-nats-configure-smoke.sh
-> tests/smoke/real-nats-configure-smoke.sh

tests/scripts/phase4-real-nats-action-smoke.sh
-> tests/smoke/real-nats-action-smoke.sh

tests/scripts/validate-config.sh
-> tests/smoke/validate-config.sh

tests/scripts/real-vyos-configure-lab-smoke.sh
-> tests/lab/real-vyos-configure-smoke.sh

lab/desired-vyos-wan-lan-config.json
-> tests/lab/configs/desired-vyos-wan-lan-config.json

lab/desired-vyos-wan-only-config.json
-> tests/lab/configs/desired-vyos-wan-only-config.json
```

After moving, remove the old paths if no references remain.

Update all references in:

```text
.github/workflows/ci.yml
README docs
PROMPTS/TDD coverage docs
script comments
script default paths
```

Do not leave stale `phase3` or `phase4` script names in CI.

## Naming Rule

Avoid phase-specific names for executable scripts.

Good:

```text
real-nats-configure-smoke.sh
real-nats-action-smoke.sh
real-vyos-configure-smoke.sh
real-vyos-action-trace-smoke.sh
collect-lab-evidence.sh
```

Bad:

```text
phase3-real-nats-configure-smoke.sh
phase4-real-nats-action-smoke.sh
phase9-real-vyos-configure-smoke.sh
```

Phase numbers are acceptable in prompt/coverage docs, not in operational script names.

## Distinguish Smoke vs Lab

Keep this separation:

```text
tests/smoke/
```

For CI-friendly smoke scripts that do not require a real VyOS device.

```text
tests/lab/
```

For manual/on-demand real VyOS VM/device scripts.

This is important for reviewer clarity.

## CI Updates

Update `.github/workflows/ci.yml` to use the new smoke script paths.

Existing CI steps should point to:

```text
tests/smoke/validate-config.sh
tests/smoke/real-nats-configure-smoke.sh
tests/smoke/real-nats-action-smoke.sh
```

Do NOT add real VyOS lab scripts to required PR CI.

If adding a GitHub Actions workflow for real VyOS lab, it must be a separate manual workflow:

```text
.github/workflows/vyos-lab-smoke.yml
```

and must use:

```yaml
on:
  workflow_dispatch:
```

Prefer:

```yaml
runs-on: [self-hosted, vyos-lab]
```

Do not run this on `pull_request` or `push`.

## Phase 9 Lab Scripts

### 1. `tests/lab/real-vyos-configure-smoke.sh`

Use the existing `tests/scripts/real-vyos-configure-lab-smoke.sh` as the base.

Do not duplicate it.

Update and harden it to cover:

```text
LAB-001 configure applies on real VyOS
LAB-002 state checkpoint updates after apply
LAB-003 no unexpected duplicate/retry loop
LAB-005 evidence/log collection hooks
```

Expected behavior:

```text
validate environment variables
use real NATS URL
use selected config fixture
submit configure through NATS/KV/client path
wait for success status/result
verify expected config is visible on VyOS
verify state file contains applied UUID
optionally resubmit same UUID and verify already_in_sync / no duplicate apply
write evidence artifacts
```

Required environment variables should include, as applicable:

```text
NATS_URL
VYOS_TARGET
VYOS_HOST
VYOS_USER
VYOS_PASSWORD or VYOS_SSH_KEY
STATE_PATH
DESIRED_CONFIG_FILE
ARTIFACT_DIR
AGENT_CONFIG_FILE or AGENT_BINARY if script starts the agent
```

If the current script expects different names, keep compatibility where possible but document them.

Safety requirements:

```text
set -euo pipefail
do not echo passwords/secrets
fail clearly if required env vars are missing
use unique config UUID by default
avoid destructive changes
prefer WAN-only or WAN/LAN fixture depending on user-provided config
show rollback/revert notes
write logs to ARTIFACT_DIR
```

### 2. `tests/lab/real-vyos-action-trace-smoke.sh`

Add this script if real trace action is implemented or if there is a clear command path to exercise.

Expected behavior if real trace exists:

```text
submit trace action through NATS
wait for action success result
verify payload/message indicates trace ran
write status/result/evidence logs
```

If real trace action is NOT implemented yet:

- Keep this script as a safe placeholder that exits with a clear message unless explicitly enabled, OR do not add the script and document LAB-004 as deferred.
- Do NOT fake a real trace result using placeholder executor and call it real VyOS trace.
- In `PHASE9_COVERAGE.md`, write:

```text
LAB-004 Deferred / Partially Covered:
real platform trace executor is not implemented yet.
Placeholder trace workflow is already covered by Phase 5 and Phase 7.
Future work: implement real VyOS trace executor and enable this lab smoke.
```

### 3. `tests/lab/collect-lab-evidence.sh`

Add evidence collection script.

It should collect/copy available artifacts into `ARTIFACT_DIR`, for example:

```text
phase9-summary.md
configure-status.jsonl
configure-result.jsonl
action-status.jsonl
action-result.jsonl
agent.log
vyos-before.txt
vyos-after.txt
state.json
commands-run.txt
environment-summary.txt
```

Do not include:

```text
passwords
tokens
private keys
raw secrets
```

The script should be usable after manual lab execution.

## Lab Config Fixtures

Move existing config files into:

```text
tests/lab/configs/
```

Keep both:

```text
desired-vyos-wan-lan-config.json
desired-vyos-wan-only-config.json
```

Update lab scripts to default to one of these, for example:

```text
tests/lab/configs/desired-vyos-wan-only-config.json
```

but allow override with:

```text
DESIRED_CONFIG_FILE=/path/to/config.json
```

## Documentation

Add:

```text
tests/lab/README.md
```

It must explain:

```text
what Phase 9 proves
why Phase 9 is manual/on-demand
difference between tests/smoke and tests/lab
required lab topology
required environment variables
how to run real VyOS configure smoke
how to run action trace smoke or why it is deferred
how to collect evidence
how to attach evidence to PR/release notes
how to avoid leaking secrets
known limitations
rollback/revert notes
```

Include example commands.

## Coverage Documentation

Add:

```text
PROMPTS/TDD/PHASE9_COVERAGE.md
```

Map all LAB items:

```text
LAB-001
LAB-002
LAB-003
LAB-004
LAB-005
```

For each row include:

```text
ID
Status: Manual / Covered by script / Deferred / Partially Covered
Script/workflow
Command
Notes
Evidence artifacts
```

Also document:

```text
old paths renamed
new paths
CI smoke path updates
manual workflow status
required secrets/env vars
current real VyOS execution status
```

Important:
Do not claim real VyOS passed unless it was actually run against a real VyOS VM/device.

If only scripts/docs were added, status should say:

```text
Manual script provided; execution evidence must be attached after running in lab.
```

## Optional Manual GitHub Actions Workflow

Add only if useful:

```text
.github/workflows/vyos-lab-smoke.yml
```

Requirements:

```yaml
name: VyOS Lab Smoke

on:
  workflow_dispatch:
    inputs:
      desired_config:
        description: "Lab config fixture"
        required: false
        default: "tests/lab/configs/desired-vyos-wan-only-config.json"
      target:
        description: "VyOS target"
        required: true
```

Runner:

```yaml
runs-on: [self-hosted, vyos-lab]
```

Secrets:

```text
VYOS_HOST
VYOS_USER
VYOS_PASSWORD or VYOS_SSH_KEY
NATS_URL
```

Artifact upload:

```yaml
- uses: actions/upload-artifact@v4
  with:
    name: vyos-lab-evidence
    path: tests/lab/artifacts/
```

Rules:

```text
workflow_dispatch only
not required PR CI
self-hosted runner preferred
do not print secrets
upload evidence artifacts
```

If self-hosted runner is not available, document this workflow as optional or defer adding it.

## Normal CI Validation

After reorganizing paths, normal CI must still pass.

Run:

```bash
go test ./...
go test -race ./...
go build ./...
go test -tags=integration ./...
bash -n tests/smoke/*.sh
bash -n tests/lab/*.sh
```

If `shellcheck` is available:

```bash
shellcheck tests/smoke/*.sh tests/lab/*.sh
```

Do not require real VyOS to pass these syntax/normal CI checks.

## Final Codex Response Required

After implementation, summarize:

1. Files moved/renamed.
2. Files added.
3. Files removed.
4. CI path updates.
5. Phase 9 LAB-* coverage status.
6. Whether existing scripts were reused or replaced.
7. How to run CI-friendly smoke tests.
8. How to run real VyOS lab configure smoke.
9. How to collect evidence.
10. Whether a manual GitHub workflow was added.
11. Commands run and results.
12. Whether real VyOS was actually executed.
13. What remains before release.

## Acceptance Criteria

Phase 9 is complete when:

```text
[ ] old phase-specific smoke script names are removed or no longer referenced
[ ] CI-friendly scripts live under tests/smoke/
[ ] real VyOS lab scripts live under tests/lab/
[ ] lab config fixtures live under tests/lab/configs/
[ ] .github/workflows/ci.yml references new tests/smoke paths
[ ] real VyOS lab scripts are not required on every PR
[ ] real-vyos-configure-smoke.sh covers LAB-001/LAB-002/LAB-003 as manual lab script
[ ] LAB-004 real trace is covered or explicitly deferred if real trace is not implemented
[ ] collect-lab-evidence.sh or equivalent evidence process exists
[ ] tests/lab/README.md explains manual lab execution and evidence collection
[ ] PHASE9_COVERAGE.md maps LAB-001 through LAB-005
[ ] scripts validate env vars and do not print secrets
[ ] normal CI remains green
[ ] bash -n passes for smoke/lab scripts
[ ] no claim is made that real VyOS passed unless actually executed
```
