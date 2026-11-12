import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { useWorkspace } from "../../app/context";
import { SafeLink } from "../../shared/ui/status";
import type { Envelope } from "../../shared/api/types";
import {
  SubmissionStatus,
  useActivityFeedback,
} from "../activities/feedback/feedback";
import {
  contentPending,
  currentRoadmapRecord,
  roadmapProgress,
  roadmapGenerationLabel,
  roadmapNextAction,
  roadmapSourceChanged,
  roadmapSteps,
} from "./workflow";
import "./roadmaps.css";
import { DependencyReview } from "./dependency-review";
import {
  CopyExecutionInfo,
  requestExecutionInfo,
  type ExecutionReference,
} from "../activities/execution-info";
import { OperationResult } from "../activities/operation-result";

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
  provides_tests?: string[];
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
  provides_tests: string[];
  acceptance: File[];
};
export type RoadmapRecord = {
  generations?: {
    generation_id: string;
    local_key: string;
    created_at: string;
    state: string;
    settled?: boolean;
    runner_status?: string;
    resume_required?: boolean;
    diagnostic?: string;
    run_id?: string;
    attempt_id?: string;
    session_id?: string;
    retry_request?: string;
    document?: Documents;
  }[];
  request_id?: string;
  execution_ref?: ExecutionReference;
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
  analysis?: {
    tasks: RoadmapTask[];
    questions: string[];
    overlap: string;
    coordination?: string;
  };
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
  const { api, identity, online, submit, requests, pendingRequests } =
    useWorkspace();
  const feedback = useActivityFeedback();
  const [params, setParams] = useSearchParams();
  const [profile, setProfile] = useState(""),
    [error, setError] = useState("");
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
  const questions = [
    ...(analysis?.questions || []),
    ...(record?.document_questions || []),
  ];
  const belongs = (e: Envelope) =>
    e.project_id === identity.project_id &&
    e.instance_id === identity.instance_id &&
    (e.action === "roadmap_parse"
      ? e.target_id === sourceID
      : history.some((r) => r.id === e.target_id));
  const inFlight = () =>
    (pendingRequests?.() || []).filter(
      (e) =>
        belongs(e) ||
        (e.action === "roadmap_prepare_static" &&
          e.target_id === identity.project_id),
    );
  const advancing = (e: Envelope) =>
    !["roadmap_observe", "roadmap_stop"].includes(e.action);
  const titles: Record<string, string> = {
    roadmap_parse: "正在提交解析…",
    roadmap_generate_documents: "正在提交文档生成…",
    roadmap_retry_documents: "正在提交单项重生成…",
    roadmap_publish: "正在提交文档发布…",
    roadmap_admit: "正在提交入图…",
    roadmap_resume: "正在继续原操作…",
    roadmap_confirm: "正在确认任务与依赖…",
    roadmap_edit_documents: "正在保存文档…",
    roadmap_prepare_static: "正在初始化静态材料…",
    roadmap_observe: "正在查询原操作…",
    roadmap_stop: "正在提交停止请求…",
  };
  const sending = inFlight();
  const originalRows = (requests || []).filter((r) => belongs(r.envelope));
  const unresolved = originalRows.find(
    (r) =>
      advancing(r.envelope) &&
      !["applied", "rejected"].includes(r.response?.status || ""),
  );
  const awaiting = originalRows.some((r) => {
    if (r.response?.status !== "applied" || !advancing(r.envelope))
      return false;
    const received = r.response.roadmap as RoadmapRecord | undefined;
    if (!received?.id) return false;
    const saved =
      received.id === current?.id
        ? current
        : history.find((item) => item.id === received.id);
    return !saved || saved.revision < received.revision;
  });
  const awaitingStatic =
    !!value &&
    (requests || []).some(
      (r) =>
        r.envelope.project_id === identity.project_id &&
        r.envelope.instance_id === identity.instance_id &&
        r.envelope.action === "roadmap_prepare_static" &&
        r.response?.status === "applied" &&
        r.envelope.payload.commit === value?.preparation.commit &&
        !value.preparation.local_ready,
    );
  const flowReason = sending.some(advancing)
    ? titles[sending.find(advancing)!.action]
    : unresolved
      ? "提交结果待核实 · 请查看原请求"
      : awaiting || awaitingStatic
        ? "已受理，正在读取工作信息"
        : "";
  const copyRequest =
    sending.find(advancing) ||
    unresolved?.envelope ||
    [...originalRows].sort((a, b) =>
      b.created_at.localeCompare(a.created_at),
    )[0]?.envelope;
  const copyReceipt = originalRows.find(
    (r) => r.envelope.request_id === copyRequest?.request_id,
  )?.response;
  const actionLabel = (name: string, original: string) =>
    sending.some((e) => e.action === name) ? titles[name] : original;
  const readError =
    list.error || source.error || detail.error || currentDetail.error;
  const locked = !!flowReason || !online || viewingHistory || !!readError;
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
    if (
      !online ||
      viewingHistory ||
      inFlight().some(
        (e) =>
          (advancing(e) && advancing({ action: name } as Envelope)) ||
          (e.action === name && e.target_id === target),
      )
    )
      return;
    setError("");
    try {
      const envelope = api.envelope(name, target, version, payload);
      if (name === "roadmap_parse")
        envelope.payload.request_id = envelope.request_id;
      const operation = submit(envelope);
      if (
        [
          "roadmap_parse",
          "roadmap_generate_documents",
          "roadmap_retry_documents",
          "roadmap_publish",
          "roadmap_admit",
          "roadmap_resume",
        ].includes(name)
      )
        feedback.notify(
          titles[name].replace(/^正在(?:提交)?/, "").replace(/…$/, ""),
          document.activeElement as HTMLElement,
        );
      const result = await operation;
      if (result?.status === "rejected")
        setError(result.diagnostic || "原操作待核实");
      if (!viewingHistory && params.has("record"))
        setParams({ source: sourceID }, { replace: true });
      await Promise.all([
        list.refetch({ cancelRefetch: false }),
        ...(currentID ? [currentDetail.refetch({ cancelRefetch: false })] : []),
      ]);
    } catch (e) {
      setError(String(e));
    }
  }
  const preparationPending = requests?.some(
    (r) =>
      r.envelope.project_id === identity.project_id &&
      r.envelope.instance_id === identity.instance_id &&
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
    !["review", "confirmed"].includes(generationParent.state) ||
    !!generationParent.analysis?.questions.length ||
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
      : mainAction === "generate"
        ? generationParent?.state === "review"
          ? "请核对下方任务范围、原有依赖与拟议依赖；确认后立即生成逐任务 Spec / Plan 和验收材料。"
          : "任务与依赖已确认，可重试生成逐任务 Spec / Plan 和验收材料。"
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
            读取 Markdown → 确认任务依赖并生成 Spec / Plan → 审阅文档 → 发布合并
            → 入 DAG 时分配编号
          </p>
        </div>
        <button
          disabled={!online}
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
      {error && (
        <p className="notice amber" role="alert">
          {error}
        </p>
      )}
      {readError && (
        <p className="notice amber" role="alert">
          {value
            ? "Roadmap 刷新失败，保留已读取的内容。相关操作暂不可提交。"
            : "Roadmap 读取失败。"}
          {String(readError)}
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
            {actionLabel("roadmap_prepare_static", "初始化静态材料")}
          </button>
        </section>
      )}
      <section
        className="roadmaps-progress"
        aria-label="当前草稿转正进度"
        aria-busy={sending.length > 0 || (!!current && contentPending(current))}
      >
        <div className="roadmaps-progress-heading">
          <h2>
            {source.data?.path.split("/").at(-1) || "选择草稿"} · 当前流程
          </h2>
          <strong role="status">
            {flowReason || (list.isPending ? "读取进度中" : progress.label)}
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
        {!viewingHistory && copyRequest && (!record || flowReason) && (
          <CopyExecutionInfo
            key={copyRequest.request_id}
            info={{
              ...requestExecutionInfo(copyRequest, copyReceipt),
              project: identity.repository_identity,
            }}
          />
        )}
        <SubmissionStatus
          announce={false}
          label={
            sending.length && !flowReason
              ? titles[sending[0].action]
              : flowReason
          }
          busy={sending.length > 0}
          showActivity={!!originalRows.length || !!current}
        />
        {!flowReason && !sending.length && current && (
          <button type="button" onClick={feedback.open}>
            查看活动
          </button>
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
                {actionLabel(
                  "roadmap_parse",
                  "Runner generate · 解析任务与依赖",
                )}
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
                {actionLabel(
                  "roadmap_generate_documents",
                  generationParent?.state === "review"
                    ? "确认任务依赖并生成 Spec / Plan"
                    : "重试生成 Spec / Plan",
                )}
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
                disabled={
                  !online ||
                  viewingHistory ||
                  !record ||
                  sending.some((e) => e.action === "roadmap_observe")
                }
                onClick={() =>
                  record && void action("roadmap_observe", record.id, 1)
                }
              >
                {actionLabel("roadmap_observe", "查询原操作结果")}
              </button>
            )}
            {mainAction === "resume" && (
              <button
                className="roadmaps-primary"
                disabled={
                  !online ||
                  viewingHistory ||
                  !record ||
                  sending.some(advancing)
                }
                onClick={() =>
                  record &&
                  void action("roadmap_resume", record.id, record.revision)
                }
              >
                {actionLabel("roadmap_resume", "继续原发布 / 入图请求")}
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
                {actionLabel("roadmap_publish", "确认发布并合并文档")}
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
                {actionLabel("roadmap_admit", "正式入 DAG · 分配编号")}
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
                disabled={
                  !online ||
                  viewingHistory ||
                  sending.some((e) => e.action === "roadmap_observe")
                }
                onClick={() => void action("roadmap_observe", record.id, 1)}
              >
                {actionLabel("roadmap_observe", "查询原操作结果")}
              </button>
            )}
            {record && contentPending(record) && (
              <button
                disabled={
                  !online ||
                  viewingHistory ||
                  sending.some((e) => e.action === "roadmap_stop")
                }
                onClick={() => void action("roadmap_stop", record.id, 1)}
              >
                {actionLabel("roadmap_stop", "停止当前内容请求")}
              </button>
            )}
            {analysis && <a href="#roadmap-dependencies">查看任务与依赖</a>}
            {analysis?.coordination && <p>协作安排：{analysis.coordination}</p>}
            {!!record?.documents?.length && (
              <a href="#roadmap-documents">审阅文档</a>
            )}
          </div>
        )}
      </section>
      {record?.generations?.length ? (
        <section className="roadmaps-section" aria-label="逐任务文档生成">
          <h2>逐任务文档生成</h2>
          <p>
            {new Map(record.generations.map((g) => [g.local_key, g])).size}{" "}
            项任务 · 各自独立生成
          </p>
          {[
            ...new Map(
              record.generations.map((g) => [g.local_key, g]),
            ).values(),
          ].map((g) => (
            <div key={g.generation_id} className="roadmaps-generation">
              <h3>{g.local_key}</h3>
              <p>{roadmapGenerationLabel(g, record.cancelled)}</p>
              {g.diagnostic && <p className="notice amber">{g.diagnostic}</p>}
              <CopyExecutionInfo
                info={{
                  project: identity.repository_identity,
                  title: `Spec / Plan · ${g.local_key}`,
                  generationID: g.generation_id,
                  runnerUnverified: g.state === "unknown",
                  requestID: g.retry_request || record.request_id || record.id,
                  record: { id: record.id, label: "文档批次" },
                  runner: {
                    run_id: g.run_id,
                    attempt_id: g.attempt_id,
                    session_id: g.session_id,
                  },
                }}
              />
              {["failed", "invalid", "rejected"].includes(g.state) && (
                <button
                  disabled={
                    locked ||
                    record.cancelled ||
                    g.settled !== true ||
                    !!record.publication ||
                    !!record.document_questions?.length ||
                    sending.some(advancing)
                  }
                  onClick={() =>
                    void action(
                      "roadmap_retry_documents",
                      record.id,
                      record.revision,
                      { local_key: g.local_key },
                    )
                  }
                >
                  重新生成 {g.local_key}
                </button>
              )}
              {g.document && !record.documents?.length && (
                <details>
                  <summary>预览已生成正文</summary>
                  <h4>Spec</h4>
                  <pre>{g.document.spec.body}</pre>
                  <h4>Plan</h4>
                  <pre>{g.document.plan.body}</pre>
                  <h4>测试选择</h4>
                  <pre>
                    {JSON.stringify({ tests: g.document.tests }, null, 2)}
                  </pre>
                  {g.document.acceptance.map((file) => (
                    <div key={file.path}>
                      <h4>{file.path}</h4>
                      <pre>{file.body}</pre>
                    </div>
                  ))}
                </details>
              )}
            </div>
          ))}
          <details>
            <summary>执行历史（{record.generations.length}）</summary>
            {record.generations.map((g) => (
              <p key={g.generation_id}>
                {g.local_key} · {g.state} · {g.generation_id}
              </p>
            ))}
          </details>
        </section>
      ) : null}
      {record && (
        <section className="roadmaps-section" aria-label="所选记录结果">
          <h2>
            {viewingHistory
              ? "历史记录"
              : record.purpose === "documents"
                ? "本次文档生成"
                : "本次解析"}{" "}
            · {roadmapProgress(record).label}
          </h2>
          {record && (
            <CopyExecutionInfo
              key={record.id}
              info={{
                project: identity.repository_identity,
                title:
                  (viewingHistory ? "历史 · " : "") +
                  (record.purpose === "documents"
                    ? "Spec / Plan 生成"
                    : "Roadmap 解析"),
                requestID:
                  record.execution_ref?.request_id || record.request_id,
                record: { id: record.id, label: "Roadmap 记录" },
                runner:
                  record.purpose === "documents"
                    ? undefined
                    : record.execution_ref || { run_id: record.run_id },
              }}
            />
          )}
          {record?.diagnostic && (
            <p className="notice amber" role="alert">
              {viewingHistory ? "历史记录问题" : "当前流程问题"}：
              {record.diagnostic}
            </p>
          )}
          {!!questions.length && (
            <section aria-label="待澄清问题" className="notice amber">
              <h3>{viewingHistory ? "历史 · " : ""}待澄清问题</h3>
              <ul>
                {questions.map((q, i) => (
                  <li key={i}>{q}</li>
                ))}
              </ul>
            </section>
          )}
        </section>
      )}
      {params.get("request_id") && (
        <OperationResult
          key={params.get("request_id")}
          requestID={params.get("request_id")!}
          targetID={selectedID || sourceID}
          sourceID={sourceID}
        />
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
          {value?.sources.length === 0 && (
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
          </p>
          <textarea
            aria-label="Markdown 原文 · 只读"
            readOnly
            value={source.data?.body || ""}
            rows={16}
          />
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
              {actionLabel("roadmap_parse", "重新解析草稿")}
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
          </details>
        </section>
      )}
      <section className="roadmaps-section" id="roadmap-documents">
        <h2>{viewingHistory ? "历史 · " : ""}Spec / Plan 与验收材料</h2>
        {mainAction !== "generate" && <p>{value?.documents_reason}</p>}
        {(record?.design || !!record?.investigation?.length) && (
          <details>
            <summary>设计与调查依据</summary>
            {record?.design && <p>{record.design}</p>}
            {record?.investigation?.map((ref, index) => (
              <p key={`${ref.source}:${ref.path}:${index}`}>
                调查依据 ·{" "}
                {ref.source === "repository" ? "固定仓库" : "本次输入"}：
                {ref.path} · {ref.reason}
              </p>
            ))}
          </details>
        )}
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
            {!!currentDocument?.provides_tests?.length && (
              <p>
                本任务将新增测试：{currentDocument.provides_tests.join("、")}
                ；依赖后继可在交付后使用。
              </p>
            )}
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
                  {actionLabel("roadmap_edit_documents", "保存并重新核对")}
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
