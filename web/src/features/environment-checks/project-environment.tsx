import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { useWorkspace } from "../../app/context";
import { usePageVisible } from "../../shared/query-visibility";
import { Button } from "../../shared/ui/button";
import { useExecutionEnvironments } from "../../shared/api/execution-environments";
import {
  EnvironmentObservation,
  useEnvironmentReadiness,
  type Environment,
} from "./use-readiness";
import { DetectionID, EnvironmentCard } from "./environment-card";
import { EnvironmentOverview } from "./environment-overview";
import {
  CopyExecutionInfo,
  requestExecutionInfo,
  type ExecutionReference,
} from "../activities/execution-info";
import { OperationResult } from "../activities/operation-result";
import {
  SubmissionStatus,
  useActivityFeedback,
} from "../activities/feedback/feedback";

type Record = {
  execution_ref?: ExecutionReference;
  id: string;
  revision: number;
  profile_id: string;
  environment_key: string;
  created_at?: string;
  state: string;
  status: string;
  settled: boolean;
  blocked?: boolean;
  timed_out?: boolean;
  diagnostic: string;
};
const recordState: { [key: string]: string } = {
  ready: "已通过",
  failed: "未通过",
  cancelled: "已停止",
  running: "检测中",
  queued: "等待检测",
  stopping: "正在停止",
  unverified: "结果待核实",
};
const recordLabel = (record: Record) =>
  record.timed_out
    ? "检测超时"
    : record.blocked
      ? "检测阻断"
      : recordState[record.state] || "结果待核实";
const linkName = (e: Environment) =>
  e.adapter === "codex-app-server"
    ? "Codex 直连"
    : `Orca → ${e.agent || "Codex"}`;

type Selection = { key: string; profile?: string };

