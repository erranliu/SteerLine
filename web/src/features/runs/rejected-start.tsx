import type { CandidateStart } from "../../app/use-candidate-starts";
import { useProjectReadiness } from "../project-maintenance/use-readiness";

export function RejectedStart({ start }: { start: CandidateStart }) {
  const tests = useProjectReadiness();
  return (
    <>
      <div role="status" aria-label="项目就绪状态">
        <p>{tests.text}</p>
        {tests.diagnostic && <p className="muted">{tests.diagnostic}</p>}
        {tests.ready && <p className="muted">可在任务详情重新选择方案启动。</p>}
      </div>
      <details>
        <summary>上次启动结果 · {start.status}</summary>
        {start.diagnostic && <p>{start.diagnostic}</p>}
        <p className="muted">此记录保留当时的拒绝原因，未启动执行。</p>
      </details>
    </>
  );
}
