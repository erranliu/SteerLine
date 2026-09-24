# TASK-CORE interface inventory

## Existing center authority reused

| Concern | Existing owner | TASK-CORE extension |
| --- | --- | --- |
| Repository identity and shared state location | `Config.ResolveIdentity`, `repoStateKey`, `steerline.db` | Native authority binds the Git common-dir identity, exact remote URL, remote name and branch; native state uses the existing repository-derived run directory. |
| Task / WorkflowRun identity | `TaskRecord`, `WorkflowRunRecord`, SQLite `tasks` and `workflow_runs` | Numeric native task IDs remain `TaskRecord.TaskID`; no second task store or execution identity is added. |
| Transactions and revision | `DomainStore.Transact`, `SQLiteStore` | Schema v2 adds native authority, contract, batch, dependency, delivery and database-revision records in the same database. |
| Read queries | `QueryService`, read-only `OpenSQLiteReader` | Native task pages/details use one SQLite read transaction and `native-tasks/v1`. |
| Commands / outbox | `CommandService`, `commands`, `operations` | No native task business write command is exposed in this task. The repository transaction types are available for later DAG implementation and test fixtures. |
| Beads provider | `Beads` in `beads.go`, current service dispatch path | Existing Beads behavior remains its own configured mode. Native mode blocks the legacy service and does not call `bd show`. |
| CLI | `cmd/steerline/main.go` | Adds `task list` and `task show`; existing `tasks` candidate snapshot command retains its prior meaning. |

## TASK-CORE ownership boundary

This implementation owns native Task and TaskContract data shapes, native project authority binding, additive SQLite schema v2, contract canonicalization/Git-blob verification, CAS task-state repository primitives, a database revision, and basic task queries. It does not create DAG tasks, add dependencies through a business command, compute `ready`, discard from CLI, claim, run workers, verify/merge PRs, or render a relationship graph.

The prior center schema v1 is extended additively in one SQLite migration transaction. Existing v1 task and run payloads stay intact. Native authority cannot adopt a Beads state directory, a pre-existing task database without matching native authority, an unknown schema, or a different repository/remote/branch configuration.

## Query sample

The protocol header and explicitly unavailable dependency capabilities are shown in [native-tasks-v1.sample.json](native-tasks-v1.sample.json). The sample is a fixture shape, not a live project snapshot.
