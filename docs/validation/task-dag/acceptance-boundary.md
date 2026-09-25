# TASK-DAG acceptance boundary

The user confirmed this boundary on 2026-09-25. TASK-DAG Spec D1 excludes production worker/skill integration and directs this task to use isolated projects and substitutes. BEADS-EXIT B1 depends on TASK-DAG being accepted and merged, then owns native production integration. NT08, NT11 and NT22-DAG separate center rules from those downstream external actions so the dependency remains executable.

| Current row | TASK-DAG gate in this PR | BEADS-EXIT gate retained for production |
| --- | --- | --- |
| NT08 | Claim transaction saves one fixed Task/WorkflowRun/Execution/Attempt/Operation identity. Restart and same-request replay preserve the claim and occupied slot; an unknown dispatch cannot be treated as started or safely retried. | BEADS-EXIT NT16/NT17: actually prepare and start one worker, survive response loss and restart, reconcile its fixed external identity, and never start a duplicate worker. |
| NT11 | Verify pinned committed contract blobs, fixed base and every predecessor merge before claim. Persist the original contract digest/base with the selected attempt; reject changed contract evidence. | BEADS-EXIT NT12/NT13: the native worker verifies the actual workspace/base and supplied contract digest before executing; retry retains the same task contract. |
| NT22-DAG | Discard open/in-progress tasks only after exact unmerged-PR proof, retain unknown attempts and active resources, preserve cleanup identities, block successors, and record a confirmed late-merge conflict without TaskDelivery. | Add an explicit BEADS-EXIT acceptance row for a verified Stop on a live local worker, remote cancellation limits, exact open-PR/workspace cleanup, durable external receipts or unknown outcomes, and retention of unresolved resources. |

No requirement is dropped. The downstream column is explicit in BEADS-EXIT's Spec and Plan, while TASK-DAG's acceptance names its center gate. The live GitHub delivery transaction proves NT09's platform gate with isolated SQLite; the backend acceptance probe confirms that local Codex Stop itself is available, but native DAG wiring remains downstream.
