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

type Record = {
  id: string;
  revision: number;
  profile_id: string;
  environment_key: string;
  created_at?: string;
  state: string;
  status: string;
  settled: boolean;
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
const linkName = (e: Environment) =>
  e.adapter === "codex-app-server"
    ? "Codex 直连"
    : `Orca → ${e.agent || "Codex"}`;

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
  const [environmentKey, setEnvironmentKey] = useState("");
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
  const [params] = useSearchParams();
  const profile = params.get("profile") || "";
  const task = params.get("task");
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
  const refresh = async () => {
    const queries: Promise<unknown>[] = [options.refetch()];
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
  const focusProfile = options.data?.profiles.find((p) => p.id === profile);
  const initialized = useRef(false);
  useEffect(() => {
    if (initialized.current || !options.data) return;
    initialized.current = true;
    const initial = profile
      ? focusProfile
      : options.data.profiles.find(
          (p) => p.id === options.data?.default_profile_id,
        );
    if (initial) setEnvironmentKey(initial.environment_key);
  }, [options.data, profile, focusProfile]);
  useEffect(() => {
    const node = document.getElementById(
      `environment-${focusProfile?.environment_key || profile}`,
    );
    node?.scrollIntoView?.({ block: "nearest" });
    node?.focus();
  }, [focusProfile?.environment_key, profile, environmentKey]);
  const chooseEnvironment = (key: string) => {
    setEnvironmentKey(key);
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
        <Button variant="ghost" onClick={refresh}>
          刷新环境
        </Button>
      </div>
      {task && (
        <Link to={`/projects/${identity.project_id}/runs?${returnParams}`}>
          返回任务
        </Link>
      )}
      <EnvironmentObservation />
      {options.error && <p role="alert">环境状态查询失败，请刷新重试。</p>}
      {profile && options.verified && !focusProfile && (
        <p role="alert">原执行方案已不存在或不属于开发方案，请重新选择。</p>
      )}
      <section aria-label="选择运行机与链路">
        <label>
          运行环境
          <select
            aria-label="运行环境"
            value={environmentKey}
            onChange={(event) => chooseEnvironment(event.target.value)}
          >
            <option value="">选择运行环境</option>
            {options.data?.environments.map((e) => (
              <option
                key={e.environment_key || e.profiles[0]?.id}
                value={e.environment_key}
                disabled={!e.environment_key}
              >
                {environmentName(e)}
              </option>
            ))}
          </select>
        </label>
      </section>
      {environmentKey && !environment && options.verified && (
        <p role="alert">原环境已不在当前配置中，请重新选择。</p>
      )}
      {!environment && <p className="muted">选择运行环境后即可开始检测。</p>}
      {options.verified && !options.data?.environments.length && (
        <p>
          尚无开发执行方案。
          <Link to="/settings/execution-configs">配置执行方案</Link>
        </p>
      )}
      {environment && (
        <section aria-label="当前运行环境">
          <EnvironmentCard
            key={`${environmentKey}:${profile}`}
            environment={environment}
            project={identity.project_id}
            defaultProfile={options.data!.default_profile_id}
            preferred={profile}
            disabled={!options.verified || busy || submitting}
            stopDisabled={!online || busy || submitting}
            activity={activity.data}
            command={command}
          />
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
                <strong>{recordState[r.state] || "结果待核实"}</strong>
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
              <DetectionID id={r.id} />
              {!r.settled && (
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={!online || busy || submitting}
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
