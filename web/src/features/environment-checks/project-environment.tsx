import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import { usePageVisible } from "../../shared/query-visibility";
import { Button } from "../../shared/ui/button";

import {
  EnvironmentObservation,
  environmentStage,
  useEnvironmentReadiness,
  type Environment,
} from "./use-readiness";
import { Link, useSearchParams } from "react-router-dom";
import { EnvironmentCard } from "./environment-card";
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
  const { api, identity } = useWorkspace();
  return (
    <EnvironmentWorkspace
      key={`${api.home}:${identity.project_id}:${identity.instance_id}`}
    />
  );
}
function EnvironmentWorkspace() {
  const { api, identity, online, busy, send } = useWorkspace();
  const visible = usePageVisible();
  const scope = [
    api.home,
    identity.project_id,
    identity.instance_id,
    "environment",
  ];
  const options = useEnvironmentReadiness();
  const [environmentKey, setEnvironmentKey] = useState("");
  const [allActivities, setAllActivities] = useState(false);
  const activeKey = useRef(environmentKey);
  activeKey.current = environmentKey;
  const initialized = useRef(false);
  const history = useQuery({
    queryKey: [...scope, "records", environmentKey, allActivities],
    queryFn: ({ signal }) =>
      api.get<{ records: Record[] }>(
        "/environment" +
          (!allActivities && environmentKey
            ? `?environment_key=${environmentKey}`
            : ""),
        signal,
      ),
    enabled: allActivities || !!environmentKey,
    retry: false,
    refetchInterval: visible ? 3000 : false,
  });
  const [params] = useSearchParams();
  const profile = params.get("profile") || "";
  const task = params.get("task");
  const [generator, setGenerator] = useState("");
  const materialQuery = useQuery({
    queryKey: [...scope, "materials", environmentKey],
    enabled: online && !!environmentKey,
    queryFn: ({ signal }) =>
      api.get<{
        document_path: string;
        document?: string;
        script_path?: string;
        source_commit?: string;
        target_facts: unknown;
        gaps?: string[];
        verification_diagnostic?: string;
        generation_ready: boolean;
        generation_diagnostic?: string;
      }>(`/environment/materials?environment_key=${environmentKey}`, signal),
    retry: false,
    refetchInterval: visible ? 15000 : false,
  });
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
    if (environmentKey) void materialQuery.refetch();
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
    const commandEnvironment = environmentKey;
    try {
      const applied = await send(action, target, version, payload);
      refresh();
      if (applied && action === "environment_start") {
        const refreshed = await options.refetch();
        const activity = refreshed.data?.profiles.find(
          (p) => p.id === payload.profile_id,
        )?.activity_id;
        if (activity && activeKey.current === commandEnvironment)
          setSelected(activity);
      }
    } finally {
      pending.current = false;
      setSubmitting(false);
    }
  };
  const records = history.data?.records ?? [];
  const displayed =
    detail.data &&
    selected === detail.data.id &&
    !records.some((r) => r.id === selected)
      ? [detail.data, ...records]
      : records;
  const focusProfile = options.data?.profiles.find((p) => p.id === profile);
  useEffect(() => {
    if (initialized.current || !options.verified) return;
    initialized.current = true;
    const initial = profile
      ? focusProfile
      : options.data?.profiles.find(
          (p) => p.id === options.data?.default_profile_id,
        );
    if (initial) setEnvironmentKey(initial.environment_key);
  }, [options.verified, options.data, profile, focusProfile]);
  const environment = environmentKey
    ? options.data?.environments.find(
        (e) => e.environment_key === environmentKey,
      )
    : undefined;
  const projectName =
    identity.repository_identity
      .replace(/\\/g, "/")
      .split("/")
      .filter((p) => p && p !== ".git")
      .at(-1) || "当前项目";
  const chooseEnvironment = (key: string) => {
    setEnvironmentKey(key);
    setSelected("");
    setGenerator("");
    setAllActivities(false);
  };
  const material = materialQuery.data;
  useEffect(() => {
    if (generator || !options.verified) return;
    const generators = options.data?.generators ?? [];
    const preferred =
      generators.length === 1
        ? generators[0]
        : generators.find((g) => g.id === options.data?.default_profile_id);
    if (preferred) setGenerator(preferred.id);
  }, [generator, options.verified, options.data]);
  useEffect(() => {
    const node = document.getElementById(
      `environment-${focusProfile?.environment_key || profile}`,
    );
    node?.scrollIntoView?.({ block: "nearest" });
    node?.focus();
  }, [focusProfile?.environment_key, profile, environmentKey]);
  useEffect(() => {
    if (!selected) return;
    const node = document.getElementById(`activity-${selected}`);
    node?.scrollIntoView?.({ block: "nearest" });
    node?.focus();
  }, [selected, detail.data?.id]);
  const groups = new Map<string, Environment[]>();
  for (const e of options.data?.environments || []) {
    const key = JSON.stringify([e.target.kind, e.target.environment || ""]);
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key)!.push(e);
  }
  const returnParams = new URLSearchParams();
  if (task) returnParams.set("task", task);
  if (profile) returnParams.set("start_profile", profile);
  return (
    <div className="settings-page environment-page">
      <h1>运行环境</h1>
      <Button variant="ghost" onClick={refresh}>
        刷新环境
      </Button>
      <p>
        每个项目独立验收执行环境；能力目录可见不等于验收通过。通过检查、完整
        L2、PR 发布核实及执行收尾后，才获得新任务的环境资格。
      </p>
      {task && (
        <Link to={`/projects/${identity.project_id}/runs?${returnParams}`}>
          返回任务
        </Link>
      )}
      <EnvironmentObservation />
      {options.error && <p role="alert">{String(options.error)}</p>}
      {profile && options.verified && !focusProfile && (
        <p role="alert">原执行方案已不存在或不属于开发方案，请重新选择。</p>
      )}
      <section className="settings-panel" aria-label="选择运行机与链路">
        <h2>选择运行机与链路</h2>
        {[...groups].map(([key, environments]) => (
          <section key={key}>
            <h3>
              {environments[0].target.kind === "local"
                ? "本机"
                : environments[0].name}
            </h3>
            <div className="actions">
              {environments.map((e) => (
                <Button
                  key={e.environment_key || e.profiles[0]?.id}
                  variant={
                    environmentKey === e.environment_key ? "default" : "ghost"
                  }
                  aria-pressed={
                    !!environmentKey && environmentKey === e.environment_key
                  }
                  disabled={!e.environment_key}
                  onClick={() => chooseEnvironment(e.environment_key)}
                >
                  {e.adapter === "codex-app-server"
                    ? "Codex 直连"
                    : `Orca → ${e.agent || "Codex"}`}{" "}
                  · 环境 {e.environment_key.slice(0, 8)} ·{" "}
                  {environmentStage[e.stage] ?? "状态待核实"}
                </Button>
              ))}
            </div>
          </section>
        ))}
      </section>
      {environmentKey && !environment && options.verified && (
        <p role="alert">
          原环境已不在当前配置中，请重新选择；项目全部活动仍可查看。
        </p>
      )}
      {!environment && (
        <p>请选择当前项目的运行机与链路，再生成材料或开始验收。</p>
      )}
      {options.verified && !options.data?.environments.length && (
        <p>
          尚无开发执行方案。
          <Link to="/settings/execution-configs">配置执行方案</Link>
        </p>
      )}
      {environment && (
        <section aria-label="当前准备对象">
          <h2>
            {projectName} /{" "}
            {environment.target.kind === "local" ? "本机" : environment.name} /{" "}
            {environment.adapter === "codex-app-server"
              ? "Codex 直连"
              : `Orca → ${environment.agent || "Codex"}`}
          </h2>
          <p role="status">
            {environmentStage[environment.stage] ?? "状态待核实"} ·{" "}
            {environment.reason}
          </p>
          <section className="settings-panel" id="environment-materials">
            <h2>准备要求与材料</h2>
            {materialQuery.isError && (
              <p role="alert">材料观察待核实：{String(materialQuery.error)}</p>
            )}
            <p>
              先阅读清单，在实际执行账号下准备工具、变量和权限，再开始验收。
            </p>
            {material && (
              <p className="path">
                {material.document_path} ·{" "}
                {material.script_path || "验收脚本见准备文档"}
              </p>
            )}
            {material?.source_commit && (
              <p>材料来源：{material.source_commit}</p>
            )}
            {material?.verification_diagnostic && (
              <p role="status">{material.verification_diagnostic}</p>
            )}
            <details>
              <summary>阅读项目准备清单</summary>
              <pre>
                {material?.document ||
                  "材料缺失或尚未读取，请核实后生成、审阅并提交准备文档与验收脚本。"}
              </pre>
            </details>
            <details key={environmentKey}>
              <summary>生成或更新准备材料</summary>
              {material?.generation_diagnostic && (
                <p role="status">{material.generation_diagnostic}</p>
              )}
              {material?.target_facts != null && (
                <details>
                  <summary>本次目标事实（受理时固定）</summary>
                  <pre>{JSON.stringify(material.target_facts, null, 2)}</pre>
                </details>
              )}

              {options.verified && !options.data?.generators.length && (
                <p>
                  尚无本地生成方案。
                  <Link to="/settings/execution-configs">配置生成方案</Link>
                </p>
              )}
              <p>
                生成准备文档与验收脚本后，审阅并通过项目正常 Git
                流程维护；生成成功不会获得任务准入。
              </p>
              <label>
                生成所用方案
                <select
                  aria-label="生成所用方案"
                  value={generator}
                  onChange={(e) => setGenerator(e.target.value)}
                >
                  <option value="">选择本机生成方案</option>
                  {options.data?.generators.map((p) => (
                    <option key={p.id} value={p.id}>
                      {[
                        p.name,
                        p.execution_location,
                        p.model,
                        p.reasoning_effort,
                      ]
                        .filter(Boolean)
                        .join(" · ")}
                    </option>
                  ))}
                </select>
              </label>
              <Button
                disabled={
                  !online ||
                  !options.verified ||
                  busy ||
                  submitting ||
                  !generator ||
                  !materialQuery.isSuccess ||
                  materialQuery.isError ||
                  !material?.generation_ready
                }
                onClick={() =>
                  void command("environment_generate", identity.project_id, 1, {
                    target_environment_key: environmentKey,
                    generator_profile_id: generator,
                  })
                }
              >
                生成文档和脚本
              </Button>
            </details>
          </section>

          <p>
            在此运行机的实际执行账号下，按本环境要求准备工具、依赖、变量及权限，然后运行验收。
          </p>
          <EnvironmentCard
            key={`${environmentKey}:${profile}`}
            environment={environment}
            project={identity.project_id}
            defaultProfile={options.data!.default_profile_id}
            preferred={profile}
            disabled={!options.verified || busy || submitting}
            command={command}
            onActivity={setSelected}
          />
        </section>
      )}
      <Button
        variant="ghost"
        onClick={() => {
          setAllActivities(!allActivities);
          setSelected("");
        }}
      >
        {allActivities ? "返回本环境历史" : "项目全部活动"}
      </Button>
      <section className="settings-panel">
        <h2>{allActivities ? "项目全部活动" : "本环境历史"}</h2>
        {history.error && <p role="alert">{String(history.error)}</p>}
        <p className="muted">最近 20 条活动，展开后查询原活动文件和报告。</p>
        {selected && detail.error && <p role="alert">{String(detail.error)}</p>}
        {displayed.map((summary) => {
          const r =
            selected === summary.id && detail.data ? detail.data : summary;
          return (
            <article
              className="profile"
              key={r.id}
              id={`activity-${r.id}`}
              tabIndex={-1}
            >
              <h3>
                {r.kind === "generation" ? "材料生成" : "环境验收"} ·{" "}
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
                  <details>
                    <summary>原执行文件与技术详情</summary>
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
                        验收结果文件（停止后封存一次）· v
                        {a.result?.version ?? 0} ·{" "}
                        {a.result?.collection_status ?? "等待停止后取得"}
                      </summary>
                      {a.result?.diagnostic && (
                        <p>结果采集诊断：{a.result.diagnostic}</p>
                      )}
                      <pre>{content(a.result)}</pre>
                    </details>
                  </details>
                </section>
              ))}
              {r.generated && (
                <div>
                  <p>
                    生成完成，产品未自动提交；请按正常 Git
                    流程维护，正式材料以项目提交为准。
                  </p>
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
