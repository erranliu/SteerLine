import { useWorkspace } from "../../app/context";
import type { Candidate } from "../../shared/api/types";
import { CopyExecutionInfo as CopyInfo } from "../activities/execution-info";

export function CopyExecutionInfo({ run }: { run: Candidate }) {
  const { identity } = useWorkspace();
  return (
    <CopyInfo
      info={{
        project: identity.repository_identity,
        title: "任务候选执行",
        taskID: run.domain_task_id || run.task_id,
        requestID: run.run_id,
        record: { id: run.run_id, label: "候选" },
        runner: { run_id: run.execution_id, attempt_id: run.attempt_id },
      }}
    />
  );
}
