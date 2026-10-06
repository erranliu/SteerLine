# PR225 / Task80 independent final review of cdd47b19

## Identity and disposition

- Reviewer: new independent read-only `/root/review80_final_cdd47b19`.
- Repository/worktree: `C:/Users/lmas1/.codex/worktrees/42c0/SteerLine`; branch `codex/three-line-08-app-host`.
- Exact head: `cdd47b1992cc479c2d3a07c1e76e40d745c69c85`.
- Actual base and GitHub main: `c7eac10aab147fbb9aeaefb9341d5c979d2201c3`.
- Initial and final reads: clean local worktree; GitHub PR225 OPEN/non-draft, correct refs/head/main, MERGEABLE/CLEAN; statusCheckRollup empty. UNKNOWN was not used as final mergeability.
- **Changes required: two confirmed findings.** Config ownership split and prior host/lifetime/admission repairs are present, but nonempty execution requirements now prevent real app claims; the migrated original requirements regression also panics before reaching its final assertions. Do not merge this candidate based on its passing L1.
- No repo/state/commit/push/comment/merge/other-chat writes or fixes. Only this external report and `review80-final-cdd47b19-tests/` were written. All independent processes finished before final handoff.

## Confirmed findings

### F1 — [P1] Preserve evaluated requirements when constructing the fixed claim profile

Locations: `internal/execution/native_claim_command.go:69`, `:121`, `:139`, `:279`; verification rule `internal/execution/execution_requirements.go:87`.

The new app ProfilePort resolves a selected `FixedExecutionProfile` before claim-time requirement evaluation. Its `RequirementsEvaluation` is nil whenever the project has tool, capability or credential requirements. `VerifyFixedExecutionProfile` requires a valid requirement snapshot, and a nil evaluation is valid only for an empty requirement set.

Two real production paths fail:

1. Explicit selected profile (the normal start_task/start_candidate path): line 69 now calls final fixed-profile verification before the evaluation at line 121. Thus a valid selected profile with `execution_requirements.tools=[{"id":"git"}]` is refused as `contract_mismatch`, even when Git is present.
2. Default profile fallback: this bypasses that initial explicit-profile check and evaluates the requirement successfully, storing evidence only on `config.requirementsEvaluation`. Lines 139–143 copy the earlier fixed profile and update only Publication and Digest. They omit the evaluated requirements/evidence/evidence digest. The final verification at line 279 therefore refuses the otherwise valid claim as `stale_profile: claim snapshot changed after evaluation`.

Before this PR, claim evaluated requirements first, then `config.bindProjectPolicy(*options.ExecutionProfile)` rebuilt the profile from the evaluated config, and Freeze included its evaluation and evidence digest. Current code does not preserve that behavior. This is a newly introduced supported configuration/claim regression, not merely a type-owner objection. Config loads/admission succeed, the required tool is independently observed present, and a task is formally created, but cannot be claimed. The same omission affects nonempty capabilities/credentials, and prevents creation of their valid frozen workflow snapshots.

Independent reproduction: external overlay adds only `tests/integration/review80_external_requirements_test.go`; no production replacement. It uses `newLifecycleProject`, edits the isolated real project JSON, calls actual `app.LoadConfig`, ResolveIdentity and EvaluateRequirements (requires Git evidence status `present`), formally creates task 901, opens the actual app command store, constructs actual `app.NewCommandService`, then calls ClaimNativeTask with current graph/task/authority versions. Explicit and default profile subtests both fail with the two errors above. Command:

`go run ./cmd/test-feedback -out <raw>/actual-app-requirements -- ./tests/integration -overlay=<raw>/requirements-overlay.json -run '^TestReview80ActualAppRequirementsClaim$' -count=1 -timeout=3m`

Observed shell exit **1**; 3 run/3 fail (parent + both children), 1 failed package, zero skips, paired run/terminal events. Sources, exact command, raw JSON and stderr are retained under `actual-app-requirements/` and `requirements_probe_test.go`.

Required direction: distinguish the selected pre-evaluation identity/digest from the final evaluated frozen claim snapshot; after the mandatory evaluation bind its cloned evaluation and evidence digest, along with the same publication facts, before final verification/commit. Preserve exact selection, stale-config checks and original replay semantics; do not relax final frozen-profile verification or the requirement gate. This belongs to the original executor's F1 config/freeze repair.

### F2 — [P2] Restore the migrated requirements claim/restart regression's executable fixture

