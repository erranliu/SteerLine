import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { useWorkspace } from "../../app/context";
import { SafeLink } from "../../shared/ui/status";
import type { ProjectReadiness } from "../../shared/api/types";
import "./refactoring-maintenance.css";
import { useActivityFeedback } from "../activities/feedback/feedback";

type Input = {
  direction: string;
  requirements: string;
  profile: string;
  strategy: string;
  budget: { dollars?: string; minutes?: string; rounds?: number };
};
export type MaintenanceRecord = {
  id: string;
  revision: number;
  phase: string;
  input: Input;
  round: number;
  active: boolean;
  source_commit: string;
  summary?: string;
  diagnostic?: string;
  elapsed_seconds: number;
  amount?: string;
  cost_complete: boolean;
  wall_seconds?: number;
  cost_breakdown?: {
    progress: { amount?: string };
    closing: { amount?: string };
  };
  report_stale?: boolean;
  question?: { id: string; text: string };
  pr?: { url: string };
  can_finish: boolean;
  can_stop: boolean;
  can_conflict: boolean;
  operations?: {
    id: string;
    purpose: string;
    round: number;
    state: string;
    feedback: string;
    prompt: string;
    run_id?: string;
  }[];
  reports?: {
    body: string;
    head: string;
    base: string;
    created_at: string;
    as_of?: string;
    cost?: string;
  }[];
};
type Index = {
  records: MaintenanceRecord[];
  profiles: { id: string; name: string }[];
  default_profile: string;
};
const directions = {
  simplify: "简化结构",
  cleanup: "清理冗余",
  boundaries: "理顺边界",
  readability: "改善可读性",
  "docs-tests": "文档与测试",
};
const phases: Record<string, string> = {
  select: "选择下一轮",
  execute: "Runner 执行中",
  audit: "独立审计中",
  finish: "准备交付",
  verifying: "核验交付",
  waiting_input: "等待决策",
  stopping: "正在停止",
  stopped: "已停止，可继续交付",
  blocked: "已阻断",
  delivered: "已交付",
};

