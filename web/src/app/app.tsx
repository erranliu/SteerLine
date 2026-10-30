import { lazy, Suspense, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import {
  NavLink,
  Routes,
  Route,
  Navigate,
  useLocation,
} from "react-router-dom";
import { RefreshCw } from "lucide-react";
import { AppShell } from "./app-shell";
import { json } from "../shared/api/client";
import { ApiError } from "../shared/api/client";
import type { Identity, BrowserSession } from "../shared/api/types";
import { WorkspaceContext } from "./context";
import { Button } from "../shared/ui/button";
import { ProjectMaintenance } from "../features/project-maintenance/project-maintenance";
import { Dispatch } from "../features/tasks/dispatch";
import { requiresAnswer } from "../features/runs/state";
import { short } from "../shared/ui/status";
import { SettingsPage } from "../features/settings/settings";
import { Operations } from "./operations";
import { ExecutionConfigs } from "../features/execution-configs/execution-configs";
import { EnvironmentSummary } from "../features/environment-checks/use-readiness";
import { ProjectEnvironment } from "../features/environment-checks/project-environment";
import { useApplicationProjects } from "./use-application-projects";
import { useWorkspaceConnection } from "./use-workspace-connection";
import { useOriginalRequests } from "./use-original-requests";
import { useCandidateStarts } from "./use-candidate-starts";
const Roadmaps = lazy(() =>
  import("../features/roadmaps/roadmaps").then((module) => ({
    default: module.Roadmaps,
  })),
);
const Tests = lazy(() =>
  import("../features/tests/tests").then((module) => ({
    default: module.Tests,
  })),
);
const Dag = lazy(() =>
  import("../features/tasks/dag").then((module) => ({ default: module.Dag })),
);
const Runs = lazy(() =>
  import("../features/runs/runs").then((module) => ({ default: module.Runs })),
);

export function Workspace({
  initial,
  connected: applicationConnected = true,
  home = "",
  switcher,
  configurationPending = false,
  loadedDigest,
  connectionError,
  onRefreshConnection,
}: {
  initial: { csrf_token: string; identity: Identity };
  connected?: boolean;
  home?: string;
  switcher?: React.ReactNode;
  configurationPending?: boolean;
  loadedDigest?: string;
  connectionError?: unknown;
  onRefreshConnection?: () => void;
}) {
  const { api, identity, snapshot, page, online, diagnostic, refresh } =
    useWorkspaceConnection({
      initial,
      applicationConnected,
      home,
      connectionError,
      onRefreshConnection,
    });
  const {
    requests,
    error,
    clearError,
    send,
    submit,
    busy,
    reconcile,
    reconciling,
  } = useOriginalRequests(api, !!online);
  const candidateStarts = useCandidateStarts(
    api,
    !!online && !configurationPending,
    requests.data,
    submit,
  );
  const counts = {
    runs: page?.runs?.length ?? 0,
    questions:
      page?.runs?.filter(
        (r) => r.decision?.status === "pending" && requiresAnswer(r),
      ).length ?? 0,
  };
  return (
    <WorkspaceContext.Provider
      value={{
        api,
        identity,
        online: !!online,
        snapshot: snapshot.data,
        send,
        submit,
        requests: requests.data,
        candidateStarts,
        busy,
        refresh,
      }}
    >
      <AppShell
        projectId={identity.project_id}
        switcher={
          switcher ?? (
            <div className="project-card">{identity.repository_identity}</div>
          )
        }
        questions={counts.questions}
        footer={`${counts.runs} 个候选 · 业务状态由 Go 服务核验`}
        connection={
          <div className="connection">
            <span className={"connection-dot " + (online ? "" : "offline")} />
            <span aria-live="polite">{online ? "服务在线" : "连接待核实"}</span>
            <EnvironmentSummary />
            <span className="instance-label">
              {short(identity.instance_id)}
            </span>
            <Button
              size="sm"
              variant="ghost"
              aria-label="刷新查询"
              onClick={refresh}
            >
              <RefreshCw size={15} />
            </Button>
          </div>
        }
      >
        <ProjectMaintenance key={identity.project_id + identity.instance_id} />
        {!online && (
          <div className="notice amber" role="status">
            <strong>
              {connectionError instanceof ApiError &&
              connectionError.status === 401
                ? "会话已失效，请从本机 CLI 重新打开"
                : "当前不可提交业务操作"}
            </strong>
            <p>
              {diagnostic || "正在核验项目与服务实例…"}{" "}
              {snapshot.dataUpdatedAt
                ? "最后观察 " +
                  new Date(snapshot.dataUpdatedAt).toLocaleTimeString("zh-CN")
                : ""}
            </p>
          </div>
        )}
        {error && (
          <div role="alert" className="notice amber">
            <strong>操作结果未放行</strong>
            <p>{error}</p>
            <Button size="sm" onClick={clearError}>
              收起提示
            </Button>
          </div>
        )}
        {requests.error && (
          <div className="notice amber">
            本机请求存储不可用：{String(requests.error)}
          </div>
        )}
        {configurationPending && (
          <div className="notice amber" role="status">
            配置已保存，需重启 SteerLine 后才能启动新候选。已有执行保留原参数。
          </div>
        )}
        <Dispatch />
        <Suspense fallback={<div className="empty">正在加载页面…</div>}>
          <Routes>
            <Route
              path={"/projects/" + identity.project_id + "/roadmaps"}
              element={
                <Roadmaps
                  key={api.home + identity.project_id + identity.instance_id}
                />
              }
            />
            <Route
              path={"/projects/" + identity.project_id + "/environment"}
              element={
                <ProjectEnvironment
                  key={identity.project_id + identity.instance_id}
                />
              }
            />
            <Route
              path={"/projects/" + identity.project_id + "/tests"}
              element={<Tests />}
            />
            <Route
              path={"/projects/" + identity.project_id + "/dag"}
              element={<Dag />}
            />
            <Route
              path={"/projects/" + identity.project_id + "/runs"}
              element={<Runs />}
            />
            <Route
              path={"/projects/" + identity.project_id + "/settings"}
              element={
                <SettingsPage
                  api={api}
                  online={!!online}
                  home={home}
                  csrf={initial.csrf_token}
                  loadedDigest={loadedDigest}
                  applicationConnected={applicationConnected}
                />
              }
            />
            <Route
              path="*"
              element={
                <Navigate
                  to={"/projects/" + identity.project_id + "/dag"}
                  replace
                />
              }
            />
          </Routes>
        </Suspense>
        <Operations
          requests={(requests.data ?? []).filter(
            (r) => r.envelope.project_id === identity.project_id,
          )}
          online={!!online}
          reconcile={reconcile}
          reconciling={reconciling}
        />
      </AppShell>
    </WorkspaceContext.Provider>
  );
}

export function App({ initial }: { initial: BrowserSession }) {
  const {
    session,
    home,
    sessionValid,
    catalog,
    id,
    projects,
    selected,
    projectIdentity,
    choose,
    returnTo,
  } = useApplicationProjects(initial);
  const [search, setSearch] = useState("");
  const route = useLocation();
  const savedStatus = useQuery({
    queryKey: [home, "execution-config-status"],
    queryFn: ({ signal }) =>
      json<{ saved_digest: string }>("/api/web/execution-config/status", {
        signal,
      }),
    enabled: sessionValid,
    refetchInterval: 5000,
    retry: false,
  });
  const configurationPending =
    !!savedStatus.data?.saved_digest &&
    !!catalog.data?.loaded_execution_digest &&
    savedStatus.data.saved_digest !== catalog.data.loaded_execution_digest;
  const switcher = (
    <div className="project-selector">
      <label htmlFor="project-search">切换项目</label>
      <input
        id="project-search"
        placeholder="搜索名称或位置"
        value={search}
        onChange={(e) => setSearch(e.target.value)}
      />
      <select
        aria-label="当前项目"
        value={selected ? id : ""}
        onChange={(e) => {
          if (e.target.value) choose(e.target.value);
        }}
      >
        <option value="" disabled>
          选择已配置项目
        </option>
        {projects
          .filter(
            (p) =>
              p.project_id === id ||
              [p.name, p.repository, p.config_id]
                .join(" ")
                .toLowerCase()
                .includes(search.toLowerCase()),
          )
          .map((p) => (
            <option
              key={p.config_id}
              value={p.project_id ?? p.config_id}
              disabled={!p.project_id || !!p.conflict}
            >
              {p.name} · {p.repository} ·{" "}
              {p.conflict
                ? "配置冲突"
                : !p.project_id
                  ? "配置错误"
                  : p.online
                    ? "在线"
                    : {
                        starting: "启动中",
                        failed: "启动失败",
                        stopping: "停止中",
                        stopped: "已停止",
                        ready: "在线",
                      }[p.runtime_state ?? "stopped"]}
              {(!p.project_id || p.conflict) && (p.diagnostic || p.conflict)
                ? " · " + (p.diagnostic || p.conflict)
                : ""}
            </option>
          ))}
      </select>
      {selected && (
        <div className="project-current">
          <div>
            <strong>{selected.name}</strong>
            <small>
              {selected.online
                ? "在线"
                : {
                    starting: "启动中",
                    failed: "启动失败",
                    stopping: "停止中",
                    stopped: "已停止",
                    ready: "在线",
                  }[selected.runtime_state ?? "stopped"]}
            </small>
          </div>
          <span className="path" title={selected.repository}>
            {selected.repository}
          </span>
        </div>
      )}
      <Button size="sm" variant="ghost" onClick={() => void catalog.refetch()}>
        刷新项目目录
      </Button>
      {selected?.diagnostic && <p className="muted">{selected.diagnostic}</p>}
    </div>
  );
  if (
    route.pathname === "/settings/execution-configs" ||
    route.pathname === "/settings"
  )
    return (
      <AppShell
        projectId={selected?.project_id}
        switcher={switcher}
        returnTo={returnTo}
        connection={
          <div className="connection">
            <span
              className={
                "connection-dot " +
                (sessionValid && !catalog.isError ? "" : "offline")
              }
            />
            <span>
              {sessionValid && !catalog.isError ? "应用已连接" : "连接待核实"}
            </span>
          </div>
        }
        footer="所有项目共享执行方案与生成并发设置"
      >
        {route.pathname === "/settings/execution-configs" ? (
          <ExecutionConfigs
            key={home}
            home={home}
            csrf={session.data.csrf_token}
            loadedDigest={catalog.data?.loaded_execution_digest}
            applicationConnected={sessionValid && !catalog.isError}
          />
        ) : (
          <SettingsPage
            key={home}
            projectId={selected?.project_id}
            home={home}
            csrf={session.data.csrf_token}
            loadedDigest={catalog.data?.loaded_execution_digest}
            applicationConnected={sessionValid && !catalog.isError}
          />
        )}
      </AppShell>
    );
  const identity = projectIdentity.data;
  if (selected && identity && identity.project_id === id)
    return (
      <Workspace
        key={home + "/" + id + "/" + identity.instance_id}
        home={home}
        configurationPending={configurationPending}
        loadedDigest={catalog.data?.loaded_execution_digest}
        onRefreshConnection={() => {
          void session.refetch();
          void catalog.refetch();
          void projectIdentity.refetch();
        }}
        initial={{ identity, csrf_token: session.data.csrf_token }}
        connectionError={
          session.error ||
          catalog.error ||
          projectIdentity.error ||
          session.data.diagnostic
        }
        connected={
          sessionValid &&
          !catalog.isError &&
          !projectIdentity.isError &&
          !session.data.diagnostic &&
          !!selected.online
        }
        switcher={switcher}
      />
    );
  return (
    <div className="opening">
      <span className="brand-symbol">◆</span>
      <h1>SteerLine</h1>
      {switcher}
      <NavLink to="/settings/execution-configs">执行方案配置 →</NavLink>
      <NavLink to="/settings">配置 →</NavLink>
      {!sessionValid ? (
        <p role="alert">会话已失效，请使用 steerline-web open 重新授权</p>
      ) : id ? (
        <p role="status">
          {selected
            ? projectIdentity.error
              ? "项目离线，当前不可提交业务操作"
              : "正在核验项目连接…"
            : "项目不可访问，请重新选择"}
        </p>
      ) : (
        <p>{projects.length ? "请选择当前项目" : "尚无已配置项目"}</p>
      )}
      {catalog.error && <p role="alert">{String(catalog.error)}</p>}
      <p className="muted">
        项目目录来自当前应用 home 的 etc 配置。项目服务需通过本机入口启动。
      </p>
    </div>
  );
}
