# PR 4B quality audit follow-up

Scope: conversation collection, source identity, body reads and final-answer reconciliation on the PR branch against `origin/main` at `696406f`.

## Findings repaired

1. **Empty native history hid existing progress.** The collector selected `codex-app-server` before it had any turn. Default queries then hid raw JSONL observations. Source selection now waits for the first native turn; an explicit raw-source query remains available after selection. `TestCodexHistoryEmptyPageDoesNotHideRawProgress` covers both sides.
2. **Live raw bytes were not checked against the file range.** A same-length rewrite between worker observation and commit could persist different raw bytes under a checkpoint for the changed file. The collector now compares the exact source range before writing a session binding or advancing the checkpoint. Mismatch produces a visible `source_unavailable` gap. `TestConversationChangedLiveBytesCannotBindOrCheckpoint` covers the rollback and unbound session.
3. **Missing text could be reported as complete final content.** Raw JSONL metadata-only updates could clear a prior snapshot, while a final item with no text could claim `available`. The app-server mapper and capability report had the same missing-text ambiguity and could drop the pending late-final cursor. Both adapters now distinguish a missing or `null` text field from an explicit empty string. `TestConversationMissingTextPreservesSnapshotAndEmptyFinal`, `TestCodexHistoryMissingTextDoesNotClaimFinalBody` and `TestCodexHistoryReconcilesFinalTextArrivingAfterTerminalStatus` cover storage, status and later reconciliation.
4. **Explicit empty final bodies failed SQLite validation.** An empty slice was inserted as SQL NULL into the non-null body column. The store now writes an empty BLOB and retains a complete zero-byte body reference. The same tests cover the read path.
5. **Corrupt oversized rows were loaded before validation.** Body and raw queries now request at most the 16 MiB store limit plus one byte, then reject oversized/corrupt rows. `TestConversationBodyAndRawCorruptionCannotReadAsComplete` injects both cases.

The existing real local and Orca public-history bindings passed again after these repairs. The changes do not alter the database schema or external conversation DTO.

## Verification

- Focused `TestCodexHistory|TestConversation|TestOrca` run passed in 65.869s. The gated real local/Orca history, source-replacement and failed-turn acceptance checks passed on their exact isolated bindings.
- `go test ./... -count=1 -timeout 20m` passed: core package 765.442s, remote protocol 0.402s, viewer 2.136s.
- `go vet ./...`, Windows/Linux `go build ./...`, and `git diff --check` passed.
