# Task 80 validation

Validation is registered in [test-mapping.json](test-mapping.json). It preserves the original 30 precommit runner summaries and later candidate results, including compile failures, the failed heartbeat attempt, a failed app test compile, and provisional passes. The first commit-bound L1 run on `11dd7f831c44c15de9c748d3bbe281d85bafbcf1` failed; its complete runner summary and raw logs remain under `l1/commit-bound-01/`. The failures led to repairs in the execution conversation ports, scheduler ownership, integration CLI fixture root, and boundary checks. The fixed Plan now registers a fresh commit-bound run after those source changes.

The ownership changes are tracked in [ownership.md](ownership.md). Full, L2 and live L3 are not selected by the fixed Plan. The controlled recovery scenario is conditional on an actual recovery behavior change.
