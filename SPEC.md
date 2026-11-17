# VyOS NATS Agent Specification

## 1. Project name

```text
vyos-nats-agent
```

## 2. Goal

Build a minimal VyOS-focused daemon that uses `olg-nats-agent-core` for all NATS, JetStream KV, subject, handler, result, and status communication.

The safe default must prove an end-to-end configure/action lifecycle using:

- YAML runtime configuration
- placeholder VyOS renderer
- placeholder VyOS apply engine
- placeholder and real `trace` action executors
- real NATS integration tests

The agent can also run a real configure backend when explicitly configured. Real mode uses `github.com/Telecominfraproject/olg-renderer-vyos` renderer/apply public APIs and must be validated on disposable/lab VyOS targets before production rollout.

## 3. Core architecture

`vyos-nats-agent` is a daemon.

`olg-nats-agent-core` is a library embedded inside the daemon.

The agent owns:

- process lifecycle
- YAML configuration loading
- conversion to `agentcore.Config`
- handler registration
- placeholder rendering
- placeholder apply logic
- real configure renderer/apply adapter wiring
- placeholder action execution
- local applied UUID state
- result/status publication decisions

The library owns:

- NATS connection
- JetStream access
- desired-config KV
- subject patterns
- configure/action/result/status envelopes
- handler registration mechanics
- publish/subscribe mechanics
- reconnect behavior

## 4. Configuration source of truth

The agent must be configured from YAML.

The implementation must not hardcode:

- NATS server URLs
- NATS client name
- NATS authentication paths or values
- JetStream settings
- KV bucket name
- KV key pattern
- subject patterns
- target name
- local state path
- configure backend mode
- apply save behavior
- debug logging flags
- enabled actions
- timeout values
- retry values

## 5. Config path resolution

The config path must be resolved in this order:

```text
1. --config /path/to/config.yaml
2. VYOS_NATS_AGENT_CONFIG=/path/to/config.yaml
3. /etc/vyos-nats-agent/config.yaml
```

## 6. Config file format

The primary config format is YAML.

The repository must include:

```text
config.example.yaml
```

Do not support JSON or TOML in milestone 1.

The config loader must be isolated so additional formats can be added later without changing the rest of the agent.

## 7. Config file structure

The YAML config must have two top-level sections:

```yaml
agent: {}
agentcore: {}
```

## 8. Agent config section

The `agent` section contains VyOS-agent-specific settings.

Required fields:

```yaml
agent:
  name: vyos-nats-agent
  version: 0.1.0
  target: vyos
  state_file: /tmp/vyos-nats-agent/state.json
```

Optional sections:

```yaml
agent:
  logging:
    enabled: true
    level: info
    format: text

  configure:
    mode: placeholder

  debug:
    log_payloads: false
    log_rendered: false
    log_apply_plan: false

  apply:
    save_after_commit: false

  actions:
    enabled:
      - trace

  state:
    enabled: true
    interval: 60s # Initial boot/fallback default. Authoritatively overridden by Cloud metrics.statistics.interval
    max_concurrent_requests: 5 # Configurable worker capacity for on-demand state.get queries (default: 5)
```

## 9. Agentcore config section

The `agentcore` section contains settings used to construct `agentcore.Config`.

Required subsections:

```yaml
agentcore:
  nats: {}
  subjects: {}
  kv: {}
```

Recommended full shape:

```yaml
agentcore:
  nats:
    servers:
      - nats://127.0.0.1:4222
    client_name: vyos-nats-agent
    credentials_file: ""
    nkey_seed_file: ""
    user_jwt_file: ""
    username: ""
    password: ""
    token: ""
    connect_timeout: 5s
    retry_on_failed_connect: false
    max_reconnects: -1
    reconnect_wait: 2s
    reconnect_buf_size: 0
    tls:
      enabled: false
      insecure_skip_verify: false
      ca_file: ""
      cert_file: ""
      key_file: ""
      server_name: ""

  jetstream:
    domain: ""
    api_prefix: ""
    default_timeout: 5s

  subjects:
    configure_pattern: cmd.configure.%s
    action_pattern: cmd.action.%s.%s
    result_pattern: result.%s
    status_pattern: status.%s
    health_pattern: health.%s

  kv:
    bucket: cfg_desired
    key_pattern: desired.%s
    auto_create_bucket: true
    history: 1
    ttl: 0s
    max_value_size: 0
    storage: file
    replicas: 1

  timeouts:
    publish_timeout: 5s
    subscribe_timeout: 5s
    kv_timeout: 5s
    shutdown_timeout: 10s
    handler_warn_after: 2s

  retry:
    publish_attempts: 1
    publish_backoff: 250ms

  execution:
    handler_mode: sync
```

For milestone 1, KV history is intentionally `1` because the agent converges from
the latest desired config UUID and local applied UUID comparison, not historical
KV revisions.

### KV bucket ownership

The VyOS agent needs KV settings so it can load the latest desired config, but production bucket creation should be owned by the controller/provisioning side.

`agentcore.kv.auto_create_bucket: true` is intended for local development and integration tests.

Production VyOS agents should normally use:

```yaml
agentcore:
  kv:
    auto_create_bucket: false
```

and bind to an existing JetStream KV bucket created by the controller/provisioning workflow.

## 10. Duration handling

The YAML config must use human-readable duration strings.

Examples:

```yaml
connect_timeout: 5s
publish_backoff: 250ms
ttl: 0s
```

Implementation rule:

```text
config.yaml
  -> internal/config.AppConfig with string duration fields
  -> Validate()
  -> time.ParseDuration(...)
  -> agentcore.Config
```

Do not unmarshal YAML directly into `agentcore.Config`.

## 11. Recommended YAML package

Use:

```bash
go get gopkg.in/yaml.v3
```

## 12. Default values

Defaults may be applied only when fields are omitted.

Default values:

```text
agent.name = vyos-nats-agent
agent.version = 0.1.0
agent.target = vyos
agent.state_file = /tmp/vyos-nats-agent/state.json
agent.logging.enabled = true
agent.logging.level = info
agent.logging.format = text
agent.configure.mode = placeholder
agent.debug.log_payloads = false
agent.debug.log_rendered = false
agent.debug.log_apply_plan = false
agent.apply.save_after_commit = false
agent.actions.mode = placeholder
agent.actions.enabled = [trace]

agentcore.nats.servers = [nats://127.0.0.1:4222]
agentcore.nats.client_name = vyos-nats-agent
agentcore.nats.connect_timeout = 5s
agentcore.nats.retry_on_failed_connect = false
agentcore.nats.max_reconnects = -1
agentcore.nats.reconnect_wait = 2s

agentcore.jetstream.default_timeout = 5s

agentcore.subjects.configure_pattern = cmd.configure.%s
agentcore.subjects.action_pattern = cmd.action.%s.%s
agentcore.subjects.result_pattern = result.%s
agentcore.subjects.status_pattern = status.%s
agentcore.subjects.health_pattern = health.%s

agentcore.kv.bucket = cfg_desired
agentcore.kv.key_pattern = desired.%s
agentcore.kv.auto_create_bucket = true
agentcore.kv.history = 1
agentcore.kv.ttl = 0s
agentcore.kv.storage = file
agentcore.kv.replicas = 1

agentcore.timeouts.publish_timeout = 5s
agentcore.timeouts.subscribe_timeout = 5s
agentcore.timeouts.kv_timeout = 5s
agentcore.timeouts.shutdown_timeout = 10s
agentcore.timeouts.handler_warn_after = 2s

agentcore.retry.publish_attempts = 1
agentcore.retry.publish_backoff = 250ms
agentcore.execution.handler_mode = sync
```

