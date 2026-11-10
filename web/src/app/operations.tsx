import type { Candidate, Identity, StoredRequest } from "../shared/api/types";
import { Button } from "../shared/ui/button";
import { Status } from "../shared/ui/status";
import { Link } from "react-router-dom";
import {
  CopyExecutionInfo,
  requestExecutionInfo,
  operationNames,
} from "../features/activities/execution-info";
import { requestDestination } from "../features/activities/model";
export function Operations({
  requests,
  reconcile,
  reconciling,
  online,
  compact = false,
  scope,
  candidates = [],
}: {
  requests: StoredRequest[];
  reconcile: (row: StoredRequest) => void;
  reconciling: boolean;
  online: boolean;
  compact?: boolean;
  scope?: Pick<Identity, "project_id" | "instance_id">;
  candidates?: Candidate[];
}) {
  const current = (row: StoredRequest) =>
    !scope ||
    (row.envelope.project_id === scope.project_id &&
      row.envelope.instance_id === scope.instance_id);
  if (compact) {
    const rows = requests
      .filter((r) => current(r) && r.envelope.action.startsWith("environment_"))
      .sort((a, b) => a.created_at.localeCompare(b.created_at));
    const latest = rows.at(-1);
    const pending = rows.filter(
      (r) => !["applied", "rejected"].includes(r.response?.status ?? ""),
    );
    const displayed = rows.filter((r) => r === latest || pending.includes(r));
    const actions: Record<string, string> = {
      environment_start: "开始检测",
      environment_stop: "停止检测",
      environment_invalidate: "标记失效",
    };
    return displayed.length ? (
      <section
        className="environment-operation-results"
        aria-label="操作结果"
        aria-live="polite"
      >
        {displayed.map((row) => {
          const unresolved = pending.includes(row);
          const confirmingStart =
            unresolved && row.envelope.action === "environment_start";
          return (
            <div key={row.envelope.request_id}>
              <p>
                {actions[row.envelope.action] || "环境操作"}：
                {row.response?.status === "applied"
                  ? "操作成功"
                  : row.response?.status === "rejected"
                    ? "操作未成功，请刷新后重试"
                    : confirmingStart
                      ? "正在确认启动"
                      : "结果待核实"}
                {unresolved && (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={!online || reconciling}
                    onClick={() => reconcile(row)}
                  >
                    重新查询
                  </Button>
                )}
              </p>
              {row.response?.status !== "applied" &&
                (row.response?.diagnostic || row.error) && (
                  <p className="muted">
                    {row.response?.diagnostic || row.error}
                  </p>
                )}
              <CopyExecutionInfo
                info={requestExecutionInfo(row.envelope, row.response)}
              />
              {requestDestination(row.envelope, row.response) && (
                <Link to={requestDestination(row.envelope, row.response)!}>
                  查看对应记录
                </Link>
              )}
            </div>
          );
        })}
      </section>
    ) : null;
  }
  const latest = requests
    .filter(current)
    .slice()
    .sort((a, b) => b.created_at.localeCompare(a.created_at))[0];
  const urgent = requests.filter(
    (row) =>
      current(row) &&
      (!row.response ||
        !["applied", "rejected"].includes(row.response.status) ||
        (row === latest && row.response.status === "rejected")),
  );
  const history = requests
    .filter((row) => !urgent.includes(row))
    .slice()
    .reverse()
    .slice(0, 12);
  const names: Record<string, string> = {
    stop_candidate: "停止候选参与",
    answer_decision: "回答问题",
    open_native_session: "打开原生会话",
    resolve_pr_conflict: "解决合并冲突",
    pause_dispatch: "暂停派发",
    resume_dispatch: "恢复派发",
    set_dispatch_mode: "切换派发模式",
    main_agent_cancel: "取消提示词建议生成",
  };
  const target = (row: StoredRequest) => {
    const e = row.envelope;
    if (
      ["pause_dispatch", "resume_dispatch", "set_dispatch_mode"].includes(
        e.action,
      )
    )
      return "项目派发";
    const candidate = current(row)
      ? candidates.find(
          (run) =>
            run.run_id === e.target_id ||
            run.decision?.id === e.target_id ||
            (typeof e.payload.source_run_id === "string" &&
              typeof e.payload.source_attempt_id === "string" &&
              run.execution_id === e.payload.source_run_id &&
              run.attempt_id === e.payload.source_attempt_id),
        )
      : undefined;
    if (candidate)
      return `任务 #${candidate.domain_task_id || candidate.task_id} · 候选 C${candidate.candidate_number}`;
    if (["start_task", "start_candidate"].includes(e.action))
      return `任务 #${e.target_id}`;
    if (typeof e.payload.task_id === "string")
      return `任务 #${e.payload.task_id} · 候选待核实`;
    return current(row) ? "目标详情待核实" : "历史目标";
  };
  const renderRow = (row: StoredRequest) => (
    <div className="operation" key={row.envelope.request_id}>
      <div>
        <strong>
          {operationNames[row.envelope.action] ||
            names[row.envelope.action] ||
            "未知操作，详情待核实"}
        </strong>
        <span className="muted">{target(row)}</span>
        {row.error && <p className="notice amber">{row.error}</p>}
        {row.response?.diagnostic && (
          <p className="notice amber">{row.response.diagnostic}</p>
        )}
        {!["applied", "rejected"].includes(row.response?.status ?? "") &&
          Date.now() - Date.parse(row.created_at) >= 30000 && (
            <p className="notice amber">
              结果尚未确认 · 已等待{" "}
              {Math.floor((Date.now() - Date.parse(row.created_at)) / 1000)}{" "}
              秒；保留原请求，继续核实。
            </p>
          )}
        {row.checked_at && (
          <p className="muted">
            最后核实：{new Date(row.checked_at).toLocaleString("zh-CN")}
          </p>
        )}
        <CopyExecutionInfo
          info={requestExecutionInfo(row.envelope, row.response)}
        />
        {requestDestination(row.envelope, row.response) && (
          <Link to={requestDestination(row.envelope, row.response)!}>
            查看对应记录
          </Link>
        )}
      </div>
      <Status value={row.response?.status ?? "outcome_unknown"} />
      {!["applied", "rejected"].includes(row.response?.status ?? "") && (
        <Button
          size="sm"
          disabled={!online || reconciling}
          onClick={() => reconcile(row)}
        >
          查询原请求
        </Button>
      )}
    </div>
  );
  return requests.length ? (
    <section className="operations" aria-label="操作结果">
      {urgent.map(renderRow)}
      {!!history.length && (
        <details>
          <summary>操作回执 · 最近 {history.length} 项</summary>
          {history.map(renderRow)}
        </details>
      )}
    </section>
  ) : null;
}