export function RefactoringMaintenance() {
  const { api, identity, online, busy, submit, pendingRequests } =
    useWorkspace();
  const feedback = useActivityFeedback();
  const [params, setParams] = useSearchParams();
  const selected = params.get("action") ?? "";
  const [direction, setDirection] = useState("simplify");
  const [profile, setProfile] = useState("");
  const [requirements, setRequirements] = useState("");
  const [strategy, setStrategy] = useState("cautious");
  const [dollars, setDollars] = useState("");
  const [minutes, setMinutes] = useState("");
  const [rounds, setRounds] = useState("1");
  const [answer, setAnswer] = useState("");
  const [error, setError] = useState("");
  const key = [
    api.home,
    identity.project_id,
    identity.instance_id,
    "refactoring",
  ];
  const index = useQuery({
    queryKey: key,
    queryFn: ({ signal }) => api.get<Index>("/refactoring", signal),
    enabled: online,
    refetchInterval: 2000,
  });
  const detail = useQuery({
    queryKey: [...key, selected],
    queryFn: ({ signal }) =>
      api.get<MaintenanceRecord>("/refactoring/" + selected, signal),
    enabled: online && /^[a-f0-9]{32}$/.test(selected),
    refetchInterval: 1500,
  });
  const readiness = useQuery({
    queryKey: [...key, "readiness"],
    queryFn: ({ signal }) =>
      api.get<ProjectReadiness>("/maintenance/readiness", signal),
    enabled: online,
    refetchInterval: 10000,
  });
  useEffect(() => {
    if (
      !profile &&
      index.data?.profiles.some((p) => p.id === index.data.default_profile)
    )
      setProfile(index.data.default_profile);
  }, [index.data, profile]);
  useEffect(() => {
    if (!selected && index.data?.records[0])
      setParams({ action: index.data.records[0].id }, { replace: true });
  }, [index.data, selected, setParams]);
  useEffect(() => {
    setAnswer("");
  }, [detail.data?.question?.id]);
  const current = detail.data;
  const working =
    (index.data?.records.some((r) => r.active) ?? false) ||
    (pendingRequests?.().some((r) => r.action === "refactoring_start") ??
      false);
  async function command(
    action: string,
    target: string,
    revision: number,
    payload: Record<string, unknown>,
  ) {
    setError("");
    feedback.notify("重构与维护");
    try {
      const receipt = await submit(
        api.envelope(action, target, revision, payload),
      );
      if (receipt?.status === "rejected") {
        setError(receipt.diagnostic ?? "操作被拒绝");
        return;
      }
      const result = receipt?.refactoring as { id?: string } | undefined;
      if (result?.id) setParams({ action: result.id });
      await index.refetch();
      if (selected) await detail.refetch();
    } catch (e) {
      setError(String(e));
    }
  }
  const disabled = !online || busy;
  return (
    <div className="refactoring-page">
      <header>
        <h1>重构与维护</h1>
        <p>
          SteerAgent 只读调查与决策，Runner
          在独立工作区实施。与任务开发并行推进。
        </p>
      </header>
      {(error || index.error || detail.error) && (
        <p role="alert">{error || String(index.error ?? detail.error)}</p>
      )}
      <section className="refactoring-card">
        <h2>发起维护</h2>
        {working && (
          <p>
            已有维护行动在进行，启动配置已冻结；结束或停止后可发起新的维护。
          </p>
        )}
        <p>
          {readiness.data?.status === "ready"
            ? "项目维护已就绪"
            : "项目维护未就绪"}{" "}
          {readiness.data?.blockers?.join("；")}
        </p>
        <form
          hidden={working}
          onSubmit={(e) => {
            e.preventDefault();
            void command("refactoring_start", identity.repository_identity, 1, {
              direction,
              requirements,
              profile,
              strategy,
              budget: { dollars, minutes, rounds: rounds ? Number(rounds) : 0 },
            });
          }}
        >
          <div className="refactoring-fields">
            <label>
              重构方向
              <select
                value={direction}
                onChange={(e) => setDirection(e.target.value)}
              >
                {Object.entries(directions).map(([id, name]) => (
                  <option key={id} value={id}>
                    {name}
                  </option>
                ))}
              </select>
            </label>
            <label>
              执行配置
              <select
                required
                value={profile}
                onChange={(e) => setProfile(e.target.value)}
              >
                <option value="">选择已就绪配置</option>
                {index.data?.profiles.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name || p.id}
                  </option>
                ))}
              </select>
            </label>
            <label>
              决策策略
              <select
                value={strategy}
                onChange={(e) => setStrategy(e.target.value)}
              >
                <option value="cautious">谨慎决策</option>
                <option value="autonomous">完全自主</option>
                <option value="aggressive">激进决策</option>
              </select>
            </label>
          </div>
          <label>
            补充要求
            <textarea
              value={requirements}
              maxLength={32768}
              onChange={(e) => setRequirements(e.target.value)}
            />
          </label>
          <div className="refactoring-fields">
            <label>
              美元预算
              <input
                type="number"
                min="0.000001"
                step="any"
                value={dollars}
                onChange={(e) => setDollars(e.target.value)}
              />
            </label>
            <label>
              时间预算（分钟）
              <input
                type="number"
                min="0.000001"
                step="any"
                value={minutes}
                onChange={(e) => setMinutes(e.target.value)}
              />
            </label>
            <label>
              重构轮次
              <input
                type="number"
                min="1"
                max="10000"
                step="1"
                value={rounds}
                onChange={(e) => setRounds(e.target.value)}
              />
            </label>
          </div>
          <p>
            至少设置一项预算；任一项耗尽后自动收尾，已有成果仍推进至 PR
            和报告，不因预算等待人工。
          </p>
          <button
            disabled={
              disabled ||
              working ||
              !profile ||
              readiness.data?.status !== "ready" ||
              !(dollars || minutes || rounds)
            }
            type="submit"
          >
            开始维护
          </button>
        </form>
      </section>
      {current && (
        <section className="refactoring-card" aria-label="维护详情">
          <h2>{phases[current.phase] ?? current.phase}</h2>
          <p>
            {directions[current.input.direction as keyof typeof directions]} ·
            第 {current.round} 轮 · 已用{" "}
            {Math.floor(current.elapsed_seconds / 60)} 分钟
          </p>
          <p>
            起始提交 <code>{current.source_commit}</code>
          </p>
          <p>
            策略：
            {
              {
                cautious: "谨慎决策",
                autonomous: "完全自主",
                aggressive: "激进决策",
              }[current.input.strategy]
            }{" "}
            · 执行配置：{current.input.profile}
          </p>
          <p>
            预算：
            {current.input.budget.dollars
              ? "$" + current.input.budget.dollars
              : "美元未设置"}{" "}
            ·{" "}
            {current.input.budget.minutes
              ? current.input.budget.minutes + " 分钟"
              : "时间未设置"}{" "}
            ·{" "}
            {current.input.budget.rounds
              ? current.input.budget.rounds + " 轮"
              : "轮次未设置"}
          </p>
          <p>
            已覆盖参考费用：
            {current.amount == null ? "待采集" : "$" + current.amount}
            {!current.cost_complete && "（采集不完整）"}
          </p>
          {current.cost_breakdown && (
            <p>
              推进参考费用：
              {current.cost_breakdown.progress.amount == null
                ? "待采集"
                : "$" + current.cost_breakdown.progress.amount}{" "}
              · 收尾参考费用：
              {current.cost_breakdown.closing.amount == null
                ? "待采集"
                : "$" + current.cost_breakdown.closing.amount}
            </p>
          )}
          {current.wall_seconds !== undefined && (
            <p>
              总历时 {Math.floor(current.wall_seconds / 60)}{" "}
              分钟（含等待与离线）
            </p>
          )}
          <p>
            费用包含主会话与已登记内部调用；不覆盖子
            Agent、工具内部模型和不可观测供应商调用。
          </p>
          {current.summary && <p>{current.summary}</p>}
          {current.diagnostic && <p role="alert">{current.diagnostic}</p>}
          <div className="refactoring-actions">
            {current.can_finish && (
              <button
                disabled={disabled}
                onClick={() =>
                  void command(
                    "refactoring_finish",
                    current.id,
                    current.revision,
                    {},
                  )
                }
              >
                结束并交付
              </button>
            )}
            {current.can_stop && (
              <button
                disabled={disabled}
                onClick={() =>
                  void command(
                    "refactoring_stop",
                    current.id,
                    current.revision,
                    {},
                  )
                }
              >
                立即停止
              </button>
            )}
            {current.can_conflict && (
              <button
                disabled={disabled || working}
                onClick={() =>
                  void command(
                    "refactoring_conflict",
                    current.id,
                    current.revision,
                    {},
                  )
                }
              >
                处理冲突并更新报告
              </button>
            )}
            {current.pr && <SafeLink url={current.pr.url}>查看 PR</SafeLink>}
          </div>
          {current.question && (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void command(
                  "refactoring_answer",
                  current.id,
                  current.revision,
                  { question_id: current.question!.id, answer },
                );
              }}
            >
              <p className="refactoring-pre">{current.question.text}</p>
              <label>
                你的决定
                <textarea
                  required
                  maxLength={32768}
                  value={answer}
                  onChange={(e) => setAnswer(e.target.value)}
                />
              </label>
              <button disabled={disabled || !answer.trim()}>答复并继续</button>
            </form>
          )}
          {current.reports?.map((r, i) => (
            <article key={i}>
              <h3>审计报告 {i + 1}</h3>
              <p>
                head {r.head} · base {r.base}
              </p>
              <p>
                费用截止快照：{r.cost === undefined ? "未核实" : "$" + r.cost}
                {r.as_of && " · " + r.as_of}
              </p>
              {current.report_stale && i === current.reports!.length - 1 && (
                <p>目标分支已变化，此版本报告待复核。</p>
              )}
              <div className="refactoring-pre">{r.body}</div>
            </article>
          ))}
          <h3>轮次与执行反馈</h3>
          {current.operations?.map((o) => (
            <details key={o.id}>
              <summary>
                第 {o.round} 轮 · {o.purpose} · {o.state}
              </summary>
              <p>Runner：{o.run_id || "尚未提交"}</p>
              <h4>提示词</h4>
              <pre>{o.prompt}</pre>
              <h4>反馈</h4>
                <pre>{o.feedback || (o.state === "complete" ? "未返回文字" : "等待反馈")}</pre>
            </details>
          ))}
        </section>
      )}
      <section className="refactoring-card">
        <h2>维护历史</h2>
        {index.data?.records.length === 0 && <p>暂无维护行动</p>}
        <ul>
          {index.data?.records.map((r) => (
            <li key={r.id}>
              <button onClick={() => setParams({ action: r.id })}>
                {directions[r.input.direction as keyof typeof directions]} ·{" "}
                {phases[r.phase] ?? r.phase} · {r.id.slice(0, 8)}
              </button>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