## 13. Validation rules

The config loader must reject:

- empty agent target
- empty state file path
- empty NATS server list
- empty KV bucket
- empty KV key pattern
- empty configure subject pattern
- empty action subject pattern
- empty result subject pattern
- empty status subject pattern
- invalid duration strings
- unsupported configure mode
- unsupported action names
- unsupported logging level
- unsupported logging format
- negative timeout values where unsupported
- invalid KV history values
- invalid KV storage values

For milestone 1, supported values are:

```text
configure.mode = placeholder | real
actions.mode = placeholder | real
actions.enabled = trace
```

`configure.mode = placeholder` is the default and is the only mode used by normal CI smoke tests. `configure.mode = real` wires adapters around `olg-renderer-vyos/renderer` and `olg-renderer-vyos/apply`; the NATS agent must not execute raw VyOS commands directly.

`agent.configure.mode` is the single source of truth for configure backend selection. `agent.apply.save_after_commit` only controls whether the real apply backend saves committed VyOS configuration.

Desired payload debug logging emits metadata only, not raw payload bodies. Rendered command and apply-plan logging are disabled by default. Debug logging requires `agent.logging.level = debug` plus the relevant `agent.debug.*` flag.

## 14. Runtime config package

Use this package structure:

```text
internal/config/
  config.go      // raw config structs
  defaults.go    // default values
  loader.go      // path resolution and YAML loading
  validate.go    // validation rules
  convert.go     // conversion to agentcore.Config
```

Expected usage:

```go
cfg, err := config.Load(path)
if err != nil {
    return err
}

if err := cfg.Validate(); err != nil {
    return err
}

agentCoreCfg, err := cfg.ToAgentCoreConfig()
if err != nil {
    return err
}
```

## 15. Public daemon lifecycle

Startup sequence:

```text
1. Resolve config path.
2. Load YAML config.
3. Apply defaults.
4. Validate config.
5. Convert config to agentcore.Config.
6. Create agentcore.Client.
7. Create renderer implementation for selected configure mode.
8. Create apply engine implementation for selected configure mode.
9. Create action implementations.
10. Create local state store.
11. Register configure handler.
12. Register action handlers.
13. Start agentcore.Client.
14. Publish startup status.
15. Run startup reconcile.
16. Wait for SIGINT/SIGTERM.
17. Close agentcore.Client gracefully.
```

## 16. Configure lifecycle

When the agent receives a configure notification:

```text
1. Receive ConfigureNotification.
2. Validate notification target and UUID.
3. Publish running status.
4. Load desired config with LoadDesiredConfig(ctx, target).
5. Validate desired record target and UUID.
6. Compare desired UUID with local applied UUID.
7. If already applied:
   - publish success result
   - do not render
   - do not apply
8. If not applied:
   - pass desired payload to Renderer.Render(...)
   - pass rendered config to ApplyEngine.Apply(...)
   - update local applied UUID after render and apply both succeed
   - publish success result
9. If any step fails:
   - before local state save: do not update local applied UUID and publish failure result
   - after local state save: keep local applied UUID and treat outbound reporting failure separately from configure failure
```

### Configure recovery model

Configure notification is only a fast-path trigger.

The durable source of truth is the latest desired config stored in JetStream KV at `desired.<target>`.

The agent must not rely only on receiving `cmd.configure.<target>` notifications for correctness.

If a configure notification is missed, or if `SubmitConfigure` writes KV successfully but fails before publishing the notification, the agent must still converge by:

1. loading the latest desired config from KV during startup reconcile or explicit recover,
2. comparing desired UUID with the locally applied UUID,
3. rendering/applying when the desired UUID differs,
4. updating local applied UUID only after successful apply,
5. publishing result/status using the desired config metadata where available.

## 17. Action lifecycle

For milestone 1, only the `trace` action is required.

When the agent receives an action command:

```text
1. Receive ActionCommand.
2. Check action is enabled in config.
3. Publish running status if useful.
4. Execute trace action (either placeholder or real mode based on agent.actions.mode).
5. Publish success or failure result.
```

## 18. Renderer interface

The renderer must be behind an interface.

```go
type Renderer interface {
    Render(ctx context.Context, payload []byte) ([]byte, error)
}
```

Implementations:

```text
placeholder renderer
olg-renderer-vyos renderer adapter when configure.mode = real
```

The placeholder renderer must not call real VyOS code.

It may return a deterministic text representation of the input payload.

## 19. Apply engine interface

The apply engine must be behind an interface.

```go
type Engine interface {
    Apply(ctx context.Context, rendered []byte) error
}
```

Implementations:

```text
placeholder apply engine
olg-renderer-vyos apply adapter when configure.mode = real
```

The placeholder apply engine must not call real VyOS commands. The NATS agent must not directly execute raw VyOS shell/configuration commands; real mode delegates to the public `olg-renderer-vyos/apply` API.

It may log or store the rendered config for test verification.

## 20. Action executor interface

Action execution must be behind a generic `Executor` interface.

```go
type Executor interface {
	Execute(ctx context.Context, msg agentcore.ActionCommand) (Output, error)
}

type Output struct {
	Payload json.RawMessage
	Message string
}
```

### 20.1 Placeholder Mode

The placeholder trace executor returns deterministic JSON output mimicking the success payload without executing any command.

### 20.2 Real Mode (`VyOSTraceExecutor`)

When `agent.actions.mode: real` is enabled, the agent executes real packet captures:
1. **Strict Interface Validation**: Checks `/sys/class/net/<interface>` directly. If missing, validates against regex `^(eth|bond|dum|vlan|wlan|lo)[0-9]+(\.[0-9]+)?$`. Slashes, backslashes, dots, or shell symbols are strictly rejected to prevent command injection.
2. **Parameter Bounds**: Restricts duration to a maximum of 300 seconds, and packets to a maximum of 10000.
3. **Secure PCAP Path**: Securely creates a temporary PCAP file using `os.CreateTemp("", "pcap-*.pcap")` rather than constructed from `RPCID`, preventing directory traversal vulnerability.
4. **Streaming Multipart Upload**: Streams the resulting PCAP to the controller's upload URI using a zero-copy streaming pipeline (`io.Pipe` + `mime/multipart`) to avoid memory-bound issues.
5. **tcpdump execution**: Invokes `/usr/bin/tcpdump -U -i <interface> -w <path>` and stops when the timed context expires or packet limit is reached.

