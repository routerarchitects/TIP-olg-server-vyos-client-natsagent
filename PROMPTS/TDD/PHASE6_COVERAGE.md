# Phase 6 Renderer and Apply Adapter Coverage

This table maps `TDD_SPEC.md` Phase 6 adapter-boundary requirements to the current tests.

## Renderer Adapter

| ID | Status | Test file | Test name | Notes |
|---|---|---|---|---|
| RAD-001 | Covered | `internal/renderervyos/adapter_boundary_test.go` | `TestRendererAdapterMinimalPayloadMapsCorrectly` | Verifies minimal desired config target, UUID, payload, backend call count, and rendered output mapping. |
| RAD-002 | Covered | `internal/renderervyos/adapter_boundary_test.go` | `TestRendererAdapterLargePayloadMapsCorrectly` | Verifies deterministic large JSON payload pass-through without truncation or corruption. |
| RAD-003 | Covered | `internal/renderervyos/adapter_boundary_test.go` | `TestRendererAdapterInvalidPayloadReturnsError` | Invalid JSON fails during adapter input build and backend render is not called. |
| RAD-004 | Covered | `internal/renderervyos/adapter_boundary_test.go` | `TestRendererAdapterPreservesUUID` | Verifies desired UUID reaches backend input and backend UUID maps back to internal output. |
| RAD-005 | Covered | `internal/renderervyos/adapter_boundary_test.go` | `TestRendererAdapterPreservesTarget` | Verifies desired target reaches backend input and backend target maps back to internal output. |
| RAD-006 | Not Applicable | n/a | n/a | The renderer adapter boundary has no RPC/correlation field today. `agentcore.StoredDesiredConfig` includes `rpc_id`, but the external `vyosrenderer.Input` and internal `renderer.Output` types do not carry it. Configure-service correlation is covered in Phase 3/4. |
| RAD-007 | Covered | `internal/renderervyos/adapter_boundary_test.go` | `TestRendererAdapterPropagatesRendererError` | Verifies backend render errors are wrapped with adapter context and no output is returned. |
| RAD-008 | Covered | `internal/renderervyos/adapter_boundary_test.go` | `TestRendererAdapterDoesNotMutatePayload` | Verifies original desired payload bytes remain unchanged even if backend mutates received input bytes. |

## Apply Adapter

| ID | Status | Test file | Test name | Notes |
|---|---|---|---|---|
| AAD-001 | Covered | `internal/applyvyos/adapter_boundary_test.go` | `TestApplyAdapterRenderedOutputMapsToApplyInput` | Verifies internal rendered target, UUID, and command text map to `vyosapply.Input`. |
| AAD-002 | Covered | `internal/applyvyos/adapter_boundary_test.go` | `TestApplyAdapterCallsPrepareWhenSupported` | Verifies `Prepare` is called once before `Apply` when the backend supports it. |
| AAD-003 | Covered | `internal/applyvyos/adapter_boundary_test.go` | `TestApplyAdapterLogsPlanFieldsSafely` | Verifies default info logs include safe plan summary fields and exclude raw plan command strings. |
| AAD-004 | Covered | `internal/applyvyos/adapter_boundary_test.go` | `TestApplyAdapterCallsApplyExactlyOnce` | Verifies successful apply calls backend `Apply` exactly once. |
| AAD-005 | Covered | `internal/applyvyos/adapter_boundary_test.go` | `TestApplyAdapterPropagatesPrepareError` | Verifies prepare errors are returned with adapter context and `Apply` is not called. |
| AAD-006 | Covered | `internal/applyvyos/adapter_boundary_test.go` | `TestApplyAdapterPropagatesApplyError` | Verifies apply errors are returned with adapter context after successful prepare. |
| AAD-007 | Covered | `internal/applyvyos/adapter_boundary_test.go` | `TestApplyAdapterPrepareDoesNotMutateInputForApply` | Verifies `Apply` receives the original intended input and rendered output remains unchanged after prepare. |
| AAD-008 | Covered | `internal/applyvyos/adapter_boundary_test.go` | `TestApplyAdapterBackendWithBothPrepareAndApplyUsesCorrectInput` | Verifies both `Prepare` and `Apply` receive the correct target, UUID, and rendered commands. |
| AAD-009 | Covered | `internal/applyvyos/adapter_boundary_test.go` | `TestApplyAdapterDoesNotApplyWhenPrepareFails` | Verifies prepare failure stops before backend `Apply`. |
| AAD-010 | Covered | `internal/applyvyos/adapter_boundary_test.go` | `TestApplyAdapterHandlesNilOrEmptyPlanSafely` | The plan type is a struct, so nil is not representable at this boundary. The test covers zero-value/empty plan behavior and verifies no panic or skipped apply. |
| AAD-011 | Covered | `internal/applyvyos/adapter_boundary_test.go` | `TestApplyAdapterDoesNotLogRenderedCommandsByDefault` | Verifies default adapter logs do not include raw rendered command text. |

## Deferred Or Not Applicable Items

- `RAD-006` is not applicable to the current renderer adapter contract because neither the external renderer input nor the internal renderer output contains an RPC/correlation field. Adding such a field would be a production contract/design change and is outside Phase 6.
