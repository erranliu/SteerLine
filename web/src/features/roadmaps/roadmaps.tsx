import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { useWorkspace } from "../../app/context";
import { SafeLink } from "../../shared/ui/status";
import {
  contentPending,
  currentRoadmapRecord,
  roadmapProgress,
  roadmapNextAction,
  roadmapSourceChanged,
  roadmapSteps,
} from "./workflow";
import "./roadmaps.css";
import { DependencyReview } from "./dependency-review";

export type RoadmapSource = {
  id: string;
  path: string;
  body: string;
  digest: string;
  head: string;
  modified: boolean;
  diagnostic?: string;
};
export type Dependencies = { local: string[]; formal: string[] };
export type RoadmapTask = {
  local_key: string;
  key_origin: "markdown" | "suggested";
  title: string;
  goal: string;
  scope: string;
  constraints: string;
  acceptance: string;
  evidence: string;
  original_dependencies: Dependencies;
  proposed_dependencies: Dependencies;
  reason: string;
  existing_task_id: string;
};
type File = { path: string; body: string };
type Documents = {
  local_key: string;
  spec: File;
  plan: File;
  tests: string[];
  acceptance: File[];
};
export type RoadmapRecord = {
  id: string;
  purpose?: string;
  parent_id?: string;
  code_commit?: string;
  design?: string;
  investigation?: {
    source: "repository" | "input_file";
    path: string;
    sha256: string;
    reason: string;
  }[];
  document_questions?: string[];
  publication_preview?: {
    repository: string;
    branch: string;
    files: { path: string; before: string; after: string }[];
  };
  source: RoadmapSource;
  revision: number;
  created_at: string;
  state: string;
  existing?: { id: string; title: string; status?: string }[];
  diagnostic?: string;
  run_id?: string;
  analysis?: { tasks: RoadmapTask[]; questions: string[]; overlap: string };
  confirmed_digest?: string;
  documents: Documents[];
  formal_mapping: Record<string, string>;
  cancelled: boolean;
  publication?: {
    url: string;
    state: string;
    merged_commit?: string;
    diagnostic?: string;
  };
  admission_request?: string;
  review_digest?: string;
};
type Preparation = {
  commit: string;
  ready: boolean;
  local_ready: boolean;
  branch: string;
  files: { path: string; ready: boolean; diagnostic?: string }[];
  diagnostic?: string;
};
export type RoadmapView = {
  preparation: Preparation;
  sources: RoadmapSource[];
  records: RoadmapRecord[];
  profiles: { id: string; name: string }[];
  documents_available: boolean;
  documents_reason: string;
};
export const roadmapQueryKey = (
  home: string,
  project: string,
  instance: string,
) => [home, "roadmaps", project, instance];
export function Roadmaps() {
  const { api, identity, online, submit, busy, refresh, requests } =
    useWorkspace();
  const [params, setParams] = useSearchParams();
  const [profile, setProfile] = useState(""),
    [error, setError] = useState(""),
    [tab, setTab] = useState<"source" | "json">("source");
  const [documentKey, setDocumentKey] = useState(""),
    [kind, setKind] = useState<"spec" | "plan" | "tests" | "acceptance">(
      "spec",
    );
  const [editing, setEditing] = useState(false),
    [body, setBody] = useState("");
  const key = roadmapQueryKey(
    api.home,
    identity.project_id,
    identity.instance_id,
  );
  const list = useQuery({
    queryKey: key,
    queryFn: () => api.get<RoadmapView>("/roadmaps"),
    enabled: online,
    retry: false,
    refetchInterval: online ? 5000 : false,
  });
  const value = list.data,
    sourceID =
      value?.records.find((r) => r.id === params.get("record"))?.source.id ||
      params.get("source") ||
      value?.sources[0]?.id ||
      "";
  const source = useQuery({
    queryKey: [
      ...key,
      "source",
      sourceID,
      value?.sources.find((s) => s.id === sourceID)?.digest,
    ],
    queryFn: () => api.get<RoadmapSource>("/roadmaps/sources/" + sourceID),
    enabled: online && !!sourceID,
    retry: false,
  });
  const history = (value?.records || [])
    .filter((r) => r.source.id === sourceID)
    .sort((a, b) => b.created_at.localeCompare(a.created_at));
  const currentSummary = currentRoadmapRecord(history);
  const currentID = currentSummary?.id || "";
  const historicalID = params.get("record");
  const viewingHistory = !!historicalID && historicalID !== currentID;
  const selectedID = historicalID || currentID;
  const currentDetail = useQuery({
    queryKey: [...key, "record", currentID],
    queryFn: () => api.get<RoadmapRecord>("/roadmaps/" + currentID),
    enabled: online && !!currentID,
    retry: false,
    refetchInterval: online ? 2000 : false,
  });
  const detail = useQuery({
    queryKey: [...key, "record", selectedID],
    queryFn: () => api.get<RoadmapRecord>("/roadmaps/" + selectedID),
    enabled: online && !!selectedID && history.some((r) => r.id === selectedID),
    retry: false,
  });
  const current = currentDetail.data || currentSummary;
  const record = viewingHistory
    ? history.some((r) => r.id === historicalID)
      ? detail.data
      : undefined
    : currentDetail.data;
  const analysis = record?.analysis;
  const locked =
    busy ||
    !online ||
    viewingHistory ||
    !!detail.error ||
    !!currentDetail.error ||
    !!list.error ||
    !!source.error;
  const progress = roadmapProgress(current);
  const currentStale =
    !!currentDetail.data &&
    !!source.data &&
    roadmapSourceChanged(currentDetail.data.source, source.data);
  function returnToCurrent() {
    setParams({ source: sourceID });
    setEditing(false);
    setError("");
  }
  const stale =
    !!record &&
    !!source.data &&
    roadmapSourceChanged(record.source, source.data);
  const currentDocument =
    record?.documents?.find((d) => d.local_key === documentKey) ||
    record?.documents?.[0];
  const file =
    kind === "tests"
      ? currentDocument && {
          path: currentDocument.plan.path.replace(/\.plan\.md$/, ".tests.json"),
          body:
            JSON.stringify({ tests: currentDocument.tests }, null, 2) + "\n",
        }
      : kind === "acceptance"
        ? currentDocument?.acceptance[0]
        : currentDocument?.[kind];
  const generationParent =
    record?.purpose === "documents"
      ? history.find((r) => r.id === record.parent_id)
      : record;
  const dag = (id: string) =>
    "/projects/" + identity.project_id + "/dag?task=" + encodeURIComponent(id);
  async function action(
    name: string,
    target: string,
    version: number,
    payload: Record<string, unknown> = {},
  ) {
    setError("");
    try {
      const envelope = api.envelope(name, target, version, payload);
      if (name === "roadmap_parse")
        envelope.payload.request_id = envelope.request_id;
      const result = await submit(envelope);
      if (!result)
        throw new Error("未取得回执，请在原请求记录中查询，避免重复提交");
      if (result.status === "rejected" || result.status === "outcome_unknown")
        setError(result.diagnostic || "原操作待核实");
      if (!viewingHistory && params.has("record"))
        setParams({ source: sourceID }, { replace: true });
      await list.refetch();
      if (selectedID) await detail.refetch();
      if (currentID && selectedID !== currentID) await currentDetail.refetch();
      refresh();
    } catch (e) {
      setError(String(e));
    }
  }
  const preparationPending = requests?.some(
    (r) =>
      r.envelope.project_id === identity.project_id &&
      r.envelope.action === "roadmap_prepare_static" &&
      !["applied", "rejected"].includes(r.response?.status || ""),
  );
  const parse = () =>
    source.data &&
    action("roadmap_parse", source.data.id, 1, {
      path: source.data.path,
      source_digest: source.data.digest,
      profile: profile || value?.profiles[0]?.id || "",
    });
  const hasPending = history.some((r) =>
    contentPending(r.id === current?.id ? current : r),
  );
  const publicationPending = history
    .map((r) => (r.id === current?.id ? current : r))
    .some((r) => !!r.publication && r.state !== "admitted");
  const parseDisabled =
    locked ||
    !source.data ||
    !!source.data.diagnostic ||
    hasPending ||
    publicationPending ||
    !value?.profiles.length ||
    !value?.preparation.ready;
  const generateDisabled =
    locked ||
    !generationParent ||
    hasPending ||
    publicationPending ||
    stale ||
    generationParent.state !== "confirmed" ||
    !value?.documents_available ||
    !value?.preparation.ready;
  const mainAction = roadmapNextAction(
    current,
    currentStale && !hasPending && !publicationPending,
  );
  useEffect(() => {
    setEditing(false);
    setDocumentKey("");
    setError("");
  }, [sourceID, selectedID]);
  const guidance = ["blocked", "documents-blocked"].includes(
    current?.state || "",
  )
    ? "请按下方待澄清问题在外部补充草稿，再重新解析任务与依赖。"
    : currentStale
      ? "草稿已修改，当前流程保留原快照。采用新内容需重新解析和确认。"
      : mainAction === "confirm"
        ? "请核对下方任务范围、原有依赖与拟议依赖，再确认本次规划。"
        : mainAction === "generate"
          ? "任务与依赖已确认，下一步生成逐任务 Spec / Plan 和验收材料。"
          : mainAction === "publish"
            ? "请审阅下方完整文档与发布差异。发布会创建文档 PR 并按展示内容合并。"
            : mainAction === "admit"
              ? "文档合并已核实。入图将创建正式任务并分配编号；自动派发开启时，就绪任务可能启动。"
              : mainAction === "dag"
                ? "本次转正已完成，执行状态请查看正式任务 DAG。"
                : mainAction === "resume" || mainAction === "observe"
                  ? "沿原请求核实结果，暂不重新提交。"
                  : mainAction === "wait"
                    ? "当前内容请求正在处理，结果将在此更新。"
                    : "解析草稿，核对任务范围、已有正式任务和依赖。";
  return (
    <section className="roadmaps-page" aria-label="Roadmap 草稿">
      <header className="page-header">
        <div>
          <h1>Roadmap 草稿</h1>
          <p>
            读取 Markdown → 核对任务与依赖 → 确认 → Spec / Plan → 发布合并 → 入
            DAG 时分配编号
          </p>
        </div>
        <button
          disabled={busy || !online}
          onClick={() => {
            void list.refetch();
            void source.refetch();
            void detail.refetch();
            void currentDetail.refetch();
          }}
        >
          刷新 Markdown 与观察
        </button>
      </header>
      {(error ||
        list.error ||
        source.error ||
        detail.error ||
        currentDetail.error) && (
        <p className="notice amber" role="alert">
          {error ||
            String(
              list.error || source.error || detail.error || currentDetail.error,
            )}
        </p>
      )}
      {!online && (
        <p className="notice amber">
          连接待核实 · 以下为最后观察，暂不能提交操作。
        </p>
      )}
      {value && !value.preparation.ready && (
        <section aria-label="Roadmap 静态材料初始化" className="notice">
          <h2>先准备项目静态材料</h2>
          <p>
            静态规则只需初始化一次；任务图、来源映射和确认规划由 SteerLine
            自动提供，无需手写 JSON。
          </p>
          <ul>
            {value.preparation.files.map((file) => (
              <li key={file.path}>
                {file.path} · {file.ready ? "固定提交可读取" : "未就绪"}
              </li>
            ))}
          </ul>
          <p>
            {value.preparation.local_ready
              ? "材料已在工作树中，请提交并合入 " +
                value.preparation.branch +
                "，更新本地后刷新。"
              : "初始化仅生成缺少的静态文件，保留已有项目指南；请审阅后按项目正常流程提交并发布。"}
          </p>
          {value.preparation.diagnostic && (
            <p role="alert">{value.preparation.diagnostic}</p>
          )}
          <button
            disabled={
              locked ||
              preparationPending ||
              !value.preparation.commit ||
              value.preparation.local_ready
            }
            onClick={() =>
              void action("roadmap_prepare_static", identity.project_id, 1, {
                commit: value.preparation.commit,
              })
            }
          >
            初始化静态材料
          </button>
        </section>
      )}
      <section
        className="roadmaps-progress"
        aria-label="当前草稿转正进度"
        aria-busy={!!current && contentPending(current)}
      >
        <div className="roadmaps-progress-heading">
          <h2>
            {source.data?.path.split("/").at(-1) || "选择草稿"} · 当前流程
          </h2>
          <strong role="status">
            {list.isPending ? "读取进度中" : progress.label}
          </strong>
        </div>
        <ol aria-label="转正阶段" className="roadmaps-steps">
          {roadmapSteps.map((step, index) => (
            <li
              key={step}
              className={
                index < progress.step
                  ? "complete"
                  : index === progress.step
                    ? "current"
                    : ""
              }
              aria-current={index === progress.step ? "step" : undefined}
            >
              <span aria-hidden="true">
                {index < progress.step ? "✓" : index + 1}
              </span>
              {step}
            </li>
          ))}
        </ol>
        <p>{guidance}</p>
        {current?.diagnostic && (
          <details className="roadmaps-diagnostic">
            <summary>当前流程问题</summary>
            <p>{current.diagnostic}</p>
          </details>
        )}
        {mainAction === "generate" && !value?.documents_available && (
          <p className="notice amber">{value?.documents_reason}</p>
        )}
        {viewingHistory ? (
          <div className="roadmaps-history-notice" role="note">
            正在查看历史记录，操作已禁用。
            <button onClick={returnToCurrent}>返回当前流程</button>
          </div>
        ) : (
          <div className="roadmaps-actions">
            {mainAction === "parse" && (
              <button
                className="roadmaps-primary"
                disabled={parseDisabled}
                onClick={() => void parse()}
              >
                Runner generate · 解析任务与依赖
              </button>
            )}
            {mainAction === "confirm" && (
              <button
                className="roadmaps-primary"
                disabled={
                  locked ||
                  !record ||
                  record.state !== "review" ||
                  stale ||
                  !!analysis?.questions.length
                }
                onClick={() =>
                  record &&
                  void action("roadmap_confirm", record.id, record.revision)
                }
              >
                确认本次任务与依赖（{analysis?.tasks.length ?? 0} 项）
              </button>
            )}
            {mainAction === "generate" && (
              <button
                className="roadmaps-primary"
                disabled={generateDisabled}
                onClick={() =>
                  generationParent &&
                  void action(
                    "roadmap_generate_documents",
                    generationParent.id,
                    generationParent.revision,
                  )
                }
              >
                生成 Spec / Plan
              </button>
            )}
            {mainAction === "wait" && (
              <button className="roadmaps-primary" disabled>
                {progress.label}
              </button>
            )}
            {mainAction === "observe" && (
              <button
                className="roadmaps-primary"
                disabled={locked || !record}
                onClick={() =>
                  record && void action("roadmap_observe", record.id, 1)
                }
              >
                查询原操作结果
              </button>
            )}
            {mainAction === "resume" && (
              <button
                className="roadmaps-primary"
                disabled={locked || !record}
                onClick={() =>
                  record &&
                  void action("roadmap_resume", record.id, record.revision)
                }
              >
                继续原发布 / 入图请求
              </button>
            )}
            {mainAction === "publish" && (
              <button
                className="roadmaps-primary"
                disabled={locked || !record || editing || stale}
                onClick={() =>
                  record &&
                  void action("roadmap_publish", record.id, record.revision, {
                    review_digest: record.review_digest || "",
                    merge: true,
                  })
                }
              >
                确认发布并合并文档
              </button>
            )}
            {mainAction === "admit" && (
              <button
                className="roadmaps-primary"
                disabled={locked || !record}
                onClick={() =>
                  record &&
                  void action("roadmap_admit", record.id, record.revision)
                }
              >
                正式入 DAG · 分配编号
              </button>
            )}
            {mainAction === "dag" && (
              <Link
                className="roadmaps-primary"
                to={"/projects/" + identity.project_id + "/dag"}
              >
                查看正式任务 DAG
              </Link>
            )}
            {record && mainAction !== "observe" && mainAction !== "dag" && (
              <button
                disabled={locked}
                onClick={() => void action("roadmap_observe", record.id, 1)}
              >
                查询原操作结果
              </button>
            )}
            {record && contentPending(record) && (
              <button
                disabled={locked}
                onClick={() => void action("roadmap_stop", record.id, 1)}
              >
                停止当前内容请求
              </button>
            )}
            {analysis && <a href="#roadmap-dependencies">查看任务与依赖</a>}
            {!!record?.documents?.length && (
              <a href="#roadmap-documents">审阅文档</a>
            )}
          </div>
        )}
      </section>
      <div className="roadmaps-grid">
        <aside className="roadmaps-library">
          <h2>仓库 Markdown</h2>
          <p>草稿在外部维护，无需配套 JSON。</p>
          {value?.sources.map((s) => (
            <button
              key={s.id}
              className={s.id === sourceID ? "active" : ""}
              onClick={() => {
                setParams({ source: s.id });
                setEditing(false);
                setError("");
              }}
            >
              <strong>{s.path.split("/").at(-1)}</strong>
              <span>{s.path}</span>
              <span className="roadmaps-source-status">
                {
                  roadmapProgress(
                    currentRoadmapRecord(
                      (value?.records || [])
                        .filter((r) => r.source.id === s.id)
                        .map((r) => (r.id === current?.id ? current : r)),
                    ),
                  ).label
                }
              </span>
              {s.diagnostic && <span>{s.diagnostic}</span>}
            </button>
          ))}
          {!list.isFetching && !value?.sources.length && (
            <p>drafts 目录下暂无 Markdown 草稿。</p>
          )}
        </aside>
        <article className="roadmaps-document">
          <h2>{source.data?.path.split("/").at(-1) || "选择草稿"}</h2>
          <p className="muted">{source.data?.path}</p>
          <p>
            来源：
            {source.data?.modified
              ? "工作树内容（含未提交变化）"
              : "工作树文件"}
            {source.data?.head && " · HEAD " + source.data.head.slice(0, 8)}
          </p>
          <div className="roadmaps-actions">
            <button
              aria-pressed={tab === "source"}
              onClick={() => setTab("source")}
            >
              Markdown 原文
            </button>
            <button
              aria-pressed={tab === "json"}
              onClick={() => setTab("json")}
            >
              SteerLine 生成的 JSON
            </button>
          </div>
          {tab === "source" ? (
            <textarea
              aria-label="Markdown 原文 · 只读"
              readOnly
              value={source.data?.body || ""}
              rows={22}
            />
          ) : (
            <pre>
              {record
                ? JSON.stringify(
                    {
                      roadmap_id: record.source.id,
                      source_digest: record.source.digest,
                      confirmed: !!record.confirmed_digest,
                      tasks: analysis?.tasks || [],
                      formal_mapping: record.formal_mapping,
                    },
                    null,
                    2,
                  )
                : "尚未解析，无需提供 JSON。"}
            </pre>
          )}
          {stale && (
            <p className="notice amber">
              来源已变化，旧解析保持原快照。采用新内容前请重新解析和确认。
            </p>
          )}
        </article>
        <aside className="roadmaps-controls">
          <h2>当前流程与历史</h2>
          <p>解析和文档生成按同一次规划关联。历史仅供查看。</p>
          <details open={viewingHistory}>
            <summary>历史记录（{history.length}）</summary>
            <label>
              查看历史记录
              <select
                value={historicalID || ""}
                onChange={(e) => {
                  setParams(
                    e.target.value
                      ? { source: sourceID, record: e.target.value }
                      : { source: sourceID },
                  );
                  setEditing(false);
                  setError("");
                }}
              >
                <option value="">当前流程</option>
                {history.map((r) => (
                  <option key={r.id} value={r.id}>
                    {r.purpose === "documents" ? "文档生成 · " : "任务解析 · "}
                    {new Date(r.created_at).toLocaleString()} ·{" "}
                    {roadmapProgress(r).label}
                  </option>
                ))}
              </select>
            </label>
            {record?.parent_id && (
              <p>
                关联的任务解析：
                {history.find((r) => r.id === record.parent_id)?.created_at ||
                  record.parent_id}
              </p>
            )}
          </details>
          <details>
            <summary>重新解析与生成方案</summary>
            <label>
              内容生成方案
              <select
                value={profile || value?.profiles[0]?.id || ""}
                onChange={(e) => setProfile(e.target.value)}
                disabled={viewingHistory}
              >
                {value?.profiles.map((p) => (
                  <option key={p.id} value={p.id}>
                    {p.name}
                  </option>
                ))}
              </select>
            </label>
            {!value?.profiles.length && (
              <p>需要合格的本机 Codex 内容生成方案，请先在配置中设置。</p>
            )}
            <p>
              重新解析会开始新一轮规划，已有记录保留。正式编号仅在入图时分配。
            </p>
            <button disabled={parseDisabled} onClick={() => void parse()}>
              重新解析草稿
            </button>
          </details>
          {viewingHistory && (
            <p>
              历史状态：
              {record ? roadmapProgress(record).label : "读取历史记录中"}
            </p>
          )}
          {viewingHistory && !history.some((r) => r.id === historicalID) && (
            <p role="alert">当前草稿中未找到该历史记录，请返回当前流程。</p>
          )}
          {viewingHistory && record?.diagnostic && (
            <p className="notice amber">{record.diagnostic}</p>
          )}
        </aside>
      </div>
      {analysis && (
        <section className="roadmaps-section" id="roadmap-dependencies">
          <h2>任务与依赖</h2>
          <p>
            {viewingHistory
              ? "历史规划快照"
              : record?.state === "admitted"
                ? "入图前规划快照"
                : "局部依赖预览，尚未正式入图"}
            {stale && " · 草稿已修改，以下保留原解析快照"}
          </p>
          {analysis.questions.map((q, i) => (
            <p key={i} className="notice amber">
              {q}
            </p>
          ))}
          {record && (
            <DependencyReview
              key={JSON.stringify([
                api.home,
                identity.project_id,
                identity.instance_id,
                sourceID,
                record.id,
                record.revision,
              ])}
              analysis={analysis}
              existing={record.existing}
              dag={dag}
            />
          )}
          <details className="roadmaps-review-basis">
            <summary>重叠检查与解析依据</summary>
            <p>{analysis.overlap}</p>
            <p>来源：{record?.source.path}</p>
            <p>来源提交：{record?.source.head}</p>
            <p>原文摘要：{record?.source.digest}</p>
          </details>
        </section>
      )}
      <section className="roadmaps-section" id="roadmap-documents">
        <h2>{viewingHistory ? "历史 · " : ""}Spec / Plan 与验收材料</h2>
        {mainAction !== "generate" && <p>{value?.documents_reason}</p>}
        <ul>
          <li>固定仓库与完整提交下的只读代码调查及实际来源。</li>
          <li>消费已确认内部任务和依赖，形成整批设计。</li>
          <li>返回完整逐任务文档和稳定路径，由 SteerLine 审阅发布。</li>
        </ul>
        {record?.code_commit && <p>固定代码提交：{record.code_commit}</p>}
        {record?.design && <p>{record.design}</p>}
        {record?.investigation?.map((ref, index) => (
          <p key={`${ref.source}:${ref.path}:${index}`}>
            调查依据 · {ref.source === "repository" ? "固定仓库" : "本次输入"}：
            {ref.path} · {ref.sha256.slice(0, 12)} · {ref.reason}
          </p>
        ))}
        {record?.document_questions?.map((q, i) => (
          <p className="notice amber" key={i}>
            {q}
          </p>
        ))}
        {!!record?.documents.length && (
          <div>
            <div className="roadmaps-actions">
              {record.documents.map((d) => (
                <button
                  key={d.local_key}
                  onClick={() => {
                    setDocumentKey(d.local_key);
                    setEditing(false);
                  }}
                >
                  {d.local_key}
                </button>
              ))}
              <button
                onClick={() => {
                  setKind("spec");
                  setEditing(false);
                }}
              >
                Spec
              </button>
              <button
                onClick={() => {
                  setKind("plan");
                  setEditing(false);
                }}
              >
                Plan
              </button>
            </div>
            <button
              onClick={() => {
                setKind("tests");
                setEditing(false);
              }}
            >
              测试安排 JSON
            </button>
            <button
              onClick={() => {
                setKind("acceptance");
                setEditing(false);
              }}
            >
              验收材料
            </button>
            <p>{file?.path}</p>
            {editing ? (
              <>
                <textarea
                  aria-label="修改生成正文"
                  value={body}
                  onChange={(e) => setBody(e.target.value)}
                  rows={18}
                />
                <button
                  disabled={locked || !body.trim()}
                  onClick={() => {
                    void action(
                      "roadmap_edit_documents",
                      record.id,
                      record.revision,
                      { local_key: currentDocument!.local_key, kind, body },
                    );
                    setEditing(false);
                  }}
                >
                  保存并重新核对
                </button>
                <button onClick={() => setEditing(false)}>取消</button>
              </>
            ) : (
              <>
                <pre>{file?.body}</pre>
                <button
                  disabled={
                    locked ||
                    record.state !== "documents-review" ||
                    !!record.publication
                  }
                  onClick={() => {
                    setBody(file?.body || "");
                    setEditing(true);
                  }}
                >
                  修改生成正文
                </button>
              </>
            )}
          </div>
        )}
      </section>
      {record && (
        <section className="roadmaps-section">
          <h2>文档发布与正式入 DAG</h2>
          {record.publication_preview && (
            <details>
              <summary>完整发布内容与目标</summary>
              <p>
                {record.publication_preview.repository} ·{" "}
                {record.publication_preview.branch}
              </p>
              {record.publication_preview.files.map((f) => (
                <details key={f.path}>
                  <summary>{f.path}</summary>
                  <p>原内容</p>
                  <pre>{f.before || "新文件"}</pre>
                  <p>发布内容</p>
                  <pre>{f.after}</pre>
                </details>
              ))}
            </details>
          )}
          <p>
            文档使用稳定路径和内部代号；文档合并后仍不分配编号。入图时
            Task、依赖、编号与来源映射整批提交。
          </p>
          <p className="notice amber">
            “发布并合并”会创建文档 PR
            并按已展示内容合并；不强制绕过保护规则。项目若已开启自动派发，新任务入图且满足前置后可能启动。
          </p>
          {record.publication && (
            <p>
              <SafeLink url={record.publication.url}>文档 PR</SafeLink> ·{" "}
              {record.publication.state}
              {record.publication.diagnostic &&
                " · " + record.publication.diagnostic}
            </p>
          )}
          <div className="roadmaps-table">
            <table>
              <thead>
                <tr>
                  <th>Roadmap / 内部代号</th>
                  <th>正式编号</th>
                </tr>
              </thead>
              <tbody>
                {analysis?.tasks.map((t) => (
                  <tr key={t.local_key}>
                    <td>
                      {record.source.path.split("/").at(-1)} / {t.local_key}
                    </td>
                    <td>
                      {record.formal_mapping[t.local_key] ? (
                        <Link to={dag(record.formal_mapping[t.local_key])}>
                          #{record.formal_mapping[t.local_key]}
                        </Link>
                      ) : t.existing_task_id ? (
                        <Link to={dag(t.existing_task_id)}>
                          已有 #{t.existing_task_id}
                        </Link>
                      ) : (
                        "入 DAG 时分配"
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
    </section>
  );
}

export function RoadmapTaskSource({ task }: { task: string }) {
  const { api, identity, online } = useWorkspace();
  const list = useQuery({
    queryKey: roadmapQueryKey(
      api.home,
      identity.project_id,
      identity.instance_id,
    ),
    queryFn: () => api.get<RoadmapView>("/roadmaps"),
    enabled: online,
    staleTime: 30000,
    retry: false,
  });
  const record = list.data?.records?.find((r) =>
    Object.values(r.formal_mapping || {}).includes(task),
  );
  const ref =
    record &&
    Object.entries(record.formal_mapping).find(([, id]) => id === task)?.[0];
  return record ? (
    <p>
      来源：
      <Link
        to={
          "/projects/" +
          identity.project_id +
          "/roadmaps?source=" +
          record.source.id
        }
      >
        {record.source.path.split("/").at(-1)} / {ref}
      </Link>
    </p>
  ) : null;
}