## 21. Local state

The runtime config is YAML.

The local state file is JSON.

Default path:

```text
/tmp/vyos-nats-agent/state.json
```

State shape:

```json
{
  "target": "vyos",
  "applied_uuid": "cfg-e2e-1",
  "applied_at": "2026-05-12T10:30:00Z"
}
```

Rules:

- create the parent directory if needed
- tolerate missing file as empty state
- write updates atomically
- update `applied_uuid` only after successful apply
- never update applied UUID after render/apply failure
- milestone 1 converges from only the latest desired config (`history = 1`) and
  does not depend on historical KV revisions

### Apply success with state-save failure

The agent updates local `applied_uuid` only after apply succeeds. If apply
succeeds but saving local state fails, the agent surfaces the error as a reporting
failure warning (skipping both success and failure result publications) and does not
checkpoint the UUID. A later retry may re-process the same desired config.

Real-mode lab validation should include retry behavior around post-apply
state-save failures before production rollout.

In real mode, apply failures must prevent local state save. State save remains the configure service's responsibility, not the renderer/apply adapters' responsibility.

### Apply success with reporting failure

If apply and local state save both succeed, the desired UUID is considered
applied locally. If final success status or success result publication fails
after that checkpoint, the agent must not publish a contradictory configure
failure result for the same UUID. Instead it should keep the checkpointed UUID
and surface the problem as a reporting failure (for example through returned
errors, warnings, or later observability hooks).

## 22. Result publishing

Every accepted configure/action request should publish a final result under
normal conditions.

Configure exception:
- if final success status or final success result publication fails after
  apply and local state checkpoint both succeed, the agent must not publish a
  contradictory configure failure result for that UUID
- this is treated as reporting failure, not configure failure
- durable reporting retry is future hardening work

Configure success:

```json
{
  "version": "1.0",
  "rpc_id": "rpc-123",
  "target": "vyos",
  "command_type": "configure",
  "uuid": "cfg-123",
  "result": "success",
  "message": "VyOS configuration applied",
  "timestamp": "2026-05-12T10:30:00Z"
}
```

Configure failure:

```json
{
  "version": "1.0",
  "rpc_id": "rpc-123",
  "target": "vyos",
  "command_type": "configure",
  "uuid": "cfg-123",
  "result": "failure",
  "error_code": "apply_failed",
  "message": "VyOS commit failed",
  "timestamp": "2026-05-12T10:30:00Z"
}
```

Action success:

```json
{
  "version": "1.0",
  "rpc_id": "rpc-456",
  "target": "vyos",
  "command_type": "action",
  "action": "trace",
  "result": "success",
  "message": "Trace completed",
  "payload": {},
  "timestamp": "2026-05-12T10:30:00Z"
}
```

## 23. Status & device state publishing

### 23.1 Lifecycle & progress status publishing

Status publishing is allowed for startup/running/progress messages.

Minimum startup status:

```json
{
  "version": "1.0",
  "target": "vyos",
  "status": "running",
  "stage": "startup",
  "message": "vyos-nats-agent started",
  "timestamp": "2026-05-12T10:30:00Z"
}
```

### 23.2 Periodic device state collection & publishing

The VyOS NATS agent runs directly within the VyOS router runtime environment (such as a VyOS VM or bare-metal VyOS appliance) and is the authoritative source for the router's operational state, network interface counters, physical and logical port link states, and network neighbors.

It gathers metrics and periodically publishes a structured state snapshot exclusively over NATS on `state.<target>` to be consumed by the uCentral client and forwarded upstream to the OpenWiFi Cloud Gateway.

#### 23.2.1 Execution Environment & Metric Scoping

* **Execution Boundary:** `vyos-nats-agent` executes inside the VyOS router runtime environment (VyOS VM or bare-metal appliance) with direct access to local VyOS configuration APIs.
* **Metric Scoping:** All telemetry (`/proc`, `/sys`, `iproute2`, LLDP) reflects the **VyOS Router Appliance** (guest CPU/RAM, routing interfaces, and routed client neighbors), not the underlying Ubuntu hypervisor host. Host bare-metal infrastructure monitoring is handled out-of-band and is decoupled from OpenWiFi router state.

#### Subject Routing & Separation Rules

* **Exclusive `state.<target>` routing:** Periodic operational state MUST be published exclusively on `state.<target>`.
* **On-Demand live query (`state.get.<target>`):** The agent registers a request-reply handler on `state.get.<target>`. When a query arrives, it collects fresh router metrics (/proc, sysfs, Netlink) immediately, computes transient deltas without mutating or advancing the periodic baseline, and responds on the reply inbox with an `agentcore.StateEnvelope`.
* **Separation from `status.<target>` and `status.get.<target>`:** The existing `status.<target>` and `status.get.<target>` subjects are strictly reserved for lifecycle events (e.g. startup) and configure/action execution progress. These are not interchangeable message types: periodic and on-demand device state have a completely different schema and purpose. Publishing full device-state payloads on the status subjects could cause consumers to interpret them as progress/status messages or reject them as invalid.
* **Agentcore prerequisite:** Support for dedicated `state.<target>` and `state.get.<target>` subjects in `agentcore` (`DefaultStatePattern = "state.%s"`, `DefaultStateGetPattern = "state.get.%s"`, `PublishState`, `RegisterStateHandler`, and `RegisterStateGetHandler`) must be implemented or verified before state features are enabled.

```text
+---------------------------------------------------------------------------------------------------+
|                                 VYOS NATS AGENT STATE TELEMETRY                                   |
|                                                                                                   |
|  [ VyOS Router Environment (/proc, sysfs, iproute2) ]                                            |
|       │                                           ▲                                               |
|       ▼ (Periodic collection)                     │ (On-Demand live query)                        |
|  [ Periodic State Collector ]              [ On-Demand Responder (state.get.<target>) ]           |
|       │                                           ▲                                               |
|       ▼                                           │ (Request)                                     |
|  [ NATS: state.<target> ]                  [ NATS: state.get.<target> ]                           |
|  (Publish StateEnvelope)                   (Request-Reply / Live State)                           |
+───────┼───────────────────────────────────────────▲───────────────────────────────────────────────+
        │                                           │
        ▼ (Periodic State)                          │ (Live Query)
   [ uCentral Client ] ─────────────────────────────┘
```

#### Metrics Data Sources

