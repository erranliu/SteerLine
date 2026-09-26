# Beads path removal validation

This report records the implementation and regression evidence for [Spec 12](../../spec/beads-path-removal.spec.md). The accepted #11 baseline is in [test-feedback results](../test-feedback/results.md); this report compares its latest complete run with a fresh complete run after the removal.

The full run uses the repository runner:

```powershell
go run ./cmd/test-feedback -phase beads-path-removal-audit -out docs/validation/beads-path-removal/full-final -native-acceptance 'Live Codex, Orca, PR-provider, and visible-window acceptance switches were not enabled.'
```

The runner stores the unmodified `go test -json` stream, stderr, environment, source revision and per-test timings under `full-final/`. [Coverage map](coverage-map.md) classifies removed old-path tests and maps current invariants to native coverage. [Results](results.md) records build/static checks, test timing and live acceptance limits.

Live Codex, Orca, PR-hosting and visible-window switches are separate acceptance gates. A skipped or unrun gate is reported as unverified; this automated suite does not promote test doubles to live platform evidence.
