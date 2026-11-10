import type { Candidate } from "../../shared/api/types";
import { stageLabel } from "./presentation";
export function RunnerWorkflow({ run }: { run: Candidate }) {
  if (!run.audit) return null;
  return (
    <div aria-label="审查结果">
      {!run.audit.results?.length && <p className="muted">尚无审查结果</p>}
      {run.audit.results?.map((result, index) => (
        <article key={result.command_id + result.step}>
          <p>
            审查 {index + 1} · {stageLabel(result.step)} ·{" "}
            {result.failure
              ? "未核实"
              : result.fixed_count == null
                ? "尚无完整结果"
                : "已采集"}
            {result.fixed_count != null &&
              ` · ${result.failure ? "待核实修复数量" : "修复数量"} ${result.fixed_count}`}
            {!result.failure && result.fixed_count != null
              ? ` · 遗留问题 ${result.remaining_issues?.length ?? 0}`
              : result.remaining_issues?.length
                ? ` · 已报告遗留问题 ${result.remaining_issues.length}，完整数量待核实`
                : " · 遗留数量待核实"}
          </p>
          {result.failure && <p className="notice amber">{result.failure}</p>}
          {result.remaining_issues?.map((issue, i) => (
            <p className="notice amber" key={i}>
              {issue.severity} · {issue.issue} · 未解决：{issue.reason}
            </p>
          ))}
          {!!result.fixed_issues?.length && (
            <details>
              <summary>修复说明与验证证据</summary>
              {result.fixed_issues.map((issue, i) => (
                <p key={i}>
                  {issue.severity} · {issue.issue} · 修复：{issue.repair} ·
                  验证：{issue.evidence}
                </p>
              ))}
            </details>
          )}
        </article>
      ))}
    </div>
  );
}