| State Section | Metric Fields | Linux / VyOS Source |
| :--- | :--- | :--- |
| **`unit.load`** | `[1m, 5m, 15m]` load averages | `/proc/loadavg` |
| **`unit.cpu_load`** | Aggregate & per-core CPU load % | `/proc/stat` |
| **`unit.memory`** | `total`, `free`, `cached`, `buffered` RAM (bytes) | `/proc/meminfo` |
| **`unit.uptime`** | Seconds since boot (integer) | `/proc/uptime` |
| **`unit.localtime`** | Current epoch timestamp | Standard system clock |
| **`interfaces`** | Name, interface uptime, IP addresses | `ip -j addr`, `ip -j link` |
| **`interfaces.counters`** | Cumulative rx/tx bytes, packets, errors, drops | `/sys/class/net/<iface>/statistics/` or `/proc/net/dev` |
| **`interfaces.delta_counters`** | Byte/packet increment since previous sampling tick | In-memory delta computation |
| **`interfaces.clients`** | Observed L3 network neighbors (filtered MAC, IP, ports) | `ip -j neigh` (filtered) with optional DHCP lease / Bridge FDB reconciliation |
| **`link-state`** | `carrier` (bool), `speed` (Mbps), `duplex` (`full`/`half`) | `/sys/class/net/<iface>/{carrier,speed,duplex}` |
| **`lldp-peers`** | Connected switches/routers announcing via LLDP | `lldpcli -f json show neighbors` (empty `{}` if inactive) |
| **`radios`** | Radio metrics (channel, frequency, tx_power, active SSIDs) | Populated via `nl80211`/`iw` when Wi-Fi hardware is present; empty array `[]` on purely wired appliances |
| **`uuid`** | Current applied configuration UUID | Read from local state file (`/tmp/vyos-nats-agent/state.json`). Formatted as decimal string (e.g. `"1793880893"`), or `""`/`"0"` if no configuration has been applied yet. |

#### 23.2.2 Neighbor Discovery & Client Telemetry Semantics (`interfaces.clients`)

The `interfaces.clients` list represents **observed L3 network neighbors** rather than an authoritative connected-client inventory. In routed Linux environments, ARP/NDP caches reflect recently resolved IP-to-MAC mappings, meaning idle clients may age out while stale entries can linger.

The collector applies the following rules when processing neighbors:

1. **Neighbor State Filtering:**
   * Entries extracted from `ip -j neigh` with states `FAILED`, `INCOMPLETE`, or `NOARP` are discarded.
   * Only active/valid neighbor states are retained: `REACHABLE`, `PERMANENT`, `STALE`, `DELAY`, and `PROBE`.
2. **Deduplication by MAC Address:**
   * In dual-stack networks, a single client MAC often has both IPv4 and IPv6 (link-local and SLAAC/GUA) addresses.
   * Neighbors are grouped by normalized MAC address (`strings.ToLower(mac)`). Multiple IP addresses are aggregated into the `ipv4_addresses` and `ipv6_addresses` arrays within a single `InterfaceClient` record rather than emitting duplicate client entries.
3. **Bridge and VLAN Interface Attribution:**
   * For bridge interfaces (e.g., `br0`), `ip -j neigh` attributes the neighbor to the bridge master device. Where bridge forwarding table lookup (`bridge -j fdb show`) resolves the MAC to a physical bridge port (e.g., `eth1`), `ports: ["eth1"]` is populated; otherwise, the interface name itself is used (`ports: ["br0"]`).
   * For 802.1Q VLAN interfaces (e.g., `eth1.100`), the client is attributed to the specific VLAN subinterface under which the neighbor was resolved.
4. **`last_seen` Semantics (Elapsed Seconds, Never Unix Epoch):**
   * The OpenWiFi state schema defines `last_seen` as elapsed duration ("How long ago was traffic from this neighbour last seen."), mirroring AP firmware implementations (e.g., `now_monotonic - mac.ts` in seconds from `udevmand/src/mac.c`). It MUST represent elapsed seconds since last reliably observed activity at state collection time, and MUST NEVER be populated with a Unix epoch timestamp.
   * **Use `0` Only for Verified Fresh Activity:** Use `0` only if fresh activity is actually known (e.g. traffic reliably observed during the current sampling second), NOT merely because an ARP/NDP entry is marked `REACHABLE`.
   * **Omission when Age is Unmeasurable:** Kernel `ip -j neigh` state alone is not a last-packet timestamp. If activity age is not measurable or known, the collector MUST omit this optional field (`omitempty`) instead of inventing a timestamp or speculative age. No new MAC tracking subsystem is needed in v1.
   * **Schema-Valid Omission:** In `TIP-olg-ucentral-schema` (`ucentral.state.pretty.json`), `last_seen` is an optional property of `#/$defs/interface.client`. Omitting the field produces valid JSON schema output.
5. **Reconciliation for Authoritative Connected Clients:**
   * When an authoritative connected-device inventory is required by the Gateway/UI, the collector reconciles multiple sources:
     * **L2 Physical Link Presence:** Learned dynamic MACs from `bridge -j fdb show` and Wi-Fi station dumps (`nl80211`/`iw`).
     * **L3 Address Assignment:** Active DHCP lease files (e.g., Kea/ISC DHCP leases).
     * **L3 Activity:** Filtered ARP/NDP cache (`ip -j neigh`).
   * Merging these sources produces a unified inventory containing client hostnames, lease expiration, verified L2 port presence, and L3 activity timestamps.
6. **Required Test Matrix (`interfaces.clients` & `last_seen`):**
   * `TestNeighbor_KnownAge`: Neighbor with measurable traffic activity includes `last_seen` as elapsed seconds ($\ge 0$).
   * `TestNeighbor_UnknownAge_Omitted`: Neighbor sourced from ARP/NDP cache where last-packet age is not measurable omits `last_seen` entirely (`omitempty`), ensuring no fabricated age or epoch timestamp is published.
   * `TestNeighbor_SchemaValidation_Omission`: Validates that serializing `interfaces.clients` without `last_seen` passes OpenWiFi state schema validation.

#### 23.2.3 Applied Configuration UUID Checkpoint & Concurrency Semantics

The configuration UUID published in `StateEnvelope.UUID` correlates the router's active operational state with the controller's configuration transactions:

1. **Source & Type (`state.State{AppliedUUID string}`):** Sourced from the local state checkpoint (`internal/state/store.go`). It is represented as a string on the NATS envelope (carrying decimal timestamp strings such as `"1793880893"`).
2. **Only Last Successfully Applied UUID:** The agent updates `AppliedUUID` only after a configuration transaction is committed successfully (`Result: success`). If an apply attempt fails validation or triggers a rollback (`Result: rolled_back`), `AppliedUUID` preserves the last-known-good active UUID. Failed or pending configurations are never reported.
3. **Distinguishing Absent/Corrupt Checkpoint from `UUID = "0"`:**
   * **`UUID = "0"`:** Explicitly denotes that the device has applied an initial/baseline or factory-default configuration.
   * **Absent Checkpoint (file missing):** Fresh, unconfigured boot before any configuration has occurred; reported as empty `UUID: ""`.
   * **Corrupt Checkpoint (unparseable JSON on disk):** Treated as empty `UUID: ""` and logged as a warning, preventing collector crashes.
