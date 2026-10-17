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
          <p>修复次数 · {run.audit.repair_count}</p>
          {!run.audit.results?.length && <p className="muted">尚无审计结果</p>}
          {run.audit.results?.map((result) => (
            <article key={result.command_id + result.step}>
              <p>
                {result.step} · {result.verdict || "尚无结论"}
              </p>
              {result.failure && (
                <p className="notice amber">{result.failure}</p>
              )}
              {result.findings?.map((finding, i) => (
                <p key={i}>
                  {finding.severity} · {finding.message} {finding.path}
                  {finding.line ? ":" + finding.line : ""}
                </p>
              ))}
              <details>
                <summary>结果来源</summary>
                <p>
                  报告 {result.source_report_command_id} · 命令{" "}
                  {result.command_id} · 轮次 {result.turn_id}
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
