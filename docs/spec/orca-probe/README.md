# Orca protocol probe

This folder contains the repeatable, read-only evidence collector for Spec 01, a local CLI/runtime snapshot, a [real-remote capability report](remote-capability-report-2026-09-23.json), a [field map](field-mapping.md), sanitized remote CLI samples, and an explicitly illustrative SteerLine envelope. The remote report separates verified behavior from missing fault injections.

## Capture a fresh snapshot

From the repository root on a machine with Orca CLI installed:

```powershell
go run ./cmd/orca-probe snapshot -out .\docs\specs\orca-probe\capability-report.json
```

The command reads only `orca --version`, `orca agent-context --json`, `orca status --json`, and `orca environment list --json`. It records the center runtime version/capabilities and the number of saved remote environments; it does not save runtime IDs or environment names. This local snapshot deliberately leaves remote capabilities as `not_tested`. The separately reviewed remote report holds the real-machine verdicts.

## Capture scoped evidence

`capture` runs one allowlisted read-only Orca command and writes a sanitized JSON record. Worker, task, Run, request and worktree reads use exact IDs. Inbox reads require both the Run ID and the coordinator's own terminal handle; `check-peek` and `check-all` do not consume a Delivery.

```powershell
go run ./cmd/orca-probe capture -operation remote-status `
  -environment <saved-environment-id> `
  -out .\remote-status.sample.json

go run ./cmd/orca-probe capture -operation repo-show `
  -environment <saved-environment-id> -repo id:<test-repo-id> `
  -out .\test-repo.sample.json

go run ./cmd/orca-probe capture -operation worker-show `
  -dispatch <test-dispatch-id> `
  -out .\worker-show.sample.json

go run ./cmd/orca-probe capture -operation worker-read `
  -dispatch <test-dispatch-id> -source auto -limit 50 `
  -out .\worker-read.sample.json

go run ./cmd/orca-probe capture -operation check-all `
  -run <test-run-id> -terminal <coordinator-terminal-handle> `
  -out .\inbox-history.sample.json

go run ./cmd/orca-probe capture -operation request-show `
  -request <platform-request-id> `
  -out .\request-receipt.sample.json
```

Raw command output is kept only in memory. JSON responses are sanitized before writing: IDs/cursors/paths/commits are replaced with stable placeholders within each sample, URLs and secret-shaped fields are removed, and prompt/message text is redacted. A JSON `payload` string is parsed, sanitized field by field and serialized back to the same string-shaped field so its protocol structure remains inspectable. Alias numbers are local to each capture file. Non-JSON output is omitted. Review every sample before sharing it.

The collector intentionally has no mutation operations. Run the mutation scenarios in Spec 01 only after selecting an independent test Orca environment, exact test repo ID, throwaway branch/base, and test agent. Use the CLI syntax captured in the report and save each receipt through `orca-probe sanitize -in <receipt.json> -out <sample.json>`. Never use a production repository or task for these experiments.

## Evidence labels

| Label | Meaning |
| --- | --- |
| `cli_help` / `captured` | The local Orca CLI schema advertised a command or flag. |
| `captured_cli_response` | A read-only command was run and its JSON was sanitized. |
| `synthetic_protocol_draft` | An example of the proposed SteerLine logical envelope; not an Orca payload. |
| `real_remote` | A reviewed capability report based on this paired remote run. |
| `captured_cli_response_curated_sanitized` | A concise sample transcribed from actual CLI receipts with identities removed. |
| `not_tested` | No qualifying remote observation was made. |

The local snapshot never upgrades a capability based on help text or the logical fixture. The remote report records the observed evidence. [FIFO and 8 KiB payload replay](samples/fifo-size-2026-09-23.json) passed; the actual Orca size limit remains open. In the direct-terminal path, `terminal wait` returned the exit code but `terminal read` lost stdout after exit; see the [baseline sample](samples/direct-script-2026-09-23.json). A later [file-backed receipt probe](samples/p11-file-receipt-2026-09-24.json) recovered structured output after a home Orca restart through a second agentless terminal.

