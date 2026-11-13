import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { useWorkspace } from "../../app/context";
import { Button } from "../../shared/ui/button";
import { SafeLink } from "../../shared/ui/status";
import type { ProjectReadiness } from "../../shared/api/types";
import "./refactoring-maintenance.css";
import { useActivityFeedback } from "../activities/feedback/feedback";
import { environmentLink } from "../environment-checks/use-readiness";

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
  direction_name: string;
  direction_prompt?: string;
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
  question?: {
    id: string;
    prompt: string;
    facts: string[];
    attempts: string[];
    options: { id: string; label: string; impact: string }[];
    recommended_option_id: string;
    recommendation_reason: string;
  };
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
type StartReadiness = {
  branch: string;
  project: ProjectReadiness;
  environment: "ready" | "not_ready" | "unverified" | "bypassed";
  reason?: string;
};
type DirectionCatalogue = {
  directions: { id: string; name: string; prompt: string; revision: string }[];
  diagnostics?: string[];
};
const phases: Record<string, string> = {
  first: "首轮调查",
  select: "选择下一轮",
  execute: "Runner 执行中",
  audit: "独立审计中",
  finish: "准备交付",
  verifying: "核验交付",
  report: "生成重构报告",
  evidence: "补充报告证据",
  version_audit: "复核 PR 版本",
  sync: "同步 PR 正文",
  sync_verifying: "核验 PR 正文",
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
  const [option, setOption] = useState("");
  const [error, setError] = useState("");
  const [startError, setStartError] = useState("");
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
  const catalogue = useQuery({
    queryKey: [...key, "directions"],
    queryFn: ({ signal }) =>
      api.get<DirectionCatalogue>("/refactoring/directions", signal),
    enabled: online,
    refetchOnWindowFocus: false,
  });
  const detail = useQuery({
    queryKey: [...key, selected],
    queryFn: ({ signal }) =>
      api.get<MaintenanceRecord>("/refactoring/" + selected, signal),
    enabled: online && /^[a-f0-9]{32}$/.test(selected),
    refetchInterval: 1500,
  });
  const working =
    (index.data?.records.some((r) => r.active) ?? false) ||
    (pendingRequests?.().some((r) => r.action === "refactoring_start") ??
      false);
  const selectedProfile = index.data?.profiles.find((p) => p.id === profile);
  const readiness = useQuery({
    queryKey: [...key, "readiness", profile],
    queryFn: ({ signal }) =>
      api.get<StartReadiness>(
        "/refactoring/readiness" +
          (profile ? "?" + new URLSearchParams({ profile }) : ""),
        signal,
      ),
    enabled: online && !working,
    retry: false,
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
    setOption("");
  }, [detail.data?.question?.id]);
  const current = detail.data;
  const directions = catalogue.data?.directions ?? [];
  const selectedDirection = directions.find(({ id }) => id === direction);
  async function command(
    action: string,
    target: string,
    revision: number,
    payload: Record<string, unknown>,
  ) {
    setError("");
    const starting = action === "refactoring_start";
    if (starting) setStartError("");
    feedback.notify("重构与维护");
    try {
      const receipt = await submit(
        api.envelope(action, target, revision, payload),
      );
      if (receipt?.status === "rejected") {
        const reason = receipt.diagnostic ?? "操作被拒绝";
        if (starting) {
          setStartError(reason);
          void readiness.refetch();
          void catalogue.refetch();
          void index.refetch();
        } else {
          setError(reason);
        }
        return;
      }
      const result = receipt?.refactoring as { id?: string } | undefined;
      if (result?.id) setParams({ action: result.id });
      await index.refetch();
      if (selected) await detail.refetch();
    } catch (e) {
      if (starting) setStartError(String(e));
      else setError(String(e));
    }
  }
  const disabled = !online || busy;
  const qualification =
    online && readiness.isSuccess ? readiness.data : undefined;
  const projectReady = qualification?.project.status === "ready";
  const environmentReady = qualification?.environment === "ready";
  const environmentBypassed = qualification?.environment === "bypassed";
  const canStart =
    !disabled &&
    !working &&
    index.isSuccess &&
    !!selectedDirection &&
    catalogue.isSuccess &&
    !catalogue.isFetching &&
    !!selectedProfile &&
    projectReady &&
    (environmentReady || environmentBypassed) &&
    !readiness.isFetching &&
    !!(dollars || minutes || rounds);
  const maintenanceParams = new URLSearchParams(params);
  maintenanceParams.set("maintenance", "preparation");
  return (
    <div className="refactoring-page">
      <header className="page-heading">
        <div>
          <h1>重构与维护</h1>
          <p>
            SteerAgent 只读调查与决策，Runner
            在独立工作区实施。与任务开发并行推进。
          </p>
        </div>
      </header>
      {(error || index.error || detail.error) && (
        <p className="notice amber" role="alert">
          {error || String(index.error ?? detail.error)}
        </p>
      )}
      <section className="settings-panel refactoring-panel">
        <h2>发起维护</h2>
        {working && (
          <p>
            已有维护行动在进行，启动配置已冻结；结束或停止后可发起新的维护。
          </p>
        )}
        <form
          hidden={working}
          onSubmit={(e) => {
            e.preventDefault();
            if (!canStart || !selectedDirection) return;
            void command("refactoring_start", identity.repository_identity, 1, {
              direction: selectedDirection.id,
              direction_revision: selectedDirection.revision,
              requirements,
              profile,
              strategy,
              budget: { dollars, minutes, rounds: rounds ? Number(rounds) : 0 },
            });
          }}
        >
          <label>
            重构方向
            <select
              required
              value={selectedDirection?.id ?? ""}
              onChange={(e) => setDirection(e.target.value)}
            >
              <option value="" disabled>
                {catalogue.isPending ? "正在读取重构方向…" : "选择重构方向"}
              </option>
              {[
                { label: "内置方向", custom: false },
                { label: "自定义方向", custom: true },
              ].map(({ label, custom }) => {
                const options = directions.filter(
                  ({ id }) => id.startsWith("custom:") === custom,
                );
                return (
                  options.length > 0 && (
                    <optgroup key={label} label={label}>
                      {options.map(({ id, name }) => (
                        <option key={id} value={id}>
                          {name}
                        </option>
                      ))}
                    </optgroup>
                  )
                );
              })}
            </select>
          </label>
          {catalogue.isError && (
            <p className="notice amber" role="alert">
              无法读取重构方向：{String(catalogue.error)}
            </p>
          )}
          {catalogue.data?.diagnostics?.map((diagnostic) => (
            <p className="notice amber" key={diagnostic}>
              {diagnostic}
            </p>
          ))}
          {selectedDirection && (
            <div className="refactoring-preview">
              <h3>方向提示词</h3>
              <pre aria-label="方向提示词" tabIndex={0}>
                {selectedDirection.prompt}
              </pre>
            </div>
          )}
          <div className="refactoring-fields refactoring-configuration">
            <label>
              执行配置
              <select
                required
                value={profile}
                onChange={(e) => {
                  setProfile(e.target.value);
                  setStartError("");
                }}
              >
                <option value="">选择执行配置</option>
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
          <p className="muted">
            至少设置一项预算；任一项耗尽后自动收尾，已有成果仍推进至 PR
            和报告，不因预算等待人工。
          </p>
          <p className={projectReady ? "muted" : "notice amber"}>
            {qualification
              ? `项目基线${projectReady ? "已就绪" : "未就绪"} · ${qualification.branch}${qualification.project.commit ? " · " + qualification.project.commit.slice(0, 12) : ""}`
              : "项目基线待核实"}
            {!!qualification?.project.blockers.length &&
              " · " + qualification.project.blockers.join("；")}
            {!projectReady && (
              <>
                {" "}
                ·{" "}
                <Link to={{ search: "?" + maintenanceParams }}>
                  打开项目维护
                </Link>
              </>
            )}
          </p>
          <p
            role="status"
            className={
              environmentReady || environmentBypassed ? "muted" : "notice amber"
            }
          >
            {!selectedProfile
              ? index.isSuccess && index.data.profiles.length === 0
                ? "没有可用于维护的本地 Codex 执行配置"
                : "请选择执行配置以核实运行环境"
              : environmentReady
                ? "所选运行环境已就绪"
                : environmentBypassed
                  ? "环境就绪检查已关闭"
                  : qualification?.reason ||
                    (readiness.isError
                      ? "运行环境状态查询失败，请刷新重试"
                      : "所选运行环境状态待核实")}
            {selectedProfile && !environmentReady && !environmentBypassed && (
              <>
                {" "}
                ·{" "}
                <Link
                  to={environmentLink(identity.project_id, selectedProfile.id)}
                >
                  打开项目运行环境
                </Link>
              </>
            )}
          </p>
          {startError && (
            <p className="notice amber" role="alert">
              {startError}
            </p>
          )}
          <Button variant="default" disabled={!canStart} type="submit">
            开始维护
          </Button>
        </form>
      </section>
      {current && (
        <section
          className="settings-panel refactoring-panel"
          aria-label="维护详情"
        >
          <h2>{phases[current.phase] ?? current.phase}</h2>
          <p>
            {current.direction_name} · 第 {current.round} 轮 · 已用{" "}
            {Math.floor(current.elapsed_seconds / 60)} 分钟
          </p>
          <p>
            起始提交 <code>{current.source_commit}</code>
          </p>
          {current.direction_prompt && (
            <details>
              <summary>方向提示词</summary>
              <pre>{current.direction_prompt}</pre>
            </details>
          )}
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
          {current.diagnostic && (
            <p className="notice amber" role="alert">
              {current.diagnostic}
            </p>
          )}
          <div className="refactoring-actions">
            {current.can_finish && (
              <Button
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
              </Button>
            )}
            {current.can_stop && (
              <Button
                variant="destructive"
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
              </Button>
            )}
            {current.can_conflict && (
              <Button
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
              </Button>
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
                  { question_id: current.question!.id, option_id: option, note: answer },
                );
              }}
            >
              <p className="refactoring-pre">{current.question.prompt}</p>
              <ul>
                {[...current.question.facts, ...current.question.attempts].map((fact, index) => (
                  <li key={index}>{fact}</li>
                ))}
              </ul>
              <fieldset>
                <legend>选择处理方式</legend>
                {current.question.options.map((choice) => (
                  <div key={choice.id}>
                    <label>
                      <input
                        type="radio"
                        name="maintenance-option"
                        value={choice.id}
                        checked={option === choice.id}
                        onChange={() => setOption(choice.id)}
                      />
                      {choice.label}
                    </label>
                    <p>{choice.impact}</p>
                    {current.question!.recommended_option_id === choice.id && (
                      <p>推荐 · {current.question!.recommendation_reason}</p>
                    )}
                  </div>
                ))}
              </fieldset>
              <label>
                补充说明（可选）
                <textarea
                  maxLength={32768}
                  value={answer}
                  onChange={(e) => setAnswer(e.target.value)}
                />
              </label>
              <Button variant="default" disabled={disabled || !option}>
                答复并继续
              </Button>
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
              <pre>
                {o.feedback ||
                  (o.state === "complete" ? "未返回文字" : "等待反馈")}
              </pre>
            </details>
          ))}
        </section>
      )}
      <section className="settings-panel refactoring-panel">
        <h2>维护历史</h2>
        {index.data?.records.length === 0 && <p>暂无维护行动</p>}
        <ul className="refactoring-history">
          {index.data?.records.map((r) => (
            <li key={r.id}>
              <Button
                size="sm"
                variant="ghost"
                aria-pressed={selected === r.id}
                onClick={() => setParams({ action: r.id })}
              >
                {r.direction_name} · {phases[r.phase] ?? r.phase} ·{" "}
                {r.id.slice(0, 8)}
              </Button>
            </li>
          ))}
        </ul>
      </section>
    </div>
  );
}
