# Phase 8 Production Readiness Coverage

This table maps `TDD_SPEC.md` Phase 8 production-readiness requirements to current tests and documented policy deferrals.

## Logging And Security

| ID | Status | Test file | Test name or existing coverage | Notes |
|---|---|---|---|---|
| LOG-001 | Covered | `internal/configure/logging_security_test.go` | `TestLoggingInfoLevelDoesNotLogPayload` | Default configure logs include payload size metadata, not raw payload JSON or secret-looking values. |
| LOG-002 | Covered | `internal/configure/logging_security_test.go` | `TestLoggingInfoLevelDoesNotLogRenderedCommands` | Default configure logs include rendered size/count metadata, not raw rendered command text. |
| LOG-003 | Covered | `internal/applyvyos/adapter_boundary_test.go` | `TestApplyAdapterLogsPlanFieldsSafely` | Apply adapter info logs include plan counts/flags and exclude raw plan command arrays. |
| LOG-004 | Covered | `internal/configure/logging_security_test.go` | `TestLoggingDebugWithPayloadFlagDoesNotLogRawPayload` | Explicit configure debug payload flag emits payload metadata only and omits raw payload bodies. |
| LOG-005 | Covered | `internal/configure/logging_security_test.go` | `TestLoggingDebugWithoutPayloadFlagDoesNotLogPayload` | Logger presence/debug capability alone does not emit payload unless `LogPayloads` is enabled. |
| LOG-006 | Covered | `internal/agent/logging_security_test.go` | `TestLoggingPayloadFlagWithoutDebugDoesNotLogPayload` | Runtime debug config suppresses raw-data flags unless logging level is `debug`. |
| LOG-007 | Covered | `internal/agent/logging_security_test.go` | `TestLoggingPartialDebugConfigDoesNotEmitDebugLogs` | Debug level alone does not enable payload/rendered/apply-plan raw-data logging. |
| LOG-008 | Covered | `internal/configure/logging_security_test.go` | `TestLoggingInfoLevelDoesNotLogPayload`, `TestLoggingInfoLevelDoesNotLogRenderedCommands`, `TestFailureLogsContainSafeErrorContext` | There is no redaction layer today; default safety is achieved by not logging raw payloads/commands/dependency error text. |
| LOG-009 | Covered | `internal/configure/logging_security_test.go` | `TestLoggingLargePayloadDoesNotCrash` | Large payload configure path completes and logs safe size metadata only. |
| LOG-010 | Covered | `internal/configure/logging_security_test.go` | `TestLoggingLargePayloadDoesNotConvertUnnecessarilyToString` | Avoids brittle allocation assertions and verifies default observable contract: large payload body is absent from logs. |
| LOG-011 | Covered | `internal/configure/logging_security_test.go` | `TestFailureLogsContainSafeErrorContext` | Failure logs include target/UUID/RPC/stage/error code and avoid raw payload/dependency error details. |

## Restart And Persistence

| ID | Status | Test file | Test name or existing coverage | Notes |
|---|---|---|---|---|
| RST-001 | Covered | `internal/configure/restart_persistence_test.go` | `TestRestartWithPersistedStateDoesNotReapplySameUUID` | New service/store instance loads persisted UUID and skips duplicate apply. |
| RST-002 | Covered | `internal/configure/restart_persistence_test.go` | `TestRestartWithMissingStateReappliesConfig` | Missing state file is treated as no checkpoint and desired config is applied/saved safely. |
| RST-003 | Covered | `internal/configure/restart_persistence_test.go` | `TestRestartWithCorruptStateFailsSafely` | Corrupt state stops before render/apply and publishes `state_load_failed`. Also supported by Phase 2 state corruption tests. |
| RST-004 | Covered | `internal/configure/restart_persistence_test.go` | `TestRestartReadsStateBeforeConfigureDecision` | State load is asserted before render/apply decision. |
| RST-005 | Covered | `internal/state/restart_persistence_test.go` | `TestRestartDoesNotLoseConfiguredStateWhenPathPersistent` | New file-store instance reads persisted UUID from the same path. |
| RST-006 | Deferred | n/a | n/a | Current default config intentionally uses `/tmp/vyos-nats-agent/state.json` for local/dev bootstrap. Production state path policy belongs to deployment/config hardening, not Phase 8 behavior tests. |

## Retry And Idempotency