function readSelection(key: string): Selection | null {
  try {
    const saved: Selection | null = JSON.parse(
      sessionStorage.getItem(key) ?? "null",
    );
    return saved &&
      typeof saved.key === "string" &&
      (saved.profile === undefined || typeof saved.profile === "string")
      ? saved
      : null;
  } catch {
    return null;
  }
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
  const { api, identity, online, send, submit, requests, pendingRequests } =
    useWorkspace();
  const feedback = useActivityFeedback();
  const visible = usePageVisible();
  const scope = [
    api.home,
    identity.project_id,
    identity.instance_id,
    "environment",
  ];
  const options = useEnvironmentReadiness();
  const [params] = useSearchParams();
  const requestedActivity = params.get("activity") || "";
  const requestedRecord = useQuery({
    queryKey: [...scope, "requested-record", requestedActivity],
    queryFn: ({ signal }) =>
      api.get<Record>(
        "/environment/" + encodeURIComponent(requestedActivity),
        signal,
      ),
    enabled: online && !!requestedActivity,
    retry: false,
    refetchInterval: visible && !!requestedActivity ? 3000 : false,
  });
  useEffect(() => {
    if (requestedRecord.data?.id === requestedActivity)
      document.getElementById("requested-environment-activity")?.focus();
  }, [requestedActivity, requestedRecord.data?.id]);
  const profile = params.get("profile") || "";
  const task = params.get("task");
  const preferenceKey =
    "steerline-environment:" + JSON.stringify([api.home, identity.project_id]);
  const [saved] = useState(() => readSelection(preferenceKey));
  const [selected, setSelected] = useState<Selection | null>(null);
  const focusProfile = options.data?.profiles.find((p) => p.id === profile);
  const defaultProfile = options.data?.profiles.find(
    (p) => p.id === options.data?.default_profile_id,
  );
  const savedEnvironment = options.data?.environments.find(
    (e) => e.environment_key === saved?.key,
  );
  const initialProfile = profile ? focusProfile : defaultProfile;
  const selection =
    selected ??
    (!profile && savedEnvironment
      ? saved
      : initialProfile && {
          key: initialProfile.environment_key,
          profile: profile || undefined,
        });
  const environmentKey = selection?.key ?? "";
  const remember = (next: Selection) => {
    setSelected(next);
    try {
      sessionStorage.setItem(preferenceKey, JSON.stringify(next));
    } catch {
      // The selection still works on this page when browser storage is unavailable.
    }
  };
  const hasRemote =
    options.data?.environments.some(
      (e) => e.target.kind === "orca_environment",
    ) ?? false;
  const machines = useExecutionEnvironments(api.home, api.csrf, hasRemote);
  const machineName = (e: Environment) =>
    e.target.kind === "local"
      ? "本机"
      : machines.data?.environments?.find(
          (m) =>
            m.id === e.target.environment || m.name === e.target.environment,
        )?.name || "远端机器";
  const environmentName = (e: Environment) => {
    const name = `${machineName(e)} · ${linkName(e)}`;
    const peers =
      options.data?.environments.filter(
        (other) => `${machineName(other)} · ${linkName(other)}` === name,
      ) ?? [];
    return peers.length > 1
      ? `${name} · ${e.profiles.map((p) => p.name).join("、")}`
      : name;
  };
  const [allActivities, setAllActivities] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const environment = options.data?.environments.find(
    (e) => e.environment_key === environmentKey,
  );
  const activityID = environment?.activity_id;
  const activity = useQuery({
    queryKey: [...scope, "record", activityID],
    queryFn: ({ signal }) =>
      api.get<Record>("/environment/" + activityID, signal),
    enabled: !!activityID,
    retry: false,
    refetchInterval: visible && !!activityID ? 3000 : false,
  });
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
    enabled: historyOpen && (allActivities || !!environmentKey),
    retry: false,
    refetchInterval: visible && historyOpen ? 3000 : false,
  });
  const [materialsOpen, setMaterialsOpen] = useState(false);
  const materialQuery = useQuery({
    queryKey: [...scope, "materials", environmentKey],
    enabled: online && !!environmentKey && materialsOpen,
    queryFn: ({ signal }) =>
      api.get<{ document?: string; diagnostic?: string }>(
        `/environment/materials?environment_key=${environmentKey}`,
        signal,
      ),
    retry: false,
  });
  const pending = useRef(false);
  const [submitting, setSubmitting] = useState(false);
  const startRowsFor = (key: string) =>
    (requests || []).filter(
      ({ envelope }) =>
        envelope.project_id === identity.project_id &&
        envelope.instance_id === identity.instance_id &&
        envelope.action === "environment_start" &&
        envelope.payload.environment_key === key,
    );
  const startRows = startRowsFor(environmentKey);
  const latestStart = [...startRows].sort((a, b) =>
    b.created_at.localeCompare(a.created_at),
  )[0];
  const startSending = (key = environmentKey) =>
    (pendingRequests?.() || []).some(
      (e) =>
        e.action === "environment_start" && e.payload.environment_key === key,
    );
  const copyStart =
    (pendingRequests?.() || []).find(
      (e) =>
        e.action === "environment_start" &&
        e.payload.environment_key === environmentKey,
    ) || latestStart?.envelope;
  const copyStartReceipt = startRows.find(
    (r) => r.envelope.request_id === copyStart?.request_id,
  )?.response;
  const confirmingEnvironment = (current: Environment) => {
    const rows = startRowsFor(current.environment_key);
    const latest = [...rows].sort((a, b) =>
      b.created_at.localeCompare(a.created_at),
    )[0];
    const receivedID = (
      latest?.response?.environment as { id?: string } | undefined
    )?.id;
    return (
      startSending(current.environment_key) ||
      (!!receivedID &&
        receivedID !== current.activity_id &&
        current.current_revision <=
          Number(latest?.envelope.payload.expected_current_revision)) ||
      rows.some(
        ({ response }) =>
          response?.status !== "applied" && response?.status !== "rejected",
      )
    );
  };
  const confirmingStart = !!environment && confirmingEnvironment(environment);
  const refresh = async (onOptionsRefreshed?: () => void) => {
    const queries: Promise<unknown>[] = [
      options.refetch().finally(onOptionsRefreshed),
    ];
    if (hasRemote) queries.push(machines.refetch());
    if (historyOpen && (allActivities || environmentKey))
      queries.push(history.refetch());
    if (materialsOpen && environmentKey) queries.push(materialQuery.refetch());
    if (activityID) queries.push(activity.refetch());
    await Promise.allSettled(queries);
  };
  const command = async (
    action: string,
    target: string,
    version: number,
    payload: { [key: string]: unknown },
  ) => {
    if (!online) return;
    if (action === "environment_start") {
      if (startSending() || confirmingStart) return;
      if (typeof payload.profile_id === "string")
        remember({ key: environmentKey, profile: payload.profile_id });
      const operation = submit(api.envelope(action, target, version, payload));
      feedback.notify("环境检测", document.activeElement as HTMLElement);
      await operation;
      await refresh();
      return;
    }
    if (pending.current) return;
    pending.current = true;
    setSubmitting(true);
    try {
      await send(action, target, version, payload);
      await refresh();
    } finally {
      pending.current = false;
      setSubmitting(false);
    }
  };
  useEffect(() => {
    if (
      !profile ||
      !focusProfile ||
      environmentKey !== focusProfile.environment_key
    )
      return;
    const node = document.getElementById(`environment-${environmentKey}`);
    node?.scrollIntoView?.({ block: "nearest" });
    node?.focus();
  }, [focusProfile?.environment_key, profile, environmentKey]);
  const chooseEnvironment = (key: string) => {
    remember({ key });
    setAllActivities(false);
    setMaterialsOpen(false);
  };
  const returnParams = new URLSearchParams();
  if (task) returnParams.set("task", task);
  if (profile) returnParams.set("start_profile", profile);
  const material = materialQuery.data;
  return (
    <div className="settings-page environment-page">
      <div className="environment-header">
        <h1>项目运行环境</h1>
        <Button variant="ghost" onClick={() => void refresh()}>
          刷新环境
        </Button>
      </div>
      <p className="muted">查看所有环境的就绪情况，展开详情进行检测和准备。</p>
      {task && (
        <Link to={`/projects/${identity.project_id}/runs?${returnParams}`}>
          返回任务
        </Link>
      )}
      <EnvironmentObservation />
      {!options.required && (
        <p className="notice amber">
          环境就绪阻塞已关闭；检测结果仍按实际状态展示。
        </p>
      )}
      {options.verified && !!options.dataUpdatedAt && (
        <p className="muted">
          最近状态更新：
          {new Date(options.dataUpdatedAt).toLocaleString("zh-CN")}
        </p>
      )}
      {options.error && <p role="alert">环境状态查询失败，请刷新重试。</p>}
      {profile && options.verified && !focusProfile && (
        <p role="alert">原执行方案已不存在或不属于开发方案，请重新选择。</p>
      )}
      {!!options.data?.environments.length && (
        <EnvironmentOverview
          environments={options.data.environments}
          verified={options.verified}
          selected={environmentKey}
          name={environmentName}
          confirming={confirmingEnvironment}
          onSelect={chooseEnvironment}
        />
      )}
      {!options.data && !options.error && (
        <p className="muted">正在读取运行环境…</p>
      )}
      {environmentKey && !environment && options.verified && (
        <p role="alert">原环境已不在当前配置中，请重新选择。</p>
      )}
      {options.verified && !options.data?.environments.length && (
        <p>
          尚无开发执行方案。
          <Link to="/settings/execution-configs">配置执行方案</Link>
        </p>
      )}
      {requestedActivity && (
        <section
          id="requested-environment-activity"
          tabIndex={-1}
          aria-label="指定检测详情"
        >
          <h2>指定检测结果</h2>
          {requestedRecord.data?.id === requestedActivity ? (
            <>
              <DetectionID
                id={requestedActivity}
                execution={requestedRecord.data.execution_ref}
              />
              <p>
                {recordLabel(requestedRecord.data)} ·{" "}
                {requestedRecord.data.profile_id}
              </p>
              <p>{requestedRecord.data.diagnostic}</p>
              {params.get("request_id") && (
                <OperationResult
                  requestID={params.get("request_id")!}
                  targetID={requestedActivity}
                />
              )}
            </>
          ) : (
            <p>
              {requestedRecord.isError
                ? "无法读取指定检测，请核对原检测 ID"
                : "正在读取指定检测…"}
            </p>
          )}
        </section>
      )}
      {environment && (
        <section
          aria-label="当前运行环境"
          id="environment-detail"
          className="environment-detail"
        >
          <h2>{environmentName(environment)}</h2>
          <SubmissionStatus
            label={
              startSending()
                ? "正在提交检测…"
                : confirmingStart
                  ? latestStart?.response?.status === "applied"
                    ? "已受理，正在读取工作信息"
                    : "提交结果待核实 · 请查看原请求"
                  : environment.activity_id
                    ? "检测进度可在活动中心查看"
                    : ""
            }
            busy={startSending()}
            showActivity={!!latestStart || !!environment.activity_id}
          />
          <EnvironmentCard
            key={`${environmentKey}:${profile}`}
            environment={environment}
            project={identity.project_id}
            defaultProfile={options.data!.default_profile_id}
            preferred={selection?.profile}
            onProfileChange={(profile) =>
              remember({ key: environmentKey, profile })
            }
            confirmingStart={!!confirmingStart}
            submittingStart={startSending()}
            statusVerified={options.verified && !activity.isError}
            disabled={!options.verified || submitting}
            stopDisabled={!online || activity.isError || submitting}
            activity={activity.data}
            command={command}
          />
          {confirmingStart && copyStart && (
            <CopyExecutionInfo
              info={{
                ...requestExecutionInfo(copyStart, copyStartReceipt),
                project: identity.repository_identity,
              }}
            />
          )}
          {activity.error && (
            <p role="status">检测状态查询失败，请刷新重试。</p>
          )}
          <details
            open={materialsOpen}
            onToggle={(event) => setMaterialsOpen(event.currentTarget.open)}
          >
            <summary>查看准备要求</summary>
            {materialQuery.isError ? (
              <p role="alert">准备要求读取失败，请刷新重试。</p>
            ) : materialQuery.isFetching ? (
              <p>正在读取准备要求…</p>
            ) : (
              <pre>
                {material?.document
                  ?.replace(/```steerline-environment\s*\n[\s\S]*?\n```/g, "")
                  .trim() ||
                  "项目尚未提供准备要求，请联系项目维护者完善检测入口。"}
              </pre>
            )}
          </details>
        </section>
      )}
      <details
        open={historyOpen}
        onToggle={(event) => setHistoryOpen(event.currentTarget.open)}
      >
        <summary>检测历史</summary>
        <label>
          查看范围
          <select
            aria-label="历史范围"
            value={allActivities ? "project" : "environment"}
            onChange={(event) =>
              setAllActivities(event.target.value === "project")
            }
          >
            <option value="environment">当前环境</option>
            <option value="project">当前项目全部环境</option>
          </select>
        </label>
        {history.error && <p role="alert">检测历史读取失败，请刷新重试。</p>}
        {history.isFetching && !history.data && <p>正在读取检测历史…</p>}
        {history.data?.records.length === 0 && (
          <p className="muted">暂无检测记录</p>
        )}
        {history.data?.records.map((r) => {
          const matched = options.data?.environments.find(
            (e) => e.environment_key === r.environment_key,
          );
          return (
            <article className="environment-history-item" key={r.id}>
              <p>
                <strong>{recordLabel(r)}</strong>
                {r.created_at && (
                  <> · {new Date(r.created_at).toLocaleString("zh-CN")}</>
                )}
                {allActivities && (
                  <> · {matched ? environmentName(matched) : "历史环境"}</>
                )}
              </p>
              {r.diagnostic && r.status !== "ready" && (
                <p className="environment-reason">{r.diagnostic}</p>
              )}
              <DetectionID id={r.id} execution={r.execution_ref} />
              <Link to={`?activity=${encodeURIComponent(r.id)}`}>查看详情</Link>
              {!r.settled && (
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={!online || submitting}
                  onClick={() =>
                    void command("environment_stop", r.id, r.revision, {})
                  }
                >
                  停止检测
                </Button>
              )}
            </article>
          );
        })}
      </details>
    </div>
  );
}
