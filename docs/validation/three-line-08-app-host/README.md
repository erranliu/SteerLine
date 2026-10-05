# Task 80 validation

The final source commit is `50627a412224a0afb52426e0c1606a5e915664fe`. The initial commit-bound L1 attempt on `11dd7f831c44c15de9c748d3bbe281d85bafbcf1` failed after surfacing conversation type ownership, scheduler/test boundary, and CLI fixture root errors. That failed run and all its raw logs remain under `l1/commit-bound-01/` and are listed in [test-mapping.json](test-mapping.json).

Against the final source commit, all four targeted groups passed with zero skips: host TM08 (3/3), app/host wiring (2/2 including its subtest), execution scheduler (2/2), and E2E snapshot sample (1/1). The final L1 run passed all 12 groups: 445 tests passed, zero failed, zero skipped, and all 27 selected packages passed. It includes the original TM04 three subtests, all TM08 host/app-host/boundary assertions, app project binding, and the scheduler lifecycle tests. Exact commands, parent/subtest hits, group counts, source SHA and runner summaries are recorded in [test-mapping.json](test-mapping.json); reports are under each group’s `commit-bound-02/` directory.

Full, L2 and live L3 were not selected. The controlled recovery scenario remains conditional on an actual recovery behavior change.
