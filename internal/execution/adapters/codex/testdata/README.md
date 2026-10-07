# Codex conversation fixtures

These three redacted native captures are consumed by `conversation_test.go`.
They were copied byte-for-byte from `docs/validation/` at commit
`172cea6c6530b6693b742324a9c1d4f9aeb17b6d`, after the validation cleanup removed
the files still referenced by the tests:

- `pr-04b-local-codex-native-2026-09-25.jsonl`: two turns, distinct item identities,
  a stable native session, and missing explicit message phase evidence.
- `pr-04b-native-cancelled-2026-09-25.json`: native cancellation evidence.
- `pr-04b-native-failed-2026-09-25.json`: native failure and original error evidence.

The captures describe their original platform versions, not current acceptance
status. Keeping them beside their consumer lets deterministic tests run without
historical validation directories or a live model.
