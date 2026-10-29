import type { Candidate } from "../../shared/api/types";

export function RunnerWorkflow({ run }: { run: Candidate }) {
  if (!run.runner_workflow && !run.execution_profile?.workflow_id) return null;
  return (
    <section className="settings-panel" aria-label="执行方案与审计">
      <h3>冻结方案 · {run.execution_profile?.name}</h3>
      {run.execution_profile?.details?.map((value, i) => (
        <p className="muted" key={i}>
          {value}
        </p>
      ))}
      {run.runner_workflow && (
        <p>
          Workflow · {run.runner_workflow.id}/{run.runner_workflow.version}
        </p>
      )}
      <p>
        当前轮次请求模型 · {run.requested_model || "由 Runner 决定"} ·{" "}
        {run.requested_reasoning_effort || "由 Runner 决定"}
      </p>
      {run.audit && (
        <>
          <h3>Runner 审计观察</h3>
          <p>
            审计请求模型 · {run.audit.model || "由 Runner 决定"} ·{" "}
            {run.audit.reasoning_effort || "由 Runner 决定"}
          </p>
          {!run.audit.results?.length && <p className="muted">尚无审计结果</p>}
          {run.audit.results?.map((result) => (
            <article key={result.command_id + result.step}>
              <p>
                {result.step} ·{" "}
                {result.failure
                  ? "未核实"
                  : result.fixed_count == null
                    ? "尚无完整结果"
                    : "已采集"}
              </p>
              {result.failure && (
                <p className="notice amber">{result.failure}</p>
              )}
              {result.fixed_count != null && (
                <p>修复数量 · {result.fixed_count}</p>
              )}
              {result.fixed_issues?.map((issue, i) => (
                <p key={i}>
                  {issue.severity} · {issue.issue} · 修复：{issue.repair} ·
                  验证：{issue.evidence}
                </p>
              ))}
              {result.remaining_issues?.map((issue, i) => (
                <p key={i}>
                  {issue.severity} · {issue.issue} · 未解决：{issue.reason}
                </p>
              ))}
              <details>
                <summary>结果来源</summary>
                <p>
                  实施结果 {result.source_result_id} · 命令 {result.command_id}{" "}
                  · 轮次 {result.turn_id}
                </p>
                <p>
                  实施命令 {result.implementation_source.command_id} · 轮次{" "}
                  {result.implementation_source.turn_id} · 尝试{" "}
                  {result.implementation_source.attempt_id} · 输入版本{" "}
                  {result.implementation_source.input_version} · 摘要{" "}
                  {result.implementation_source.final_digest}
                </p>
                <p>
                  审计结果 {result.result_id} · Git {result.pre_head} →{" "}
                  {result.post_head}
                </p>
              </details>
            </article>
          ))}
        </>
      )}
      {!!run.materials?.length && (
        <details>
          <summary>冻结材料</summary>
          {run.materials.map((m, i) => (
            <p key={i}>
              {m.kind} · {m.path} · {m.commit} · {m.digest}
            </p>
          ))}
        </details>
      )}
    </section>
  );
}
