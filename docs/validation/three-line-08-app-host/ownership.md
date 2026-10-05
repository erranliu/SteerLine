# Task 80 ownership and migration map

Starting tree: `c7eac10aab147fbb9aeaefb9341d5c979d2201c3` (#79 / PR #223).

## Production ownership

| Previous files or definition | Current owner | Consumers and notes |
| --- | --- | --- |
| `internal/steerlineapp/**` | `internal/app/**` | Config parsing, concrete directory/SQLite/client construction, process capability construction, task/execution port injection and command wiring. Existing consumers and tests switch in the same change. |
| App configuration preflight from `internal/steerline/**` | `internal/app/**` | `application_config.go`, `service_prepare.go`, `service_preflight.go` and `state_preflight.go` run at the composition boundary and are supplied to host through `ApplicationAdmission`. |
| Project JSON previously decoded into `execution.Config` | `internal/app/project_config_input.go` (`projectConfigInput`) | `internal/app/config.go` strictly decodes the app-owned disk input, then composes execution and host values. |
| Host-facing fields previously carried by broad `execution.Config` | `internal/app/host.Config` | Repository/instance/timing facts plus explicit runtime, state-validation, process-facts, capability-validation and control-validation ports. Host admission and `ServiceHost` use only this value. |
| Lock-owning host factory previously returned concrete `*execution.Service` | `internal/app/host.ServiceLifetime` | Host calls `Run` and always calls `Close` on the transferred lifetime; app constructs the real service and SQLite store. |
| Execution record reader and file resource writer formerly stored on `execution.Config` | `execution.ServiceStorage` and `execution.Service` | App transfers record access, file-resource access and store close ownership through the explicit constructor bundle. The immutable execution config no longer carries those runtime ports. |
| `execution.Config.PR` concrete `publication.Config` | `execution/model.PRProviderConfig` | The execution config holds a pure provider value; publication clients convert at the publication implementation boundary. |
| Service lifecycle, control request/receipt files, observation, connection classification, startup record, service locks, platform launch, launcher resources and scheduler loop from `internal/steerline/**` | `internal/app/host/**`; generic loop in `internal/execution/scheduler/**` | `cmd/steerline`, `internal/app`, E2E control consumers and integration fixtures now import the new host owner. The actor and all business decisions remain in `internal/execution`. |
| `contracts.Runtime`, `contracts.Run` and their service/control snapshots | `internal/execution/reconstruction` (`Runtime`, `RunView`) | Built only from the committed checkpoint and the already sampled task/execution records. The former task candidate cache was write-only and is removed; readiness still checks the injected task work port. |
| Conversation events, write/checkpoint values, journal positions, mapper state and activity records from `contracts` | `internal/execution/model` | Codex/Orca adapters, execution, SQLite and product query use the execution-owned values directly. Source/mapper ports and discovery/history request types belong to `internal/execution/conversation`, a focused execution package that adapters can consume without importing execution use cases. |
| Host process identity/probe observation | `execution/backend.ProcessFactsPort`, constructed by `internal/app` through `internal/execution/process` | `internal/app/host` consumes the facts port and does not import the process implementation. |
| Completed control receipt stored in checkpoint reconstruction | `internal/execution/model.ControlReceipt` | `internal/execution/reconstruction` and SQLite use the execution-owned checkpoint value; execution converts it to the separate `product/api.ControlReceipt` at the transport boundary. |
| Task graph export receipt and file publication | `internal/product/api.TaskGraphExportReceipt` and `internal/product/client.WriteTaskGraphExport` | CLI output and atomic file publication no longer live in the host package. |
| Conversation filter and returned conversation/body/raw pages | `internal/product/api` | `internal/product/query` validates stored `execution/model` events and directly returns the public product DTOs; `internal/app` no longer translates through an aggregate contracts package. |
| Conversation event/body/raw storage read records | `internal/storage/ports` | SQLite remains the only SQL implementation; the existing same-snapshot project/runtime read path is retained. |
| `internal/steerlineintegration/**` and `internal/steerline/integration/cli_test.go` | `tests/integration/**` | Real app and host composition assertions stay outside production packages. |
| `internal/steerline/testprotocol/**` | `internal/testkit/protocol/**` | Test fixture consumers update in the same change; production import closures continue to forbid testkit. |
| `internal/steerline/contracts/**` and `internal/steerline/**` facade | deleted | No alias package, forwarding facade or empty future-domain package remains. |

## Host scope

`internal/app/host` owns service process start/stop, exact control transport, immutable request/receipt validation, service identity and startup records, lifecycle locks, heartbeat, console output and platform host behavior. It consumes the narrow `host.Config`, `ApplicationAdmission`, `ProcessFactsPort`, the execution scheduler/host ports and an injected activity formatter. The all-tag source guard traverses the complete production dependency closure, including reconstruction and storage ports; it rejects SQLite, backend adapters, product query/console, process/workspace/provider implementations, task services and app composition. `internal/app` remains the single composition root for config, concrete storage and adapter construction.

The full file and type ownership inventory, moved test parents, and validation runs are in [test-mapping.json](test-mapping.json).