4. **Race-Free Checkpoint Access (Concurrency Safety):** `FileStore.Save()` writes to a temporary file (`.state-*.tmp`), syncs to disk (`tmpFile.Sync()`), and replaces the destination using a POSIX atomic rename (`os.Rename()`). Consequently, concurrent reads by `FileStore.Load()` during state collection are completely race-free and will never read torn or partially written data.

#### 23.2.4 Interface Delta Counter Computation & Reset Semantics (`interfaces.delta_counters`)

To mirror OpenWiFi AP behavior and prevent counter corruption or graph spikes, the state collector implements baseline tracking and reset detection for `interfaces.delta_counters`:

1. **In-Memory Baseline per Interface:** The collector maintains an in-memory map keyed by interface name / `ifindex`:
   `periodic_counters[iface] = { rx_bytes, tx_bytes, rx_packets, tx_packets, last_sampled_at }`
2. **First Sample Behavior (Cold Boot / New Interface):** On agent startup or when an interface is newly discovered, establish the baseline sample. Set all `delta_counters` fields to `0` on the initial report to prevent historical cumulative counters from appearing as a single-interval traffic spike.
3. **Reset & Underflow Detection (`current < previous`):** If an interface restarts (`down`/`up`), a VLAN/tunnel interface is recreated, or the driver counter resets, `current < previous` occurs. The collector must detect this condition, treat `current` as the new baseline, and emit `delta = 0` (or `current` if resetting from zero). It MUST NOT perform direct unsigned subtraction (`current - previous`), which causes a massive `uint64` integer underflow.
4. **Interface Pruning & Lifecycle:** If an interface disappears from the system, its baseline entry is purged from memory. If it reappears later, it starts fresh from Step 2 with a zero-delta baseline.
5. **Periodic Baseline Isolation (No Mutation by On-Demand Queries):** The periodic reporting loop has exclusive write ownership over `periodic_counters`. An on-demand live query (`state.get.<target>`) reads current kernel counters and computes transient delta values against `periodic_counters` for the on-demand response, but MUST NOT overwrite or advance `periodic_counters` or its timestamp. This ensures that the scheduled periodic reporting interval (e.g. 60s) and its bandwidth delta metrics are never distorted by ad-hoc queries.
6. **Elapsed Time Measurement:** The collector records `last_sampled_at` with each baseline, ensuring accurate elapsed time $\Delta t$ calculation across scheduled intervals.
7. **Required Unit Test Matrix:** Implement unit test coverage for:
   * **Initial Baseline:** First sample produces zero deltas.
   * **Monotonic Increments:** Normal delta calculation when `current >= previous`.
   * **Counter Reset:** `current < previous` safely resets baseline and emits zero delta without underflow.
   * **Interface Lifecycle:** Missing interfaces are pruned; reappearing interfaces establish a new baseline.
   * **On-Demand Query Isolation:** Handling `state.get` queries between periodic samples computes transient deltas without advancing or mutating the periodic baseline; the subsequent scheduled periodic sample measures the full interval.
   * **Reconnect Gaps:** Variable interval timing preserves accurate delta rates.

#### 23.2.5 Dynamic Interval Propagation & Cloud Lifecycle Control (`metrics.statistics.interval`)

To eliminate dual-timer desynchronization and ensure Cloud configuration changes take effect immediately:

1. **Authoritative Cloud Source of Truth with Local Fallback:**
   * **Source of Truth:** The VyOS agent treats the last successfully applied Cloud configuration's `metrics.statistics.interval` as the authoritative source of truth for its periodic telemetry sampling frequency.
   * **Local Startup & Omission Fallback:** The collector initializes with a local configuration default of 60 seconds, configured via `agent.state.interval: 60s` (default: 60s). This fallback applies on initial cold boot (before any Cloud configuration has been applied) and whenever an applied Cloud configuration omits `metrics.statistics.interval`.
2. **Schema Validation & Forwarding Boundary:**
   * Ingress configurations are validated by the uCentral client against `TIP-olg-ucentral-schema` (`ucentral.schema.pretty.json`). Because the schema strictly enforces an integer `minimum: 60` for `metrics.statistics.interval`, any Cloud configuration containing `interval < 60` (including `interval: 0` or negative values) fails schema validation at ingress and is rejected.
   * Only schema-validated desired configurations are forwarded to JetStream KV (`desired.<target>`). Payloads with invalid intervals are never written to KV.
3. **Dynamic Ticker Adjustment on Apply:**
   * When a validated configuration containing `metrics.statistics.interval >= 60` is successfully committed and applied, the state collector's background ticker is dynamically adjusted (`ticker.Reset(time.Duration(interval) * time.Second)`), overriding any previous interval.
4. **Periodic State Disablement Status (Future Extension):**
   * The OpenWiFi configuration schema currently enforces `minimum: 60` and does not define a schema-valid disablement mechanism (such as an explicit boolean flag). Pushing `interval: 0` is schema-invalid and rejected at ingress.
   * Turning off periodic telemetry from the Cloud is therefore not supported under the current schema and is designated as a future extension pending an approved schema amendment.
   * The agent publishes periodic telemetry continuously at the applied interval ($\ge 60$s) or 60s fallback.
5. **On-Demand Live Queries Independent (`state.get.<target>`):**
   * On-demand live state queries (`state.get.<target>`) operate completely independently of periodic state publishing and periodic intervals.
   * Live queries execute via the dedicated worker pool (5 workers, 3s timeout) without mutating the periodic delta baseline or being constrained by periodic ticker frequency.
6. **Rollback Resilience:**
   * If a configuration transaction fails rendering or application and triggers a rollback (`Result: rolled_back`), the collector preserves or restores the sampling interval from the last successfully applied configuration (or falls back to the local 60s default).
7. **Required Test Matrix:**
   * `TestInterval_ValidMinimum`: Validates that a Cloud configuration with `metrics.statistics.interval: 60` adjusts the collection ticker to 60s.
   * `TestInterval_ValidCustom`: Validates that a Cloud configuration with `metrics.statistics.interval: 120` adjusts the collection ticker to 120s.
   * `TestInterval_MissingOmissionFallback`: Validates that a Cloud configuration omitting `metrics.statistics.interval` causes the collector to maintain the last-applied interval or fall back to the local 60s default.
   * `TestInterval_RejectedZero`: Validates that a desired configuration with `metrics.statistics.interval: 0` or `< 60` fails uCentral client schema validation, is rejected before JetStream KV write, and leaves the VyOS agent's active interval unchanged.
   * `TestInterval_RollbackPreservation`: Validates that a failed configuration transaction rolling back preserves the sampling interval from the previously applied configuration.
   * `TestInterval_OnDemandIndependence`: Validates that on-demand queries on `state.get.<target>` execute and return live telemetry independently of periodic interval settings.