Location: `internal/execution/execution_requirements_test.go:173–175`.

The migration now calls `c.BindAcceptedConfigVersion(digest)` before `neutralProfileSettings(c)`. DecodeImplementationConfig validates the existing envelope against the new accepted version, returns an error, and neutralProfileSettings returns a zero settings value. setNeutralProfile then panics because the fixture executable is empty. In the base, settings were read before changing configDigest, so the old envelope could be re-bound to the new identity.

Independent direct command, without an overlay:

`go run ./cmd/test-feedback -out <raw>/requirements-claim -- ./internal/execution -run '^TestExecutionRequirementsClaimAndRestartBlockWithoutChangingSnapshot$' -count=1 -timeout=3m`

Observed shell exit **1**, 1 run/1 fail, zero skips. Raw panic: `implementation codec fixture.config rejected profile: fixture executable is required`, stack at test line 175 and business_fixture_test.go:188. The test never reaches its accepted-file drift, successful requirements claim, restart-current-blocker and historical snapshot preservation assertions. This is a separate original assertion-migration failure, not evidence of the production error in F1; the actual app overlay above independently proves F1. Current L1 does not select this parent, explaining why 451 passing tests did not catch either issue.

Restore correct fixture rebinding and run this original parent/its existing assertions after F1 is repaired. Do not remove it, turn its expected success into a refusal or classify the panic as an allowed skip.

## Current ownership and real call paths

Read AGENTS, roadmap, architecture/common constraints, fixed TM08 Spec/latest Plan, E2E README, validation README/ownership/test-mapping/recovery comparison. Inspected actual-base...head production/type/consumer/test migration and deletions, default app/cmd/host/installation/platform paths and changes from the previous independently reviewed source. Complete actual diff/name-status and reading-aid inventories are external. The PR has 731 changed paths including archived validation. Current normalized Go inventories have 203 changed non-test paths and 170 test paths; normalization is a reading aid, not a behavioral certificate.

The remaining broad-Config ownership omission from the previous reviews is structurally repaired:

- LoadConfig, LoadQueryConfig and LoadConfigWithDirectory return real `app.Config`; ProjectConfigInput is the strict app-owned disk value. App alone retains project/global paths/digest, catalog/default and concrete construction ports.
- Task Service receives task.Input (native authority/project/routing/executable types/requester) and consumed task document capability. Claim and scheduler call taskWork.CanExecuteType/DefaultRequestedBy; task inputs do not propagate in execution.Config.
- hostRuntimeConfig projects a real host.Config with service identity/timers and consumed runtime/read-only-admission/control-validation/process-facts ports. Host lifetime is Run/Close. ApplicationAdmission closures remain app-owned and implement the actual narrow callbacks; their captures are not frozen execution inputs or host domain-writer fields.
- execution.Config contains identity/repository/workspace/Git/policy/workflow/provider and one selected implementation envelope/digest. No whole app, catalog/default, project/global files, task authority/type/requester or host timers/directory/process ports occur in Service.Config/baseConfig, ApplyFixedExecutionProfile or copyWorkerFlowConfig.
- execution.RuntimeDependencies separately holds Directory, ProcessHost, consumed ProfilePort and ConfigVerifier. ServiceStorage alone transfers state/commands/worker/conversations/records/resources/store-close capabilities; actual app factories immediately inject the same SQLite connection and task ports into the real actor.
- appProfilePort holds a ProfileCatalog resolver snapshot rather than app.Config. Its execution base values, raw profile inputs/default/global metadata, directory and PolicySource have real profile/default/digest consumers. PolicySource is needed in the fixed profile/source identity; the separate appConfigVerifier alone reads the accepted project/global files. The runtime resolver's catalog is narrow DI state, not an execution.Config/per-workflow catalog copy. Directory remains immutable and implementation factories now receive the same explicit runtime process/directory dependencies.

Independent snapshot probe mutates app Git/PR argument slices, requirements, raw implementation payload, profile/default IDs, task/claimant/heartbeat fields after runtime capture; exact returned fixed JSON, execution input JSON and accepted digest/default remain unchanged. It then mutates returned envelope, requirements, publication Git/provider args/auth refs and execution input; a new resolution still matches the original bytes. Real frozen/worker copies also pass their two existing execution context tests. Implementation payload/envelope, requirements/evaluation and publication clones are present on the consumed paths. No architecture conclusion rests solely on reflection or serialized field names.

