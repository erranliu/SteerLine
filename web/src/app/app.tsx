import { lazy, Suspense, useState } from "react";
import { NavLink, Routes, Route, Navigate } from "react-router-dom";
import {
  GitBranch,
  Layers,
  Settings,
  RefreshCw,
  ArrowUpRight,
  Command,
  PanelLeftClose,
} from "lucide-react";
import { ApiError } from "../shared/api/client";
import type { Identity, BrowserSession } from "../shared/api/types";
import { WorkspaceContext } from "./context";
import { Button } from "../shared/ui/button";
import { Dispatch } from "../features/tasks/dispatch";
import { requiresAnswer } from "../features/runs/state";
import { short } from "../shared/ui/status";
import { SettingsPage } from "../features/settings/settings";
import { Operations } from "./operations";
import { useApplicationProjects } from "./use-application-projects";
import { useWorkspaceConnection } from "./use-workspace-connection";
import { useOriginalRequests } from "./use-original-requests";
const EnvironmentChecks = lazy(() =>
  import("../features/environment-checks/environment-checks").then(
    (module) => ({ default: module.EnvironmentChecks }),
  ),
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
  connectionError,
  onRefreshConnection,
}: {
  initial: { csrf_token: string; identity: Identity };
  connected?: boolean;
  home?: string;
  switcher?: React.ReactNode;
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
  const [collapsed, setCollapsed] = useState(false);
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
        busy,
        refresh,
      }}
    >
      <div className={"app-shell " + (collapsed ? "sidebar-collapsed" : "")}>
        <aside className="sidebar">
          <div className="brand">
            <span className="brand-symbol">◆</span>
            <strong>SteerLine</strong>
            <span className="brand-caption">WORKSPACE</span>
          </div>
          {switcher ?? (
            <div className="project-card">{identity.repository_identity}</div>
          )}
          <p className="nav-heading">项目</p>
          <nav aria-label="主导航">
            <NavLink to={"/projects/" + identity.project_id + "/dag"}>
              <GitBranch size={17} />
              <span>任务 DAG</span>
            </NavLink>
            <NavLink to={"/projects/" + identity.project_id + "/runs"}>
              <Layers size={17} />
              <span>执行与待处理</span>
              {counts.questions > 0 && (
                <span className="nav-count">{counts.questions}</span>
              )}
            </NavLink>
          </nav>
          <div className="sidebar-footer">
            <NavLink to={"/projects/" + identity.project_id + "/settings"}>
              <Settings size={16} />
              <span>执行设置</span>
            </NavLink>
            <div className="sidebar-note">
              <span className="connection-dot" /> LOCAL SERVICE{" "}
              <ArrowUpRight size={12} />
            </div>
          </div>
        </aside>
        <div className="main-shell">
          <header className="topbar">
            <div className="breadcrumb">
              <Button
                variant="ghost"
                size="sm"
                aria-label="切换导航宽度"
                onClick={() => setCollapsed(!collapsed)}
              >
                <PanelLeftClose size={17} />
              </Button>
              <span>当前项目</span>
              <span>/</span>
              <strong>规划与执行</strong>
            </div>
            <div className="connection">
              <span className={"connection-dot " + (online ? "" : "offline")} />
              <span aria-live="polite">
                {online ? "服务已连接" : "连接待核实"}
              </span>
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
          </header>
          <main>
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
                      new Date(snapshot.dataUpdatedAt).toLocaleTimeString(
                        "zh-CN",
                      )
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
            <Dispatch />
            <Suspense fallback={<div className="empty">正在加载页面…</div>}>
              <Routes>
                <Route
                  path={
                    "/projects/" + identity.project_id + "/environment-checks"
                  }
                  element={<EnvironmentChecks />}
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
                  element={<SettingsPage api={api} online={!!online} />}
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
          </main>
          <footer className="app-footer">
            <span>
              <Command size={12} /> SteerLine · 本机 Web
            </span>
            <span>{counts.runs} 个候选 · 业务状态由 Go 服务核验</span>
          </footer>
        </div>
      </div>
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
  } = useApplicationProjects(initial);
  const [search, setSearch] = useState("");
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
  const identity = projectIdentity.data;
  if (selected && identity && identity.project_id === id)
    return (
      <Workspace
        key={home + "/" + id + "/" + identity.instance_id}
        home={home}
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