#### 23.2.6 On-Demand Collection Concurrency & Execution Boundary (`state.get.<target>`)

To protect the router from overload and prevent starvation of configuration and action commands:

1. **Configurable Execution Capacity:**
   * Configured via `agent.state.max_concurrent_requests` (default: `5`) and supplied during `agentcore` initialization (`Config.StateGetWorkers = 5`).
   * The VyOS agent alone owns this execution capacity and concurrency limit. The uCentral Client does not duplicate or mirror this setting.
2. **Immediate Rejection on Saturation (No Waiting Queue):**
   * If all 5 worker slots are actively executing state collections when a new query arrives on `state.get.<target>`, the agent must NOT queue or buffer the request.
   * The agent immediately returns a typed, serialized `agentcore.StateGetReply` error response to the ephemeral NATS reply inbox (`msg.Reply`):
     ```json
     {
       "version": "1.0",
       "target": "vyos",
       "error": {
         "code": "service_busy",
         "message": "All state collection workers are occupied"
       }
     }
     ```
   * **Subject & Correlation Invariants:** The reply is sent strictly to the NATS ephemeral reply inbox (`msg.Reply`). It is NEVER published to `result.<target>` or `status.<target>`. The native NATS reply inbox handles correlation with the requester's `RequestState()` call; no duplicate RPC ID is required.
   * **NATS Wire vs. Cloud Gateway Code Distinction:** The NATS wire protocol uses the string error code `"service_busy"`. Note that the numeric code `2` (`result.status: {"error": 2, "text": "service busy: maximum concurrent state requests reached"}`) is the **Cloud Gateway REST RPC response mapping** applied upstream by the uCentral Client, NOT the NATS wire error code.
3. **Execution Bounding (3-Second Deadline):**
   * Each admitted collection is bounded to a strict 3-second context cancellation (`context.WithTimeout(ctx, 3*time.Second)`).
   * This ensures execution terminates cleanly well within the uCentral Client's 5-second overall NATS deadline (`OLG_TIMEOUT_DISPATCH`).
   * If collection exceeds 3 seconds, the agent returns `StateGetReply` with `error.code: "collection_timeout"`.
4. **Nonblocking NATS Subscription Callback:**
   * The NATS subscription callback for `state.get.<target>` performs only initial validation and instantaneous capacity admission (acquiring a semaphore slot).
   * Once admitted, collection is immediately handed off to an asynchronous worker goroutine.
   * The callback must NEVER run the 3-second collection inline on the NATS subscription dispatcher thread. This guarantees that configure (`cmd.configure.<target>`) and action (`cmd.action.<target>.*`) subscriptions are never blocked or starved by on-demand state requests.
5. **Read-Only Baseline Isolation:**
   * On-demand collection is strictly read-only with respect to `periodic_counters`. It computes transient deltas for the response, but never mutates the periodic baseline or its timestamp.
6. **Capacity Release & Observability:**
   * Worker capacity is strictly released on every exit path (`defer sem.Release()`): success, kernel read error, 3-second timeout, or panic recovery.
   * Observability metrics: Expose active state workers gauge (`state_get_active_workers`), busy rejections (`state_get_busy_total`), and timeouts (`state_get_timeout_total`).
7. **Required Test Matrix:**
   * Concurrency saturation: 5 active collections saturated; the 6th simultaneous request receives an immediate `StateGetReply` with `error.code: "service_busy"` on its NATS reply inbox.
   * Successful reply: Returns `StateGetReply` wrapping valid `StateEnvelope` on ephemeral reply inbox.
   * Collection timeout: Collection taking > 3s aborts and returns `StateGetReply` with `error.code: "collection_timeout"`.
   * Collection error: Internal kernel read failure returns `StateGetReply` with `error.code: "collection_failed"`.
   * Worker slot release verified across all terminal conditions (successful reply, kernel read error, 3-second timeout expiration, and panic recovery).
   * Starvation immunity: During 5 active concurrent state collections, concurrent `configure` or `action` commands are dispatched and executed immediately without delay.

#### 23.2.7 Applied Topology Port Mapping & Role Derivation (`link-state` & `lldp-peers`)

To accurately classify interfaces into WAN (`upstream`) and LAN (`downstream`) without relying on interface name heuristics, routing table queries, or LLDP neighbor types, the state collector derives its topology mapping strictly from the **last successfully applied configuration** and the renderer's resolved port map:

1. **Reusing Applied Configuration Roles & Renderer Port Map:**
   * OpenWiFi desired configuration defines interface roles (`interfaces[].role: "upstream"` or `"downstream"`) and port selectors (`interfaces[].ethernet[].select-ports[]`, e.g. `["WAN*"]`, `["LAN1"]`, `["LAN2"]`).
   * The renderer adapter (`olg-renderer-vyos`) resolves these selectors to physical Linux interfaces using its port map (`WithPortMap`, defaulting to `WAN*` $\rightarrow$ `eth0`, `LAN1` $\rightarrow$ `eth1`, `LAN2` $\rightarrow$ `eth2`, or configured platform override maps).
   * The VyOS state collector derives its physical interface $\leftrightarrow$ role mapping directly from this resolved state of the **last successfully applied configuration**:
     * A physical interface mapped to an interface definition with `role: "upstream"` is classified under `upstream`.
     * A physical interface mapped to an interface definition with `role: "downstream"` is classified under `downstream`.
   * **Strict Source of Truth:** The collector MUST NOT classify WAN/LAN by interface names (e.g. assuming `eth0` is always WAN), kernel routing table entries (`ip route show default`), or LLDP neighbor capabilities.
   * **Failed / Unapplied Config Isolation:** The collector MUST NEVER treat a pending, unapplied, or failed desired configuration as authoritative. If a configuration attempt fails and rolls back, the collector preserves or restores the port mapping from the last-known-good applied configuration. On fresh boot before any Cloud configuration is applied, the collector falls back to the default port map (`WAN*` $\rightarrow$ `eth0` as upstream, `LAN1` $\rightarrow$ `eth1` as downstream).