A later [paired-client disconnect/reconnect probe](samples/disconnect-reconnect-2026-09-23.json) preserved the original Delivery but left its in-flight Dispatch unverifiable after the UI showed Connected again. The installed Orca version treats an explicit UI disconnect as retirement of the old runtime binding. This was a manual-disconnect control; the worker was not answered, retried, stopped or released through an unverified binding.

A [separate fresh-start attempt](samples/manual-disconnect-start-blocked-2026-09-23.json) was refused as manually disconnected in the current home runtime despite connected remote status; that Run recorded no workers.

An authorized [home runtime restart](samples/home-restart-remote-unavailable-2026-09-23.json) preserved the old Run record while the remote machine was powered off. After power-on, the [original Dispatch recovered and settled](samples/restart-recovery-settled-2026-09-24.json). This covers one P07 timing; full P07 remains open.

A separate [30-second transient transport fault](samples/p06-transient-2026-09-24.json) passed P06: the original Dispatch became `unverifiable/host_unavailable`, the same question Delivery replayed, and the exact worker recovered without a replacement. It then settled and released; the test worktree remained at the same HEAD.

The [background coordinator observation](samples/p01-background-2026-09-24.json) passed P01: the home Run launched a named remote worker with actual execution identities, and no SteerLine-named process was resident on the remote host at the process-inventory checkpoint.

The [output cursor probe](samples/p03-output-2026-09-24.json) passed P03: retained transcript messages paged without duplicate IDs, omitted history was flagged, and a live terminal cursor returned `source_changed` after release moved reads to the archived transcript.

The [center restart probe](samples/p07-center-restarts-2026-09-24.json) passed P07 by recovering the original remote Dispatch after home Orca exits, including an exit during worker-start. The [response-loss probe](samples/p08-response-loss-2026-09-24.json) passed P08 using saved request IDs or read-only Run/Task reconciliation when the entire CLI response was discarded. A mid-start request can remain `pending` after its worker settles; the original Dispatch must be inspected rather than blindly retried.

The [current-version P10 probe](samples/p10-current-version-payload-2026-09-24.json) preserved unknown fields and blobs through 30 KiB from home to remote. The user explicitly deferred older-version compatibility; the report makes no mixed-version claim. The actual Orca size limit and host permission prompts remain untested.

The [P13 local guard](samples/p13-verify-gate-unit-2026-09-24.json) checks verify-step identity, required receipts, exit codes and candidate commits without trusting a succeeded summary. Spec 03 now calls it on accepted supervised results: the [positive receipt](samples/spec03-supervised-staged-implement-verify-2026-09-25.json) advanced only a verified commit, while the [negative receipt](samples/spec03-supervised-negative-verify-retry-2026-09-25.json) recorded exit 7 and planned exactly one repair without creating a PR. Both used the 1.4.209 center and 1.4.210 home pair.

The [committed worktree probe](samples/p12-committed-step-reuse-2026-09-24.json) showed both an off-target agent commit that must be rejected and a successful pair of distinct Dispatches that preserved a real commit in one assigned worktree. The [remote-runtime restart probe](samples/remote-runtime-restart-2026-09-24.json) recovered an exact waiting worker. The [disconnected stop probe](samples/disconnected-stop-2026-09-24.json) found a platform cleanup gap: a live worker stayed `stop_unknown` and exact stop/follow-up mutations were refused. After explicit user authorization, that exact test terminal was manually closed and the Dispatch abandoned; automated cancellation remains unsupported.

The agreed production scope compares Orca by `major.minor`: the 1.4.209 center and 1.4.210 home pair has supervised probe evidence. Remote cancellation remains unavailable. The version-family match does not certify untested capabilities or other version families, and Spec 03 remains fail-closed before production claim.

The [remaining gates](remaining-gates.md) list the exact fault scenarios and evidence needed before Spec 01 can be accepted for Spec 03.