| ID | Status | Test file | Test name or existing coverage | Notes |
|---|---|---|---|---|
| IDEMP-001 | Covered | `internal/configure/configure_workflow_test.go` | `TestConfigureWorkflowRepeatedSameUUIDIsIdempotent`, `TestConfigureWorkflowAlreadyInSyncSkipsApply` | Same UUID after success skips render/apply/save. |
| IDEMP-002 | Covered | `internal/configure/idempotency_retry_test.go` | `TestSameUUIDAfterRendererFailureRetries` | Renderer failure does not checkpoint; retry renders again and succeeds. |
| IDEMP-003 | Covered | `internal/configure/configure_failure_test.go` | `TestConfigureApplyFailureAllowsRetrySameUUID` | Apply failure does not checkpoint; retry applies and saves. |
| IDEMP-004 | Covered | `internal/configure/configure_failure_test.go` | `TestConfigureStateSaveFailureRetriesSafely` | State-save failure does not persist UUID; retry renders/applies/saves again. |
| IDEMP-005 | Covered | `internal/configure/idempotency_retry_test.go` | `TestDuplicateConfigureEventsDoNotDoubleApplyAfterSuccess` | Duplicate same-UUID event after success does not double apply. |
| IDEMP-006 | Covered | `internal/configure/configure_workflow_test.go` | `TestConfigureWorkflowNewUUIDTriggersRenderApplyAndStateUpdate` | New UUID after old success renders/applies/saves again. |
| IDEMP-007 | Deferred | n/a | n/a | Desired UUIDs are opaque identities and there is no monotonic ordering/rollback policy today. Preventing older UUID rollback requires an explicit product ordering policy. |
| IDEMP-008 | Covered | `internal/configure/idempotency_retry_test.go` | `TestRetryDoesNotPublishDuplicateSuccessForSameAttempt` | A single success attempt publishes one success result; failed attempt publishes no success before retry. |

## Concurrency And Race

| ID | Status | Test file | Test name or existing coverage | Notes |
|---|---|---|---|---|
| CONC-001 | Covered | `internal/configure/concurrency_race_test.go` | `TestConcurrentConfigureEventsSameUUIDApplyAtMostOnce` | Concurrent same-UUID events apply/checkpoint once due configure service serialization and state check. |
| CONC-002 | Deferred | n/a | n/a | Different-UUID concurrent ordering has no explicit product policy beyond serialized processing; final state follows lock acquisition order. Deterministic ordering belongs to queue/ordering design. |
| CONC-003 | Covered | `internal/configure/concurrency_race_test.go` | `TestConfigureAndActionCanOverlapSafely` | Configure and action services run concurrently with fake dependencies and publish success. |
| CONC-004 | Covered | `internal/state/restart_persistence_test.go` and CI | `TestConcurrentStateAccessNoRace`; `go test -race ./...` | File-store concurrent access is bounded and race detector remains the primary enforcement. |
| CONC-005 | Covered | `internal/configure/concurrency_race_test.go` | `TestBurstConfigureEventsNoPanicNoRace` | Bounded duplicate configure burst leaves valid final state and one apply. |
| CONC-006 | Covered | `internal/actions/action_large_payload_test.go` | `TestBurstActionEventsNoPanicNoRace` | Bounded action burst completes with one success result per action. |

## Large Payload And Lightweight Load

| ID | Status | Test file | Test name or existing coverage | Notes |
|---|---|---|---|---|
| PERF-001 | Covered | `internal/configure/large_payload_test.go` | `TestLargeConfigOneThousandCommandsDoesNotCrash` | One thousand rendered commands are applied intact and produce success. |
| PERF-002 | Covered | `internal/configure/large_payload_test.go`; `internal/configure/logging_security_test.go` | `TestLargePayloadDoesNotCauseUnsafeLoggingAllocation`; `TestLoggingLargePayloadDoesNotConvertUnnecessarilyToString` | Large payload is preserved through render and not emitted in default logs. |
| PERF-003 | Covered | `internal/configure/concurrency_race_test.go` | `TestBurstConfigureEventsNoPanicNoRace` | Bounded rapid configure events complete without panic/race and keep state valid. |
| PERF-004 | Covered | `internal/configure/large_payload_test.go` | `TestLargeRenderedOutputHandledSafely` | Large rendered command text reaches apply backend unchanged. |
| PERF-005 | Covered | `internal/actions/action_large_payload_test.go` | `TestLargeActionPayloadRejectedOrHandledSafely` | Current placeholder trace policy accepts valid JSON; large valid action payload completes safely. |

## CI/CD Mapping

| Item | Value |
|---|---|
| Normal tests | `go test ./...` in `Test And Build` job |
| Race tests | `go test -race ./...` in `Test And Build` job |
| Build | `go build ./...` in `Test And Build` job |
| Integration tests | Existing `Mocked integration tests` step runs `go test -tags=integration ./...` |
| Phase 8 CI change | None required; all new Phase 8 tests are normal Go tests covered by existing unit and race steps. |