2. **Physical Ports vs. Logical Interfaces in Telemetry:**
   * **`link-state` (Physical Ports Only):** Physical ports resolved by the applied port mapping are populated with carrier state, negotiated speed (Mbps), and duplex mode under their assigned role (`link-state.upstream.<iface>` or `link-state.downstream.<iface>`).
   * **`lldp-peers` (Local Port Attribution):** Discovered LLDP neighbors are attached to the local receiving physical interface under the same role assigned to that physical interface (`lldp-peers.upstream.<iface>` or `lldp-peers.downstream.<iface>`). Each interface entry contains an array of neighbor peer objects (or `[]` if no peers are heard). Empty LLDP tables (`"upstream": {}`, `"downstream": {}`) are valid.
   * **Bounded Execution:** Sampling LLDP peers via `lldpcli` (e.g., `lldpcli -f json show neighbors`) is executed with a strict bounded timeout (2 seconds) and capped output parsing to prevent hanging or memory exhaustion.
   * **Logical Bridges & VLANs:** Logical interfaces, bridge master devices (`br0`), and 802.1Q subinterfaces (`eth1.100`) remain strictly in the root `interfaces[]` list; they are NOT placed in `link-state` or `lldp-peers`.

3. **OpenWiFi State Schema Validation & Diagnostic Omission:**
   * Serialized interface keys under `link-state.upstream`, `link-state.downstream`, `lldp-peers.upstream`, and `lldp-peers.downstream` must strictly match the OpenWiFi state schema regex `^(eth|lan|wan)[0-9]*$`.
   * **Omission Policy:** For unknown, unclassified, shared/trunk, or unsupported interface names (including bridge devices, VLAN subinterfaces, loopback, or tunnel interfaces), the collector MUST **omit** them from `link-state` and `lldp-peers` rather than guessing a role, and log a diagnostic warning.
   * **Wired-Only Router Profile:** Purely wired appliances without wireless PHYs report `"radios": []` (empty array, never null) so Cloud Gateway associations calculations complete safely.

4. **Required Test Matrix:**
   * `TestPortMap_Override`: Verifies that custom port maps configured via `WithPortMap` (e.g., `WAN*` $\rightarrow$ `eth3`) resolve physical ports to their configured WAN/LAN roles correctly.
   * `TestPortMap_AppliedConfigChange`: Verifies that committing a new configuration with updated interface roles updates the state collector's topology mapping dynamically.
   * `TestPortMap_RollbackPreservation`: Verifies that a failed configuration transaction rolling back preserves the port mapping from the previously applied configuration.
   * `TestState_WiredOnlyRouter`: Verifies that state collection on a wired-only appliance emits `"radios": []` without panic or schema error.
   * `TestLLDP_PortAttribution`: Verifies that LLDP neighbors are attributed to the local receiving physical port under that port's assigned WAN/LAN role.
   * `TestSchemaValidation_PortKeys`: Verifies that all keys in `link-state` and `lldp-peers` strictly match `^(eth|lan|wan)[0-9]*$`, and that non-matching logical/subinterfaces are safely omitted with diagnostic logging.

#### Published State Wire Schema (`agentcore.StateEnvelope`)

The agent wraps the device telemetry snapshot inside a standard `agentcore.StateEnvelope` and publishes it via `client.PublishState(ctx, envelope)`. The outer envelope standardizes wire versioning, target identity, applied config UUID correlation, and timestamps, while the inner `payload` carries pure OpenWiFi-compliant router telemetry (excluding inner `serial` or `uuid` to avoid inner/outer identity ambiguity):

```json
{
  "version": "1.0",
  "uuid": "1793880893",
  "target": "vyos",
  "payload": {
    "version": 1,
    "unit": {
      "load": [ 0.15, 0.08, 0.05 ],
      "cpu_load": [ 4, 5, 3, 4, 4 ],
      "localtime": 1793968533,
      "memory": {
        "total": 4124565504,
        "free": 3201454080,
        "cached": 450123776,
        "buffered": 89456640
      },
      "uptime": 119483
    },
    "interfaces": [
      {
        "name": "eth0",
        "location": "/interfaces/0",
        "uptime": 23061,
        "ipv4": {
          "addresses": [ "172.16.1.228/21" ],
          "dhcp_server": "172.16.0.1",
          "leasetime": 43200
        },
        "counters": {
          "rx_bytes": 1051992836,
          "rx_packets": 5440208,
          "tx_bytes": 4403888,
          "tx_packets": 27885,
          "rx_dropped": 0,
          "rx_errors": 0,
          "tx_dropped": 0,
          "tx_errors": 0,
          "multicast": 884271,
          "collisions": 0
        },
        "delta_counters": {
          "rx_bytes": 659383,
          "rx_packets": 3852,
          "tx_bytes": 4141,
          "tx_packets": 17
        },
        "clients": [
          {
            "mac": "00:09:0f:09:80:04",
            "ipv4_addresses": [ "172.16.0.1" ],
            "ports": [ "eth0" ],
            "last_seen": 0
          }
        ]
      }
    ],
    "link-state": {
      "upstream": {
        "eth0": { "carrier": true, "speed": 1000, "duplex": "full" }
      },
      "downstream": {
        "eth1": { "carrier": false, "speed": 0 }
      }
    },
    "lldp-peers": {
      "upstream": {},
      "downstream": {}
    },
    "radios": []
  },
  "timestamp": "2026-05-12T10:30:00Z"
}
```

#### Lifecycle & Concurrency Rules

1. **Independent Routine:** The State Collector runs in a dedicated background goroutine started during `Runtime.Run(ctx)`.
2. **Delta Calculation:** The collector caches the previous interface counter snapshot in memory. On each tick, it computes the delta before updating the stored cache.
3. **Non-Blocking Sampling:** Collecting metrics from `/proc` and `/sys` is done using fast direct file reads or netlink calls with a strict 3-second timeout to avoid locking agent threads.
4. **Publish Failure Resilience:** If NATS publishing fails (e.g. during broker reconnects), the collector logs a warning, preserves counter baselines, and continues on the next scheduled interval without crashing the daemon.

## 24. Startup reconcile

Startup reconcile is implemented in the configure service and runs on agent startup. The agent loads the latest desired configuration from NATS JetStream KV and aligns the local applied configuration state during agent initialization.

After `agentcore.Client.Start(ctx)`, the agent runs startup reconcile.

Minimal behavior:

```text
1. Call StartupReconcile(ctx, target) or LoadDesiredConfig(ctx, target).
2. If no desired config exists, continue normally.
3. If desired UUID equals local applied UUID, continue normally.
4. If desired UUID differs, run the same render/apply/result lifecycle.
```

### Startup reconcile failure policy

Startup reconcile runs after `agentcore.Client.Start(ctx)` succeeds.

Fatal startup errors:
- config path cannot be resolved
- YAML config cannot be loaded or validated
- config cannot be converted to `agentcore.Config`
- `agentcore.Client` cannot be created
- `agentcore.Client.Start(ctx)` fails
- local state store cannot be initialized safely

Non-fatal reconcile errors:
- no desired config exists in KV
- desired config already matches local applied UUID
- desired config load times out after startup
- renderer fails during reconcile
- apply engine fails during reconcile
- result/status publish fails after a reconcile attempt

