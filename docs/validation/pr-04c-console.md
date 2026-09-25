# PR 4C Console Validation

Date: 2026-09-25
Base: `46fae8f`
Environment: Windows amd64, Go 1.26.4

Status: U-A1–U-A15 accepted for the current Windows amd64 console and the measured local Codex/Orca public-history targets documented by PR 4B. The current console command, multi-project query/control model, request-ID persistence, read-only decisions, attempt/activity/session views, turn folding, body paging and UI-owned preferences are implemented. Remote Orca and other agents remain outside the measured capability scope rather than being treated as exact-turn capable.

## Automated checks

- `go test ./internal/steerline/console ./cmd/steerline -count=1 -timeout 2m` — passed.
- `go test ./internal/steerline -run '^(TestControlReviewVerifiedReceiptRejectsWrongAction|TestControlReviewClientBindsDisplayedInstanceAndRevision|TestControlReviewReceiptAppearsAfterApplying|TestConversation.*|TestInstallLaunchersWritesCRLF|TestListAndResolveProjects)$' -count=1 -timeout 2m` — passed.
- `go test ./internal/steerline -run '^TestPRFailuresNeverClose$' -count=1 -timeout 90s` — passed in 25.2s.
- Final post-audit `go test ./... -count=1 -timeout 20m` — passed: command 3.624s, core 680.142s, console 2.075s, remoteprotocol 0.587s and viewer 2.161s. Earlier complete runs also passed.
- `go vet ./...` — passed.
- `go build -o <temporary path> ./cmd/steerline` — passed on Windows amd64.
- Windows and Linux `go build ./...` — passed.
- `git diff --check` — passed; Git printed only its configured LF-to-CRLF working-copy notices.
- Interactive PTY smoke: launched `steerline console` with an empty temporary `STEERLINE_HOME`, confirmed the config-directory/setup instructions, and exited with `q`. No service was started.
- Interactive PTY smoke: opened a valid config outside `etc` with no running service; the console showed the offline state, config path and unavailable fixed service-window installation instructions, then exited with `q`. No service was started.
- Gated `TestWindowsVisibleServiceWindowAcceptance` — passed in 18.75s: visible independent window, manual/ready startup, no pre-claim, explicit selected start, pause/stop/exit semantics and instance-bound receipts.
- Gated `TestWindowsVisibleServiceWindowCloseAndRestart` — passed in 25.14s: closing the service window stopped the original process/worker, the client classified it offline, and reopening produced a new manual instance.
- Gated `TestWindowsVisibleConversationCollectionAcrossViewerRestart` — passed in 44.27s: collection continued while the viewer was closed, reconnect retained the execution, offline full-body reads worked, restart did not duplicate events, and source replacement remained visible.
- Gated `TestWindowsConcurrentVisibleServiceWindows` — passed in 18.64s for the same config and for the same repository with a different run directory; repository ownership prevented a second live writer.
- `TestConsoleNativeLocalAndOrcaFixtureUseSameFoldScript` replayed the real PR 4B public-history evidence through the 4C reducer and 80/170-column renderer. Both targets retained two completed turns, two final answers and commentary; Orca retained its two tool records.
- Post-run process inspection found only Go-test temporary executables under the OS temp directory. The two remaining exact test processes were stopped; no SteerLine or Orca probe process remained.

The first combined core run exceeded the Plan's 10-minute package timeout while the existing PR-failure regression was active. That test passed in isolation, and the required full suite passed with the repository's established 20-minute cap.

## Post-implementation quality audit

The final audit found and corrected issues before the post-audit full-suite run:

- Receipt reconciliation now hashes the immutable request's original bytes and binds request ID, action and instance in one verified read path. Deterministic pre-publication stale instance/revision failures become terminal rejected results; terminal results do not resume polling after UI restart, and a second outstanding stop request is blocked.
- Candidate controls use the currently connected service instance rather than a potentially newer read-only history projection. PR query failures count as attention instead of appearing complete.
- Conversation item revisions are monotonic across process/final reclassification. An older late final cannot replace a newer process revision, and only message events with an explicit `final_answer` phase become final replies.
- Body responses are bound to project, generation, item, revision, offset, digest, byte count and completeness. Exact UTF-8-safe page starts persist for backward paging; a response arriving while another project is selected updates only its originating project.
- View preferences use bounded reads, reject symlink files and inconsistent action/body state, and retain terminal versus outstanding request state across restart.
- ANSI-aware truncation prevents styled rows from being cut inside escape sequences. List windows and scroll bounds now use the actual terminal height after the wrapped footer, keeping the selected row reachable in long and narrow views.

## Acceptance evidence

| ID | Result | Evidence / remaining gate |
| --- | --- | --- |
| U-A1 | Accepted | Zero configs, multiple configs, one damaged config and an explicit config outside `etc` are covered by loader tests and PTY smokes. |
| U-A2 | Accepted | The gated visible-window test proved manual/ready startup, no pre-claim and an explicit selected start; offline UI shows the fixed launcher/config or the precise installation failure. |
| U-A3 | Accepted | Enter submits nothing; repeated `s` retains one durable request ID; submission binds the displayed instance/revision; uncertain results query the same ID; an unpersistable ID blocks publication. The live service test proved one selected start. |
| U-A4 | Accepted | Project/generation checks reject late responses; per-project selection, cursors, actions and view state are isolated. The gated concurrent-window test proved repository lock behavior for same-config and same-repository/different-run-dir targets. |
| U-A5 | Accepted | Waiting decision, waiting PR merge, unknown, failed/blocked and completed use separate axes; unknown/failed/error cannot render as completed. |
| U-A6 | Accepted | Pause and stop use distinct commands; stop requires confirmation and states the shutdown consequence; visible-window acceptance proved worker continuity, explicit stop, UI/window separation, offline classification and restart. |
| U-A7 | Accepted | 80×24 and 170-column render tests cover panel reachability, Unicode titles, long errors and width bounds; PTY smokes covered the actual terminal path. |
| U-A8 | Accepted | `console`, `projects`, `console.bat`, and the `viewer.bat` compatibility shortcut route to the current console; launcher install verifies CRLF and entry files. |
| U-A9 | Accepted | Candidate freshness/error/instance checks disable start while retaining cached labels. The visible conversation lifecycle proved logs/full bodies remain readable offline and reconnect does not resubmit actions. Decisions are read-only. |
| U-A10 | Accepted | Explicit turn end folds once; message completion does not end a turn; late final stays in its turn; manual expansion persists. The real local and Orca public-history evidence passed the same reducer/render script. |
| U-A11 | Accepted | Same item IDs in separate turns stay separate; late final, unknown identity, failure/cancellation, source gaps and source changes remain scoped and visible. PR 4B real/injected evidence is consumed through the current query model. |
| U-A12 | Accepted | Turn events only affect presentation; host waits/gaps and pending business decisions remain outside folds; business status stays in the runtime authority. Real PR 4B host-wait and failure evidence remains the capability source. |
| U-A13 | Accepted | Separate live view profiles, inactive-profile reuse, manual fold intent, keep-open intent, request identities and read anchors persist; corrupt preferences reset locally without touching business state. |
| U-A14 | Accepted | Body reads use exact revision-bound, UTF-8-safe pages; long Unicode body integrity tests pass. Follow state and stable group/item/revision/line anchors persist; folding an anchored item relocates to the group heading. |
| U-A15 | Accepted | Source/capability gaps are shown without phase guessing; terminal controls in text and activity metadata are escaped. Real collector/capability evidence from PR 4B is matched by backend, protocol, target, execution, attempt, source and source identity. |

The console’s automated test file covers reducer intent, request identity, candidate cache blocking, profile isolation, damaged preferences, project-response isolation, cache preservation on refresh errors, a folded-session scroll-anchor relocation, narrow layout entry points and terminal-text/metadata escaping. Request messages are absent from the saved view file; request identities and statuses remain queryable.