The exact app profile projection independently passes default and second-profile resolution, selected route/implementation, accepted/source digest, stale profile digest and fixed wire roundtrip. The real file verifier independently rejects changed project bytes while the retained accepted snapshot/digest stays unchanged. **These checks do not cover the missing claim-time evaluated evidence in F1.**

## Prior repairs and preserved production responsibilities

- Host ServiceHost imports injected pure ports and process facts; actor and task/candidate qualification/dispatch/delivery/recovery remain execution/task. The all-tag source guard and Windows/Linux production dependency graph inspect the complete closure. Its pure task/model allowance does not stop traversal; the synthetic host→execution→workspace negative passes by detecting the forbidden route. Independent full coreboundary/testboundary runs pass 52 tests, zero skips. No wider whitelist was introduced.
- Actual app serviceAdmission runs the read-only unsupported/Beads preflight before createStartupAttempt/log evidence. Independent real app Beads and unknown-state tests preserve the absent run directory/sole marker bytes. Native authority and file/schema refusal remain strict; no migration/adoption/legacy fallback was added.
- Real lock-owning app factory transfers the one SQLite writer, NewService closes on constructor error, host owns Run then Close and joins errors. Current independent actual-store probe selects the transferred `*sqlite.SQLiteStore`, successfully reads before return and gets **`sql: database is closed`** afterwards. It has no test-defer Close, phase-refusal inference, second database or directory rename. Error-exit test confirms exactly one Close and both errors. Center stop remains distinct from worker termination.
- CAS 1→2 followed by stale expected-version 1 is retained at actual app/native project owner; independent writable-store probe requires stale_version and reads back an identical authority, without a partial write.
- App strict JSON wrapper keeps BOM acceptance, unknown/single/trailing-value assertions and real project/global/answer consumers. Backend strict decoder retains BOM rejection and typed errors. Owner-local wrapper behavior is preserved; no backend-wide loosening.
- Old steerline/steerlineapp/contracts/steerlineintegration paths are gone; their real consumers are switched. Focused conversation source/mapper/history contracts have real adapter/execution consumers, durable records and activity live in execution/model, read records in storage/ports, public DTOs in product/api. Stored-event assertions displaced by public DTO projection remain at raw storage owners. Original causes and unique activityrender formatting remain.
- TaskGraphSnapshot is solely storage/ports; SQL/schema is solely storage/sqlite. ReadExecutionStateSnapshot samples checkpoint, graph authority and execution records through one transaction. ReadApplicationProjectSnapshot combines real product view and execution facts in that same transaction; actual app.clientReader and E2E SnapshotContext use those facts. Independent differing-database-revision rejection passes. Original full DAG/task history/scoped workflow/activity SQL paths are unchanged, not replaced by whole-domain reads.
- Checkpoint/control receipt fields retain request/digest/instance/action/times/run/attempt/decision/revisions and original replay/intent/receipt/ack/CAS/unique-delivery gates. Existing TM04 rollback/original receipt, two turns and manual recovery pass independently on this head, as does actual app CandidateConsoleWiring default/selected candidate/worker/control/history flow.
- cmd run/check/control/task export now use app/host or product owners. Install/launcher/embed and Windows/Linux process identities use the actual relocated resource/implementation paths; three launcher blobs are byte-preserving moves. Current install copy/overwrite/CRLF tests pass. OS dependency checks do not claim Linux runtime or native backend certification.
- Full runner business owner remains execution; L1 retains TM04 named children, TM05/06/07/TM08, resource/Git/provider/heartbeat guards, managed Git and short isolated home, native opt-in refusal, deadline/cleanup and original candidate samples. No second SQL/backend, new remote admission, planning/HTTP/SSE or empty future-domain implementation.

## Raw evidence, source binding and independent commands

`source-git-objects.json` independently enumerates actual Git blobs for base/original source/current source/head. All **1,433 non-validation objects** at `613e56a8bef9288b395d1888dea802435ebf435f` equal current head. `git diff --quiet 613e56a8 HEAD -- ':!docs/validation/**'` exits 0; the only later changes are validation docs/logs. Source/tests/Plan/runner equality is based on objects, not a worker assertion.