For non-fatal reconcile errors, the agent must:
1. publish degraded/failure status when possible,
2. not update local applied UUID unless apply succeeds,
3. continue running and keep handling future configure notifications/actions,
4. allow a later startup reconcile or explicit recover path to converge from KV.

### Reconnection reconcile

If the NATS connection drops and reconnects during runtime, configuration updates that occurred on the controller while the agent was offline must be synced. The agent registers a reconnect callback that runs `Reconcile(...)` asynchronously (in a new goroutine) when the NATS session is reconnected, ensuring that it doesn't block the NATS client reconnect loop.

## 25. Handler concurrency

Configure apply must be serialized.

For milestone 1, direct synchronous handler execution is acceptable because placeholder work is fast.

The code must still be structured so a future phase can move configure processing to a single worker queue.

## 26. Integration tests

Integration tests must use real `nats-server -js`.

They must not use raw NATS publish for the primary test flow.

Use public `agentcore` APIs for controller-side behavior.

### 26.1 Configure end-to-end

```text
1. Start real nats-server -js.
2. Start vyos-nats-agent with test config.
3. Create controller agentcore.Client with same NATS server and KV bucket.
4. Register controller result handler.
5. Controller calls SubmitConfigure.
6. VyOS agent receives configure notification.
7. VyOS agent loads desired config from KV.
8. Placeholder renderer runs.
9. Placeholder apply engine runs.
10. Local applied UUID is updated.
11. VyOS agent publishes result.
12. Controller receives result.
13. Assert rpc_id, target, uuid, result.
```

### 26.2 Action end-to-end

```text
1. Start real nats-server -js.
2. Start vyos-nats-agent with test config.
3. Create controller agentcore.Client with same NATS server and KV bucket.
4. Register controller result handler.
5. Controller calls SubmitAction for trace.
6. VyOS agent receives action.
7. Placeholder trace executor runs.
8. VyOS agent publishes result.
9. Controller receives result.
10. Assert rpc_id, target, action, result.
```

### 26.3 Startup reconcile

This integration test verifies startup reconcile logic.

```text
1. Start real nats-server -js.
2. Controller stores desired config through SubmitConfigure or StoreDesiredConfig.
3. Start vyos-nats-agent.
4. Agent runs startup reconcile.
5. Agent applies latest desired config.
6. Agent updates local applied UUID.
```

### 26.4 Reconnection reconcile

This integration test verifies reconnection reconcile logic.

```text
1. Start real nats-server -js.
2. Start TCP proxy forwarding to NATS server.
3. Start vyos-nats-agent pointing to the proxy.
4. Pause the TCP proxy to simulate connection drop (agent goes offline).
5. Controller writes a new desired config version directly to NATS KV.
6. Resume the TCP proxy (agent reconnects automatically).
7. Agent triggers asynchronous reconciliation.
8. Assert reconciliation counter is incremented, logs verify execution, local state file is updated, and success result is published.
```

### 26.5 Startup reconcile failure

This integration test verifies that the agent behaves correctly when startup reconciliation fails.

```text
1. Start real nats-server -js.
2. Direct write of malformed/invalid JSON string to JetStream KV.
3. Start vyos-nats-agent.
4. Agent fails to load or parse desired config during startup reconcile.
5. Agent publishes degraded/failure status (stage "failed") to NATS.
6. Agent continues running and does not crash.
```

Current repository smoke scripts:

- `tests/smoke/real-nats-configure-smoke.sh`
- `tests/smoke/real-nats-action-smoke.sh`

These scripts run the placeholder-safe path. Real VyOS apply smoke tests, if added, must be manual/lab-only and guarded by an explicit opt-in flag.

## 27. Development phases

### Phase 1: Bootstrap and config loader

Implement:

- Go module
- CLI entrypoint
- YAML config loader
- config defaults
- config validation
- conversion to `agentcore.Config`
- `--validate-config` CLI mode

Do not connect to NATS in this phase.

### Phase 2: Agent lifecycle

Implement:

- `agentcore.New(...)`
- configure/action handler registration
- `Start(ctx)`
- startup status publication
- signal handling
- graceful shutdown

Do not implement configure apply yet.

### Phase 3: Configure flow with placeholders

Implement:

- `RegisterConfigureHandler(target, handler)`
- `LoadDesiredConfig(ctx, target)`
- placeholder renderer
- placeholder apply engine
- local applied UUID state
- configure result/status publishing

### Phase 4: Action flow with one action

Implement:

- `RegisterActionHandler(target, "trace", handler)`
- placeholder and real trace executor (`VyOSTraceExecutor`)
- action result/status publishing

### Phase 5: Integration tests

Implement integration tests for:

- configure flow
- action flow
- startup reconcile/latest desired config recovery

### Phase 6: Real configure backend

Implement:

- `agent.configure.mode: placeholder | real`
- real renderer adapter using `olg-renderer-vyos/renderer`
- real apply adapter using `olg-renderer-vyos/apply`
- dependency on `olg-nats-agent-core` module tag `v0.1.0`
- dependency on `olg-renderer-vyos` module tag `v0.1.0`

The core and renderer dependencies are resolved through normal Go module resolution.

## 28. Codex implementation rules

Codex must follow these rules:

- Work one phase at a time.
- Do not generate future phase code early.
- Do not hardcode config values.
- Do not bypass `olg-nats-agent-core`.
- Do not use raw NATS APIs in production code.
- Keep public APIs small.
- Keep handlers thin.
- Keep renderer/apply/action logic behind interfaces.
- Keep config loader deterministic and well-tested.
- Keep placeholder implementations available unless explicitly asked to remove them.
- Use structured errors with context.
- Run `gofmt` on changed Go files.
- Run `go test ./...` after each phase.
- Run integration tests only when NATS server is available.
- Do not claim tests passed unless they were actually run.

## 29. Acceptance criteria for milestone 1

Milestone 1 is complete when:

```text
[x] Agent starts from YAML config.
[x] Agent constructs agentcore.Config from YAML config.
[x] Agent connects to NATS.
[x] Agent registers configure handler for target "vyos".
[x] Agent registers action handler for "trace".
[x] Agent publishes startup status.
[x] Agent handles configure notification.
[x] Agent loads desired config from KV.
[x] Placeholder renderer runs.
[x] Placeholder apply engine runs.
[x] Applied UUID is saved to local state.
[x] Agent publishes configure result.
[x] Agent handles trace action.
[x] Agent publishes action result.
[x] Integration test proves configure end-to-end.
[x] Integration test proves action end-to-end.
[x] Integration test proves startup reconcile or latest desired config recovery.
[x] Integration test proves reconnection reconciliation on connection recovery.
[x] Integration test proves startup reconcile failure handling.
```
