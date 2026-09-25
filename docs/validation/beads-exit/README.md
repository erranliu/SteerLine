# BEADS-EXIT validation

Status: implementation and acceptance complete; the implementation PR is awaiting review. See [the live acceptance record](live-acceptance-2026-09-25.md) for real platform identities and limits.

## Evidence

| Gate | Result | Evidence |
| --- | --- | --- |
| NT12 real A → B | Pass | Isolated private GitHub repository, no `.beads`, unavailable `bd`: local Codex tasks 901 → 902 produced two human-merged PRs and center TaskDelivery. Because automatic dispatch claimed 902 immediately, the persisted-ready-after-restart gate is proven separately by Orca terminal-v2 tasks 903 → 904: after PR #3 was human-merged and TaskDelivery recorded, 904 remained open and ready with no unmet predecessors across service restart. |
| NT13 worker contract | Pass | Preflight rejects legacy `bd ready`, start/finish and completion dependencies. Tests assert Beads variables are absent from worker environment. |
| NT14 clean initialization | Pass | New authority initializes manual and paused. Explicit center command enables dispatch; binding/config authority drift blocks side effects. |
| NT15 old/unknown state | Pass | Beads data, unsupported schema/files, custom state forks and ambiguous external execution are rejected without import, conversion, adoption or cleanup. |
| NT16 recovery and idempotency | Pass | Tests cover transaction and projection failure, retry with retained failed attempt and replayed receipt, lost Orca create response without duplicate terminal, exact exit/result recovery after restart, and failed cleanup-receipt persistence followed by restart recovery. Retry clears the settled worker identity from its runtime projection before the next attempt. Native dispatch mode and pause are stored only in center SQLite, so there is no config-file mirror write to inject failure into. |
| NT17 local / Orca | Pass | Real local Codex and Orca terminal-v2 starts, output, workspace inspection and stop were certified. The live Orca native workflow delivered a task through human PR merge. Supervised remote remains disabled and untested. |
| NT21-EXIT Beads-free path | Pass | Native create/claim/steps/delivery use center SQLite and fixed `native-task/v1`; normal native path does not invoke Beads or mirror a second task authority. |
| NT22-EXIT discard cleanup | Pass | Local tests verify exact worker stop and worktree removal, separately settled PR-close/workspace receipts, injected receipt failure retaining pending identities, and restart recovery. PR query uncertainty does not prevent independent workspace cleanup; the center rejects malformed or premature workspace receipts. Exact GitHub/Gitea adapter identity handling is covered by HTTP/provider fixtures; live merge acceptance is recorded, but live PR-close cleanup is not claimed. Unsupported remote stop remains manual. |

## Final checks

| Command | Result |
| --- | --- |
| `go test ./... -count=1 -timeout 20m` | Pass, exit code 0 after the quality audit fixes; `internal/steerline` 1051.945s. Full package output is in [the test log](full-go-test-final-2026-09-25.txt). |
| `go vet ./...` | Pass, exit code 0. |
| `git diff --check` | Pass, exit code 0. |