`audit.py` independently decodes all archived Task80 summaries/raw events and this review's completed runs, comparing run/terminal sets, package terminals, assigned parent/subtest hits and recorded process exits. Final L1 review-fixes-05 is clean-start bound to 613e56a8: 12 groups, 27 packages, **451 run /451 pass /0 fail /0 skip**, all exits 0, no missing assignments/unpaired events. Group counts: 4,256,16,45,15,14,17,4,2,9,13,56. It includes all original TM04 children and TM08 host/app/boundary, CAS/decoder/lifetime/projection/scheduler assertions. This is audited archival evidence; independent shell exit values below are separately observed.

Failure history remains in raw-evidence-audit.json: commit-bound-01 has 356 run/340 pass/16 fail plus compile/package failures; review-fixes-02 has 451 run/444 pass/7 fail on a dirty configuration-split tree; review-fixes-03 has 448 run/446 pass and 2 unterminated runs at the integration package's 10m timeout, with later TM06/TM08 selections absent. The passing review-fixes-04/05 keep the same assigned tests and assertions, changing only that package timeout to 15m. No skip/selection/assertion weakening explains the timeout recovery. Dirty-tree target reports are not an exact commit certificate; current clean-source L1 plus current independent runs supply the correspondence.

All own commands use `go run ./cmd/test-feedback -out <raw>/<group> -- <packages> <selection> -count=1` on exact cdd47b19, Windows/amd64 Go1.26.4, isolated temporary Git/SQLite/config/homes and controlled protocol fixtures, no native opt-in:

| External group | Actual selection | Observed shell exit / events |
|---|---|---|
| app-host-config | app projection, CAS, strict decoder, real Beads/Close + host TM08 ControlIdentity/Heartbeat/Shutdown/LifetimeCloseOnRunError | 0; 9 run/pass, 2 packages |
| boundary | host closure negative + two scheduler Run tests | 0; 4 run/pass; testboundary had no matching test in this first selection, not treated as coverage |
| pure-boundaries | full internal/coreboundary + internal/testboundary | 0; 52 run/pass, 2 packages |
| candidate-wiring | TestCandidateConsoleWiring + TM08AppHostWiring/default_app_host_composition | 0; 4 run/pass, 288.565s wall, timeout5m |
| config-consumers | two fixed worker-context tests, two install/launcher tests, E2E differing-revision snapshot | 0; 5 run/pass, 3 packages |
| independent-probes | exact SQLite close, unknown-state refusal, stale CAS no partial write, captured resolver clone isolation; test-file-only app overlay | 0; 4 run/pass |
| verifier | original accepted file bytes refused after mutation; test-file-only app overlay | 0; 1 run/pass |
| tm04 | parent + claim_transaction_failure_and_original_receipt / intent_binding_and_two_original_turns / original_request_ack_and_manual_recovery | 0; 4 run/pass, 293.258s wall, timeout8m |
| requirements-claim | original TestExecutionRequirementsClaimAndRestartBlockWithoutChangingSnapshot, no overlay | **1; 1 run/fail; fixture panic F2** |
| actual-app-requirements | real app loaded requirement claim, explicit_selected_profile/default_profile; test-file-only integration overlay | **1; 3 run/fail; production F1** |

Every completed independent test run has paired nonzero test events and zero test skips. All failing logs/probe sources remain. Each summary records the exact command, current source SHA, clean worktree and no native switches. No production file is replaced by an overlay. No Full/L2/L3/live/models, arbitrary full failure matrix or native capability certification was run; minimal additional requirements selection was justified by the changed freezing path.

## Recovery comparison and handoff

For empty requirements, fixed version6/field JSON, schema18, accepted config digest, implementation envelopes and workspace/resource/original-session authorization remain field-equivalent; current independent TM04 and candidate flow substantiate replay/two-turn/manual recovery. Directory/process/resources merely arrive through actual runtime/storage DI and keep original binding checks. However the blanket recovery comparison claim that no frozen requirement/evidence behavior changed is contradicted by F1: current required-evidence freeze no longer yields an accepted workflow. This is an implementation blocker, not a waived L3 or manual-pending item. Repair F1/F2, update the comparison using real evaluated frozen bytes, then reassess the Plan's conditional controlled waiting-merge-restart scenario; behavior changes must receive its registered controlled evidence. I did not run a broad recovery suite against this known-broken requirement path.

Final branch/head/worktree and platform main/head again match the identity above; PR remains MERGEABLE/CLEAN without returned checks. Config-owner structural repair and prior F2–F4 fixes can be retained, but current TM08 preservation is incomplete due to the two confirmed regressions. Coordinator owns routing, PR discussion and expected-head merge protections. No final merge certificate is issued. Stop here.
