import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
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
import {
  DetectionID,
  recordLabel,
  type DetectionRecord,
} from "./detection-record";
import {
  EnvironmentOverview,
  type DetectionSubmission,
} from "./environment-overview";
import { OperationResult } from "../activities/operation-result";
import { useActivityFeedback } from "../activities/feedback/feedback";

const linkName = (e: Environment) =>
  e.adapter === "codex-app-server"
    ? "Codex 直连"
    : `Orca → ${e.agent || "Codex"}`;

function readPreferences(key: string): Record<string, string> {
  try {
    return JSON.parse(sessionStorage.getItem(key) ?? "null") ?? {};
  } catch {
    return {};
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
  const queryClient = useQueryClient();
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
      api.get<DetectionRecord>(
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
  const [saved] = useState(() => readPreferences(preferenceKey));
  const [preferences, setPreferences] = useState<Record<string, string>>({});
  const focusProfile = options.data?.profiles.find((p) => p.id === profile);
  const linkedEnvironment = focusProfile?.environment_key;
  const preferred = (key: string) =>
    preferences[key] ?? (key === linkedEnvironment ? profile : saved[key]);
  const remember = (key: string, profileID: string) => {
    setPreferences({ ...preferences, [key]: profileID });
    try {
      sessionStorage.setItem(
        preferenceKey,
        JSON.stringify({ ...saved, ...preferences, [key]: profileID }),
      );
    } catch {
      // The selection remains available on the current page.
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
  const [historyFilter, setHistoryFilter] = useState("");
  const [historyOpen, setHistoryOpen] = useState(false);
  const environments = options.data?.environments ?? [];
  const historyKey = environments.some(
    (e) => e.environment_key === historyFilter,
  )
    ? historyFilter
    : "";
  const history = useQuery({
    queryKey: [...scope, "records", historyKey],
    queryFn: ({ signal }) =>
      api.get<{ records: DetectionRecord[] }>(
        historyKey
          ? `/environment?environment_key=${encodeURIComponent(historyKey)}`
          : "/environment",
        signal,
      ),
    enabled: historyOpen,
    retry: false,
    refetchInterval: visible && historyOpen ? 3000 : false,
  });
  // The materials endpoint requires a configured environment key. The document
  // comes from the project's observed source and is shared across environments.
  const materialKey = environments.find(
    (e) => e.environment_key,
  )?.environment_key;
  const [copyingPrompt, setCopyingPrompt] = useState(false);
  const [promptFeedback, setPromptFeedback] = useState<{
    text: string;
    failed: boolean;
  }>();
  const copyRepairPrompt = async () => {
    const prompt = options.repairPrompt;
    if (!prompt || copyingPrompt) return;
    setCopyingPrompt(true);
    setPromptFeedback(undefined);
    try {
      await navigator.clipboard.writeText(prompt);
      setPromptFeedback({ text: "已复制修正环境提示词", failed: false });
    } catch {
      setPromptFeedback({ text: prompt, failed: true });
    } finally {
      setCopyingPrompt(false);
    }
  };
  const materialQuery = useQuery({
    queryKey: [...scope, "materials", materialKey],
    enabled: online && !!materialKey,
    queryFn: ({ signal }) =>
      api.get<{ document?: string; diagnostic?: string }>(
        `/environment/materials?environment_key=${encodeURIComponent(materialKey!)}`,
        signal,
      ),
    retry: false,
  });
  const pending = useRef(false);
  const [submitting, setSubmitting] = useState(false);
  const startState = (current: Environment): DetectionSubmission => {
    const rows = (requests || []).filter(
      ({ envelope }) =>
        envelope.project_id === identity.project_id &&
        envelope.instance_id === identity.instance_id &&
        envelope.action === "environment_start" &&
        envelope.payload.environment_key === current.environment_key,
    );
    const latest = rows.sort((a, b) =>
      b.created_at.localeCompare(a.created_at),
    )[0];
    const sending = (pendingRequests?.() || []).find(
      (e) =>
        e.action === "environment_start" &&
        e.payload.environment_key === current.environment_key,
    );
    const receivedID = (
      latest?.response?.environment as { id?: string } | undefined
    )?.id;
    const confirming =
      !!sending ||
      (!!receivedID &&
        receivedID !== current.activity_id &&
        current.current_revision <=
          Number(latest?.envelope.payload.expected_current_revision)) ||
      rows.some(
        ({ response }) =>
          response?.status !== "applied" && response?.status !== "rejected",
      );
    return {
      sending: !!sending,
      confirming,
      accepted: latest?.response?.status === "applied",
      request: sending || latest?.envelope,
      receipt:
        rows.find((r) => r.envelope.request_id === sending?.request_id)
          ?.response ?? (sending ? undefined : latest?.response),
    };
  };
  const refresh = async () => {
    const queries: Promise<unknown>[] = [
      options.refetch(),
      queryClient.refetchQueries({
        queryKey: [...scope, "record"],
        type: "active",
      }),
    ];
    if (hasRemote) queries.push(machines.refetch());
    if (historyOpen) queries.push(history.refetch());
    if (online && materialKey) queries.push(materialQuery.refetch());
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
      const current = environments.find(
        (e) => e.environment_key === payload.environment_key,
      );
      if (!current || startState(current).confirming) return;
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
    if (!linkedEnvironment || requestedActivity) return;
    const node = document.getElementById(`environment-${linkedEnvironment}`);
    node?.scrollIntoView?.({ block: "nearest" });
    node?.focus();
  }, [linkedEnvironment, profile, requestedActivity]);
  const returnParams = new URLSearchParams();
  if (task) returnParams.set("task", task);
  if (profile) returnParams.set("start_profile", profile);
  const material = materialQuery.data;
  return (
    <div className="settings-page environment-page">
      <div className="environment-header">
        <h1>项目运行环境</h1>
        <div className="actions">
          <Button
            variant="ghost"
            disabled={!options.repairPrompt || copyingPrompt}
            onClick={() => void copyRepairPrompt()}
          >
            复制修正环境提示词
          </Button>
          <Button variant="ghost" onClick={() => void refresh()}>
            刷新环境
          </Button>
        </div>
      </div>
      {promptFeedback &&
        (promptFeedback.failed ? (
          <div role="alert">
            复制失败，请手动复制：<pre>{promptFeedback.text}</pre>
          </div>
        ) : (
          <p role="status">{promptFeedback.text}</p>
        ))}
      <p className="muted">查看所有环境的就绪情况，并在表格中发起检测。</p>
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
      {options.data && environments.length > 0 && (
        <EnvironmentOverview
          environments={environments}
          verified={options.verified}
          defaultProfile={options.data.default_profile_id}
          linkedEnvironment={linkedEnvironment}
          name={environmentName}
          preferred={preferred}
          onProfileChange={remember}
          startState={startState}
          submitting={submitting}
          recordScope={[...scope, "record"]}
          command={command}
        />
      )}
      {!options.data && !options.error && (
        <p className="muted">正在读取运行环境…</p>
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
      {materialKey && (
        <section
          className="environment-materials"
          aria-labelledby="environment-materials-title"
        >
          <h2 id="environment-materials-title">准备要求</h2>
          {materialQuery.isError ? (
            <p role="alert">准备要求读取失败，请刷新重试。</p>
          ) : materialQuery.isPending && online ? (
            <p>正在读取准备要求…</p>
          ) : materialQuery.isPending ? (
            <p>连接项目后可读取准备要求。</p>
          ) : (
            <pre>
              {material?.document
                ?.replace(/```steerline-environment\s*\n[\s\S]*?\n```/g, "")
                .trim() ||
                material?.diagnostic ||
                "项目尚未提供准备要求，请联系项目维护者完善检测入口。"}
            </pre>
          )}
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
            value={historyKey}
            onChange={(event) => setHistoryFilter(event.target.value)}
          >
            <option value="">当前项目全部环境</option>
            {environments
              .filter((e) => e.environment_key)
              .map((e) => (
                <option key={e.environment_key} value={e.environment_key}>
                  {environmentName(e)}
                </option>
              ))}
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
                {!historyKey && (
                  <> · {matched ? environmentName(matched) : "历史环境"}</>
                )}
              </p>
              {r.diagnostic && r.status !== "ready" && (
                <p className="environment-reason">{r.diagnostic}</p>
              )}
              <DetectionID id={r.id} execution={r.execution_ref} />
              <Link to={`?activity=${encodeURIComponent(r.id)}`}>
                查看检测结果
              </Link>
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
