# Archived review80 baseline diagnostics

This directory preserves the independent cdd47b19 review report and the raw failing probe inputs/results. These are historical failure diagnostics, not passing evidence for the repaired source.

- `requirements-claim/` contains the original `TestExecutionRequirementsClaimAndRestartBlockWithoutChangingSnapshot` panic caused by rebinding the accepted digest before decoding the old fixture profile. The source failure is retained here; the repaired original test and all its assertions pass in `docs/validation/three-line-08-app-host/review-fixes/requirements-claim-03/`.
- `actual-app-requirements/` contains the test-file-only real app overlay showing the explicit-profile `contract_mismatch` and default-profile `stale_profile` failures. `requirements-overlay-original.json` is the exact reviewer input; `requirements-overlay.json` is a local rerun copy whose source points at the archived `.go.txt` file. The permanent `TestAppRequirementsClaimFreezesEvaluatedProfile` regression now passes both real app paths in `docs/validation/three-line-08-app-host/review-fixes/requirements-app-claim-03/`.
- The original reviewer report and PR comment copy preserve the complete findings and commands. A `.go.txt` extension keeps the archived overlay test source out of `go test ./...` package discovery.

Current passing commands and the source-bound L1 evidence under `l1/review-fixes-06/` are indexed in [test-mapping.json](../../test-mapping.json). The field comparison against the original evaluate/bind/freeze semantics is in [config-owner-comparison.md](../../recovery/config-owner-comparison.md).
