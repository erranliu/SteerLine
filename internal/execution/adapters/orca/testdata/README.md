# Orca readiness transcript fixtures

These three captured command/tool-result samples are consumed by `readiness_test.go`.
They were moved byte-for-byte from `docs/validation/worker-environment-readiness/`
at commit `172cea6c6530b6693b742324a9c1d4f9aeb17b6d`. They cover direct, saved-wrapper and stringified-wrapper evidence;
the tests also reject fabricated results, incorrect identities and failed tool calls.
The captures describe their original platform versions, not current acceptance status.
