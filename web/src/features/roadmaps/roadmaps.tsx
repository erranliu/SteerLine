import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { useWorkspace } from "../../app/context";
import { SafeLink } from "../../shared/ui/status";
import "./roadmaps.css";

export type RoadmapSource = {
  id: string;
  path: string;
  body: string;
  digest: string;
  head: string;
  modified: boolean;
  diagnostic?: string;
};
type Dependencies = { local: string[]; formal: string[] };
type Node = {
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
  acceptance: File[];
};
export type RoadmapRecord = {
  id: string;
  purpose?: string;
  parent_id?: string;
  code_commit?: string;
  design?: string;
  investigation?: { path: string; sha256: string; reason: string }[];
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
  diagnostic?: string;
  run_id?: string;
  analysis?: { tasks: Node[]; questions: string[]; overlap: string };
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
};
export type RoadmapView = {
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
const states: Record<string, string> = {
  queued: "内容请求排队中",
  unknown: "原内容请求待核实",
  accepted: "Runner 处理中",
  review: "待确认任务与依赖",
  blocked: "待澄清",
  invalid: "生成结果无效",
  failed: "内容请求失败",
  rejected: "请求被拒绝",
  cancelled: "内容请求已停止",
  confirmed: "已确认 · 内部代号",
  "documents-blocked": "生成发现问题 · 请修改草稿并重新解析",
  "documents-review": "待审阅正式文档",
  publishing: "文档发布中",
  "awaiting-merge": "等待核实合并",
  "awaiting-admission": "文档已合并 · 待入图",
  "admission-unknown": "入图回执待核实",
  admitted: "已正式入图",
};
const dependencies = (d: Dependencies) =>
  [
    ...d.local.map((k) => "本 Roadmap " + k),
    ...d.formal.map((id) => "正式 #" + id),
  ].join("、") || "无";

export function Roadmaps() {
  const { api, identity, online, submit, busy, refresh } = useWorkspace();
  const [params, setParams] = useSearchParams();
  const [profile, setProfile] = useState(""),
    [error, setError] = useState(""),
    [tab, setTab] = useState<"source" | "json">("source");
  const [recordID, setRecordID] = useState(""),
    [documentKey, setDocumentKey] = useState(""),
    [kind, setKind] = useState<"spec" | "plan" | "acceptance">("spec");
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
    sourceID = params.get("source") || value?.sources[0]?.id || "";
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
  const selectedID = history.some((r) => r.id === recordID)
    ? recordID
    : history[0]?.id || "";
  const detail = useQuery({
    queryKey: [...key, "record", selectedID],
    queryFn: () => api.get<RoadmapRecord>("/roadmaps/" + selectedID),
    enabled: online && !!selectedID,
    retry: false,
    refetchInterval: online ? 2000 : false,
  });
  const record = detail.data,
    analysis = record?.analysis;
  const locked = busy || !online;
  const stale =
    !!record && !!source.data && record.source.digest !== source.data.digest;
  const currentDocument =
    record?.documents?.find((d) => d.local_key === documentKey) ||
    record?.documents?.[0];
  const file =
    kind === "acceptance"
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
      const returned = result.roadmap as RoadmapRecord | undefined;
      if (returned?.id) setRecordID(returned.id);
      await list.refetch();
      await detail.refetch();
      refresh();
    } catch (e) {
      setError(String(e));
    }
  }
  const parse = () =>
    source.data &&
    action("roadmap_parse", source.data.id, 1, {
      path: source.data.path,
      source_digest: source.data.digest,
      profile: profile || value?.profiles[0]?.id || "",
    });
  const hasPending = history.some(
    (r) => ["queued", "accepted", "unknown"].includes(r.state) && !r.cancelled,
  );
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
          disabled={locked}
          onClick={() => {
            void list.refetch();
            void source.refetch();
            void detail.refetch();
          }}
        >
          刷新 Markdown 与观察
        </button>
      </header>
      {(error || list.error || source.error || detail.error) && (
        <p className="notice amber" role="alert">
          {error || String(list.error || source.error || detail.error)}
        </p>
      )}
      {!online && (
        <p className="notice amber">
          连接待核实 · 以下为最后观察，暂不能提交操作。
        </p>
      )}
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
                setRecordID("");
                setEditing(false);
                setError("");
              }}
            >
              <strong>{s.path.split("/").at(-1)}</strong>
              <span>{s.path}</span>
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
          <h2>解析与确认</h2>
          <label>
            内容生成方案
            <select
              value={profile || value?.profiles[0]?.id || ""}
              onChange={(e) => setProfile(e.target.value)}
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
          <button
            disabled={
              locked ||
              !source.data ||
              !!source.data.diagnostic ||
              hasPending ||
              !value?.profiles.length
            }
            onClick={() => void parse()}
          >
            Runner generate · 解析任务与依赖
          </button>
          <p>
            保留草稿已有代号与依赖，并与已有正式 DAG
            核对。正式编号仅在入图时分配。
          </p>
          {history.length > 0 && (
            <label>
              解析历史
              <select
                value={selectedID}
                onChange={(e) => {
                  setRecordID(e.target.value);
                  setEditing(false);
                }}
              >
                {history.map((r) => (
                  <option key={r.id} value={r.id}>
                    {r.purpose === "documents" ? "文档生成 · " : "任务解析 · "}
                    {new Date(r.created_at).toLocaleString()} ·{" "}
                    {states[r.state] || r.state}
                  </option>
                ))}
              </select>
            </label>
          )}
          {record?.purpose === "documents" && <p>文档生成请求</p>}
          <p role="status">
            {record ? states[record.state] || record.state : "尚未解析"}
          </p>
          {record?.diagnostic && (
            <p className="notice amber">{record.diagnostic}</p>
          )}
          {record && (
            <div className="roadmaps-actions">
              <button
                disabled={
                  locked ||
                  record.state !== "review" ||
                  stale ||
                  !!analysis?.questions.length
                }
                onClick={() =>
                  void action("roadmap_confirm", record.id, record.revision)
                }
              >
                确认任务与依赖
              </button>
              <button
                disabled={locked}
                onClick={() => void action("roadmap_observe", record.id, 1)}
              >
                查询原操作结果
              </button>
              <button
                disabled={
                  locked ||
                  record.cancelled ||
                  !["queued", "accepted", "unknown"].includes(record.state)
                }
                onClick={() => void action("roadmap_stop", record.id, 1)}
              >
                停止当前内容请求
              </button>
            </div>
          )}
          {record?.confirmed_digest && (
            <p>已确认内部代号与依赖，新任务尚未因确认而获得正式编号。</p>
          )}
        </aside>
      </div>
      {analysis && (
        <section className="roadmaps-section">
          <h2>草稿原有依赖与 Runner 建议</h2>
          <p>{analysis.overlap}</p>
          {analysis.questions.map((q, i) => (
            <p key={i} className="notice amber">
              {q}
            </p>
          ))}
          <div className="roadmaps-table">
            <table>
              <thead>
                <tr>
                  <th>内部代号 / 任务</th>
                  <th>草稿原有依赖</th>
                  <th>核对后的拟议依赖</th>
                  <th>变化依据</th>
                </tr>
              </thead>
              <tbody>
                {analysis.tasks.map((t) => (
                  <tr key={t.local_key}>
                    <td>
                      <strong>
                        {t.local_key} · {t.title}
                      </strong>
                      <p>
                        {t.key_origin === "markdown"
                          ? "沿用原代号"
                          : "建议临时代号"}
                      </p>
                      <p>{t.goal}</p>
                      <details>
                        <summary>范围、约束与原文依据</summary>
                        <p>{t.scope}</p>
                        <p>{t.constraints}</p>
                        <p>通过条件：{t.acceptance}</p>
                        <blockquote>{t.evidence}</blockquote>
                      </details>
                    </td>
                    <td>{dependencies(t.original_dependencies)}</td>
                    <td>
                      {dependencies(t.proposed_dependencies)}
                      {t.existing_task_id && (
                        <p>
                          <Link to={dag(t.existing_task_id)}>
                            关联已有 #{t.existing_task_id}
                          </Link>
                        </p>
                      )}
                    </td>
                    <td>{t.reason}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}
      <section className="roadmaps-section">
        <h2>Spec / Plan 与 Runner 能力</h2>
        <p>{value?.documents_reason}</p>
        <ul>
          <li>固定仓库与完整提交下的只读代码调查及实际来源。</li>
          <li>消费已确认内部任务和依赖，形成整批设计。</li>
          <li>返回完整逐任务文档和稳定路径，由 SteerLine 审阅发布。</li>
        </ul>
        <button
          disabled={
            locked ||
            !generationParent ||
            hasPending ||
            generationParent.state !== "confirmed" ||
            !value?.documents_available
          }
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
        {record?.code_commit && <p>固定代码提交：{record.code_commit}</p>}
        {record?.design && <p>{record.design}</p>}
        {record?.investigation?.map((ref) => (
          <p key={ref.path}>
            调查依据：{ref.path} · {ref.sha256.slice(0, 12)} · {ref.reason}
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
          <div className="roadmaps-actions">
            <button
              disabled={
                locked || record.state !== "documents-review" || editing
              }
              onClick={() =>
                void action("roadmap_publish", record.id, record.revision, {
                  review_digest:
                    (record as RoadmapRecord & { review_digest?: string })
                      .review_digest || "",
                  merge: true,
                })
              }
            >
              确认发布并合并文档
            </button>
            <button
              disabled={locked || record.state !== "awaiting-admission"}
              onClick={() =>
                void action("roadmap_admit", record.id, record.revision)
              }
            >
              正式入 DAG · 分配编号
            </button>
          </div>
          {(record.publication || record.state === "admission-unknown") &&
            record.state !== "admitted" && (
              <button
                disabled={locked || record.state === "awaiting-admission"}
                onClick={() =>
                  void action("roadmap_resume", record.id, record.revision)
                }
              >
                继续原发布 / 入图请求
              </button>
            )}
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
