import { useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import { usePageVisible } from "../../shared/query-visibility";
import { Button } from "../../shared/ui/button";

import { environmentStage, useEnvironmentReadiness } from "./use-readiness";
type Snapshot = {
  version: number;
  collection_status: string;
  content_base64?: string;
  diagnostic?: string;
};
type Round = {
  run?: {
    summary: string;
    diagnostic: string;
    agent_reply: string;
    agent_reply_verified: boolean;
  };
  number: number;
  deadline: string;
  run_id: string;
  settled: boolean;
  timed_out: boolean;
  diagnostic: string;
  progress?: Snapshot;
  result?: Snapshot;
  report?: {
    completed: boolean;
    checks: { id: string; status: string; message: string }[];
    l2: { test_id: string; status: string; message: string }[];
    pr: {
      url: string;
      head_revision: string;
      cleanup_completed: boolean;
      outcome_unknown: boolean;
    };
    message: string;
  };
};
type Record = {
  id: string;
  revision: number;
  kind: string;
  profile_id: string;
  state: string;
  status: string;
  settled: boolean;
  diagnostic: string;
  rounds: Round[];
  generated?: {
    document: string;
    script: string;
    gaps: string[];
    script_path: string;
  };
};
function content(snapshot?: Snapshot) {
  try {
    return snapshot?.content_base64
      ? new TextDecoder().decode(
          Uint8Array.from(atob(snapshot.content_base64), (c) =>
            c.charCodeAt(0),
          ),
        )
      : "尚无文件内容";
  } catch {
    return "文件内容不可显示";
  }
}
function download(name: string, body: string) {
  const url = URL.createObjectURL(
    new Blob([body], { type: "text/plain;charset=utf-8" }),
  );
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  a.click();
  URL.revokeObjectURL(url);
}
export function ProjectEnvironment() {
  const { api, identity, online, busy, send } = useWorkspace();
  const visible = usePageVisible();
  const scope = [
    api.home,
    identity.project_id,
    identity.instance_id,
    "environment",
  ];
  const options = useEnvironmentReadiness();
  const history = useQuery({
    queryKey: [...scope, "records"],
    queryFn: ({ signal }) =>
      api.get<{ records: Record[] }>("/environment", signal),
    retry: false,
    refetchInterval: visible ? 3000 : false,
  });
  const [profile, setProfile] = useState("");
  const [invalidateReason, setInvalidateReason] = useState("");
  const chosen = options.data?.profiles.find((p) => p.id === profile);
  const [generator, setGenerator] = useState("");
  const [timeout, setTimeout] = useState(3600);
  const [selected, setSelected] = useState("");
  const pending = useRef(false);
  const [submitting, setSubmitting] = useState(false);
  const detail = useQuery({
    queryKey: [...scope, selected],
    enabled: !!selected,
    queryFn: ({ signal }) =>
      api.get<Record>("/environment/" + selected, signal),
    retry: false,
    refetchInterval: visible && !!selected ? 3000 : false,
  });
  const refresh = () => {
    void options.refetch();
    void history.refetch();
    if (selected) void detail.refetch();
  };
  const command = async (
    action: string,
    target: string,
    version: number,
    payload: { [key: string]: unknown },
  ) => {
    if (pending.current) return;
    pending.current = true;
    setSubmitting(true);
    try {
      await send(action, target, version, payload);
      refresh();
    } finally {
      pending.current = false;
      setSubmitting(false);
    }
  };
  const records = history.data?.records ?? [];
  const displayed = detail.data && selected === detail.data.id && !records.some(r => r.id === selected)
    ? [detail.data, ...records] : records;
  return (
    <div className="settings-page">
      <h1>项目环境准备</h1>
      <p>
        目标环境通过完整 L2 和完整
        PR，且原执行收尾核实后，才允许执行新任务。每次最多三轮；状态仅为可执行任务或环境未就绪。
      </p>
      <section className="settings-panel">
        <h2>材料 → 用户准备 → 验收 → 结果</h2>
        <p>
          服务在线仅表示可接收操作。先阅读项目准备清单，在实际执行账号下准备工具、变量和权限，再明确开始完整
          L2 与真实验证 PR 验收；生成材料或基础检查不会授予任务资格。
        </p>
        {options.error && <p role="alert">{String(options.error)}</p>}
        {options.data?.environments?.map((e) => (
          <article key={e.environment_key || e.profiles[0]?.id}>
            <strong>
              {e.name} · {environmentStage[e.stage] ?? "准备状态待核实"}
            </strong>
            <p>{e.reason}</p>
            <p>执行方案：{e.profiles.map((p) => p.name).join("、")}</p>
            {e.source_commit && <p>材料来源：{e.source_commit}</p>}
            <details>
              <summary>1. 阅读项目准备清单 · {e.document_path}</summary>
              <pre>
                {e.document ||
                  "材料缺失，请先生成、审阅并提交准备文档与验收脚本。"}
              </pre>
            </details>
            {e.activity_id && (
              <Button
                variant="ghost"
                onClick={() => setSelected(e.activity_id!)}
              >
                查看当前验收
              </Button>
            )}
            {e.last_passed_activity_id && (
              <Button
                variant="ghost"
                onClick={() => setSelected(e.last_passed_activity_id!)}
              >
                查看历史通过报告
              </Button>
            )}
          </article>
        ))}
        <label>
          验收执行方案
          <select
            aria-label="验收执行方案"
            value={profile}
            onChange={(e) => setProfile(e.target.value)}
          >
            <option value="">选择本机或远端执行方案</option>
            {options.data?.profiles.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </label>
        <label>
          每轮总超时（秒）
          <input
            aria-label="每轮总超时（秒）"
            type="number"
            min={1}
            value={timeout}
            onChange={(e) => setTimeout(Number(e.target.value))}
          />
        </label>
        <Button
          disabled={
            !online ||
            busy ||
            submitting ||
            !chosen ||
            !Number.isInteger(timeout) ||
            timeout < 1
          }
          onClick={() =>
            void command("environment_start", identity.project_id, 1, {
              profile_id: profile,
              round_timeout_seconds: timeout,
              expected_current_revision: chosen!.current_revision,
            })
          }
        >
          2. 开始验收 / 重新准备
        </Button>
        <label>
          环境变化或失效原因
          <input
            aria-label="环境失效原因"
            value={invalidateReason}
            onChange={(e) => setInvalidateReason(e.target.value)}
            maxLength={4096}
          />
        </label>
        <Button
          disabled={
            !online ||
            busy ||
            submitting ||
            !chosen?.environment_key ||
            !invalidateReason.trim()
          }
          onClick={() =>
            void command("environment_invalidate", identity.project_id, 1, {
              environment_key: chosen!.environment_key,
              expected_current_revision: chosen!.current_revision,
              reason: invalidateReason,
            })
          }
        >
          标记当前项目环境失效
        </Button>
        <Button variant="ghost" onClick={refresh}>
          刷新环境
        </Button>
      </section>
      <section className="settings-panel">
        <h2>生成项目准备材料</h2>
        <p>
          生成准备文档与验收脚本后，审阅并通过项目正常 Git
          流程维护；生成成功不会获得任务准入。
        </p>
        <label>
          生成配置
          <select
            aria-label="生成配置"
            value={generator}
            onChange={(e) => setGenerator(e.target.value)}
          >
            <option value="">选择本地生成配置</option>
            {options.data?.generators.map((p) => (
              <option key={p.id} value={p.id}>
                {p.name}
              </option>
            ))}
          </select>
        </label>
        <Button
          disabled={!online || busy || submitting || !generator}
          onClick={() =>
            void command("environment_generate", identity.project_id, 1, {
              profile_id: generator,
              round_timeout_seconds: 0,
            })
          }
        >
          生成文档和脚本
        </Button>
      </section>
      <section className="settings-panel">
        <h2>适配与生成历史</h2>
        {history.error && <p role="alert">{String(history.error)}</p>}
        <p className="muted">最近 20 条活动，展开后查询原活动文件和报告。</p>
        {selected && detail.error && <p role="alert">{String(detail.error)}</p>}
        {displayed.map((summary) => {
          const r =
            selected === summary.id && detail.data ? detail.data : summary;
          return (
            <article className="profile" key={r.id}>
              <h3>
                {r.kind === "generation" ? "材料生成" : "环境适配"} ·{" "}
                {r.profile_id} · {r.status === "ready" ? "验收通过" : r.state}
              </h3>
              <p>
                <code>{r.id}</code>
              </p>
              <p>{r.diagnostic}</p>
              <Button variant="ghost" onClick={() => setSelected(r.id)}>
                查看活动详情
              </Button>
              {selected === r.id && detail.error && (
                <p role="alert">{String(detail.error)}</p>
              )}
              {r.rounds.map((a) => (
                <section key={a.number}>
                  <h4>
                    第 {a.number} 轮 ·{" "}
                    {a.settled ? "原执行已停止核实" : "待收尾"}
                  </h4>
                  <p>
                    截止 {a.deadline} · Run {a.run_id || "等待原请求核实"}
                    {a.timed_out && " · 已超时"}
                  </p>
                  <p>{a.diagnostic}</p>
                  {a.run?.summary && <p>原执行说明：{a.run.summary}</p>}
                  {a.run?.diagnostic && <p>{a.run.diagnostic}</p>}
                  {a.run?.agent_reply && (
                    <details>
                      <summary>
                        Agent 回复（
                        {a.run.agent_reply_verified
                          ? "原生来源已核实"
                          : "来源未核实"}
                        ，仅供排障）
                      </summary>
                      <pre>{a.run.agent_reply}</pre>
                    </details>
                  )}
                  <details>
                    <summary>
                      进度文件（不作为验收结论）· v{a.progress?.version ?? 0}
                    </summary>
                    <pre>{content(a.progress)}</pre>
                  </details>
                  <details>
                    <summary>
                      验收结果文件（停止后封存一次）· v{a.result?.version ?? 0}{" "}
                      · {a.result?.collection_status ?? "等待停止后取得"}
                    </summary>
                    {a.result?.diagnostic && (
                      <p>结果采集诊断：{a.result.diagnostic}</p>
                    )}
                    <pre>{content(a.result)}</pre>
                  </details>
                  {a.report && (
                    <div>
                      <p>{a.report.message}</p>
                      <h4>环境要求检查</h4>
                      {a.report.checks.map((c) => (
                        <p key={c.id}>
                          {c.id} ·{" "}
                          {c.status === "passed"
                            ? "通过"
                            : c.status === "failed"
                              ? "不通过"
                              : "未验证"}{" "}
                          · {c.message}
                        </p>
                      ))}
                      <h4>L2 验收</h4>
                      {a.report.l2.map((t) => (
                        <p key={t.test_id}>
                          {t.test_id} · {t.status} · {t.message}
                        </p>
                      ))}
                      <p>
                        PR：
                        {/^(https?):\/\//.test(a.report.pr.url) ? (
                          <a
                            href={a.report.pr.url}
                            target="_blank"
                            rel="noreferrer"
                          >
                            {a.report.pr.url}
                          </a>
                        ) : (
                          a.report.pr.url || "尚未取得"
                        )}{" "}
                        ·{" "}
                        {a.report.pr.outcome_unknown
                          ? "结果未知"
                          : a.report.pr.url && a.report.pr.head_revision
                            ? "已发布"
                            : "未完成"}{" "}
                        ·{" "}
                        {a.report.pr.cleanup_completed
                          ? "脚本资源已清理"
                          : "清理未完成"}
                      </p>
                    </div>
                  )}
                </section>
              ))}
              {r.generated && (
                <div>
                  <p>
                    缺项：
                    {r.generated.gaps.join("；") || "无已报告缺项，仍须审阅"}
                  </p>
                  <details>
                    <summary>准备文档</summary>
                    <pre>{r.generated.document}</pre>
                  </details>
                  <details>
                    <summary>验收脚本</summary>
                    <pre>{r.generated.script}</pre>
                  </details>
                  <Button
                    onClick={() =>
                      download("environment.md", r.generated!.document)
                    }
                  >
                    导出准备文档
                  </Button>
                  <Button
                    onClick={() =>
                      download(
                        r.generated!.script_path.split("/").pop()!,
                        r.generated!.script,
                      )
                    }
                  >
                    导出验收脚本
                  </Button>
                </div>
              )}
              {!r.settled && (
                <Button
                  disabled={!online || busy || submitting}
                  onClick={() =>
                    void command("environment_stop", r.id, r.revision, {})
                  }
                >
                  停止原活动
                </Button>
              )}
            </article>
          );
        })}
      </section>
    </div>
  );
}
