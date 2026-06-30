# Real VyOS Lab Configure Smoke

This is a manual end-to-end test for `agent.configure.mode: real`.

Use only a disposable/lab VyOS VM. The real apply backend can change VyOS configuration.

## Topology

```text
controller/dev machine
  - nats-server -js
  - lab smoke controller script

VyOS VM
  - vyos-nats-agent binary
  - config.yaml with agent.configure.mode: real
  - NATS URL pointing to controller/dev machine
```

The controller script writes desired JSON to the configured JetStream KV bucket
through `agentcore.SubmitConfigure`, which also publishes the configure
notification. The VyOS agent receives that notification, loads KV, renders with
`olg-renderer-vyos`, applies through `olg-renderer-vyos/apply`, saves local
state, and publishes result/status.

## 1. Start NATS On The Controller Machine

Install `nats-server` if needed, then start JetStream on an address the VyOS VM
can reach:

```bash
nats-server -js -p 4222 -sd /tmp/vyos-nats-js
```

Find the controller machine IP reachable from the VyOS VM:

```bash
ip addr
```

The examples below use:

```text
nats://192.0.2.10:4222
```

Replace it with your real lab IP.

## 2. Build The Agent Binary

From this repo:

```bash
GOOS=linux GOARCH=amd64 go build -o ./bin/vyos-nats-agent ./cmd/vyos-nats-agent
```

If your VyOS VM architecture differs, adjust `GOARCH`.

## 3. Copy Binary And Config To VyOS

Copy the binary:

```bash
scp ./bin/vyos-nats-agent vyos@<vyos-vm-ip>:~/vyos-nats-agent
```

Create a lab config locally:

```bash
cp config.example.yaml /tmp/vyos-nats-agent-real.yaml
```

Edit `/tmp/vyos-nats-agent-real.yaml`:

```yaml
agent:
  target: vyos
  state_file: /tmp/vyos-nats-agent/state.json

  configure:
    mode: real

agentcore:
  nats:
    servers:
      - nats://192.0.2.10:4222

  kv:
    auto_create_bucket: true
```

Copy it to the VM:

```bash
scp /tmp/vyos-nats-agent-real.yaml vyos@<vyos-vm-ip>:~/vyos-nats-agent.yaml
```

## 4. Run The Agent On VyOS

SSH into the VM:

```bash
ssh vyos@<vyos-vm-ip>
```

Prepare paths and run:

```bash
sudo mkdir -p /config/vyos-nats-agent
sudo install -m 0755 ~/vyos-nats-agent /usr/local/bin/vyos-nats-agent
sudo /usr/local/bin/vyos-nats-agent --config ~/vyos-nats-agent.yaml
```

Keep this terminal open so you can see logs. In another terminal, run the
controller-side smoke.

## 5. Prepare Desired JSON Payload

Use a renderer-supported OLG/uCentral payload. For real mode, avoid the Phase 3
placeholder payload shape such as `{"hostname":"..."}`; the real renderer
expects the OLG/uCentral config object or a wrapper containing `config`.

Example wrapper shape:

```json
{
  "schema_name": "olg-ucentral",
  "schema_version": "4.2.0",
  "config": {
    "interfaces": [],
    "services": {},
    "uuid": 1770891457
  }
}
```

Use a payload that is safe for your lab VM. Interface/NAT payloads can change
addresses, bridges, VLANs, NAT rules, commit state, and saved configuration.

For a first end-to-end smoke, start with the WAN-only sample payload:

```text
./lab/desired-vyos-wan-only-config.json
```

For larger interface/NAT tests, create a separate payload that matches the lab
VM topology.

## 6. Run The Lab Smoke Controller

From this repo on the controller machine:

```bash
REAL_VYOS_LAB_ACK=I_UNDERSTAND \
NATS_URL=nats://192.0.2.10:4222 \
PAYLOAD_FILE=./lab/desired-vyos-wan-only-config.json \
PRINT_LOGS_ON_PASS=true \
./tests/scripts/real-vyos-configure-lab-smoke.sh
```

Expected success marker:

```text
[PASS] Real VyOS lab configure smoke passed
```

The script fails if:

- it cannot connect to NATS
- the desired JSON is invalid
- the agent publishes a configure failure
- no configure result arrives before timeout

## 7. Verify On VyOS

Check agent state:

```bash
sudo cat /tmp/vyos-nats-agent/state.json
```

Check VyOS configuration using normal VyOS inspection commands appropriate for
the payload you applied.

## Notes

- Normal CI must keep using `agent.configure.mode: placeholder`.
- This script intentionally requires `REAL_VYOS_LAB_ACK=I_UNDERSTAND`.
- Full payload/rendered/apply-plan logs are disabled by default. For temporary
  lab debugging, set `agent.logging.level: debug` and enable the specific
  `agent.debug.*` flags needed for the run.
- The agent binary does not directly execute raw VyOS commands. In real mode it
  calls the public `olg-renderer-vyos/renderer` and `olg-renderer-vyos/apply`
  APIs.
- `olg-nats-agent-core` and `olg-renderer-vyos` are resolved through normal Go
  module dependency management.
