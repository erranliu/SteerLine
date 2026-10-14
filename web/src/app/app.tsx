import {
  lazy,
  Suspense,
  useEffect,
  useMemo,
  useState,
  useSyncExternalStore,
} from "react";
import {
  onlineManager,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
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
import { Api, ApiError, json } from "../shared/api/client";
import { listRequests } from "../shared/api/journal";
import type { Identity, Snapshot, StoredRequest } from "../shared/api/types";
import { WorkspaceContext } from "./context";
import { Button } from "../shared/ui/button";
import { Dispatch } from "../features/tasks/dispatch";
import { requiresAnswer } from "../features/runs/state";
import { Status, short } from "../shared/ui/status";
const Dag = lazy(() =>
  import("../features/tasks/dag").then((module) => ({ default: module.Dag })),
);
const Runs = lazy(() =>
  import("../features/runs/runs").then((module) => ({ default: module.Runs })),
);

function Operations({
  requests,
  api,
  online,
  onError,
}: {
  requests: StoredRequest[];
  api: Api;
  online: boolean;
  onError: (e: string) => void;
}) {
  const queryClient = useQueryClient();
  const reconcile = useMutation({
    mutationFn: (row: StoredRequest) => api.reconcile(row.envelope),
    onSettled: () =>
      queryClient.invalidateQueries({ queryKey: ["local-requests"] }),
    onError: (error) => onError(String(error)),
  });
  return requests.length ? (
    <details className="operations">
      <summary>
        操作回执{" "}
        <span>
          {
            requests.filter(
              (r) =>
                !r.response ||
                ["submitted", "accepted", "outcome_unknown"].includes(
                  r.response.status,
                ),
            ).length
          }{" "}
          项待核实
        </span>
      </summary>
      {requests
        .slice()
        .reverse()
        .slice(0, 12)
        .map((row) => (
          <div className="operation" key={row.envelope.request_id}>
            <div>
              <strong>{row.envelope.action}</strong>
              <span>
                目标 {short(row.envelope.target_id)} · 请求{" "}
                {short(row.envelope.request_id)}
              </span>
              {row.error && <p className="muted">{row.error}</p>}
              {row.response?.diagnostic && (
                <p className="muted">{row.response.diagnostic}</p>
              )}
            </div>
            <Status value={row.response?.status ?? "outcome_unknown"} />
            {(!row.response ||
              !["applied", "rejected"].includes(row.response.status)) && (
              <Button
                size="sm"
                disabled={
                  !online ||
                  reconcile.isPending ||
                  row.envelope.instance_id !== api.identity.instance_id
                }
                onClick={() => reconcile.mutate(row)}
              >
                查询原请求
              </Button>
            )}
          </div>
        ))}
    </details>
  ) : null;
}
function SettingsPage({ api }: { api: Api }) {
  const result = useQuery({
    queryKey: [
      api.identity.project_id,
      api.identity.instance_id,
      "presentation",
    ],
    queryFn: ({ signal }) =>
      api.get<{
        presentation: {
          profiles: {
            id: string;
            name: string;
            host: string;
            agent: string;
            unavailable_reason: string;
            details: string[];
          }[];
          default_profile_id: string;
        };
      }>("/presentation", signal),
  });
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">PROJECT / SETTINGS</div>
          <h1>执行设置</h1>
          <p>当前服务已保存的执行方案与默认配置。</p>
        </div>
      </div>
      <section className="settings-panel">
        <h2>已有执行方案</h2>
        <p className="muted">
          方案由服务配置维护。每次启动可选择方案，冻结参数由 Go 服务核验。
        </p>
        {result.data?.presentation.profiles.map((p) => (
          <article className="profile" key={p.id}>
            <div>
              <strong>{p.name}</strong>
              {result.data.presentation.default_profile_id === p.id && (
                <Status value="ready" />
              )}
            </div>
            <p>
              {p.host} / {p.agent}
            </p>
            {p.details?.map((d, i) => (
              <p className="muted" key={i}>
                {d}
              </p>
            ))}
            {p.unavailable_reason && (
              <p className="notice amber">{p.unavailable_reason}</p>
            )}
          </article>
        ))}
        {result.error && <p className="notice amber">{String(result.error)}</p>}
        <h3>本机连接</h3>
        <p className="path">{api.identity.repository_identity}</p>
        <p className="muted">
          项目 {short(api.identity.project_id)} · 服务实例{" "}
          {short(api.identity.instance_id)}
        </p>
      </section>
    </>
  );
}
export function App({
  initial,
}: {
  initial: { csrf_token: string; identity: Identity };
}) {
  const queries = useQueryClient();
  const [error, setError] = useState("");
  const [errorRequest, setErrorRequest] = useState("");
  const networkOnline = useSyncExternalStore(
    (changed) => onlineManager.subscribe(changed),
    () => onlineManager.isOnline(),
  );
  const [collapsed, setCollapsed] = useState(false);
  const session = useQuery({
    queryKey: ["browser-session"],
    queryFn: () =>
      json<{ csrf_token: string; identity: Identity }>("/auth/status"),
    initialData: initial,
    refetchInterval: 2500,
    retry: false,
  });
  const identity = session.data.identity;
  const api = useMemo(
    () => new Api(identity, session.data.csrf_token),
    [identity.project_id, identity.instance_id, session.data.csrf_token],
  );
  const connected = !session.isError && identity.ready;
  const snapshot = useQuery({
    queryKey: [identity.project_id, identity.instance_id, "snapshot"],
    queryFn: ({ signal }) => api.get<Snapshot>("/snapshot", signal),
    enabled: connected,
    refetchInterval: 2000,
    retry: false,
  });
  const page = snapshot.data?.connection.snapshot.page;
  const online =
    networkOnline &&
    connected &&
    !snapshot.isError &&
    snapshot.data?.connection.state === "connected" &&
    snapshot.data.connection.can_control &&
    snapshot.data.connection.identity_matched &&
    snapshot.data.connection.config_matched &&
    page?.service.instance_id === identity.instance_id;
  const mutation = useMutation({
    mutationFn: (envelope: ReturnType<Api["envelope"]>) => api.submit(envelope),
    onError: (e, envelope) => {
      setError(String(e));
      setErrorRequest(envelope.request_id);
    },
    onSuccess: () => {
      setError("");
      setErrorRequest("");
    },
    onSettled: () => {
      queries.invalidateQueries({
        predicate: (q) =>
          q.queryKey[0] === identity.project_id ||
          q.queryKey[0] === "local-requests",
      });
    },
  });
  const requests = useQuery({
    queryKey: ["local-requests"],
    queryFn: listRequests,
    refetchInterval: 2000,
  });
  useEffect(() => {
    if (
      errorRequest &&
      requests.data?.find((row) => row.envelope.request_id === errorRequest)
        ?.response?.status === "applied"
    ) {
      setError("");
      setErrorRequest("");
    }
  }, [errorRequest, requests.dataUpdatedAt, requests.data]);
  useEffect(() => {
    if (!online) return;
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      try {
        const pending = (await listRequests()).filter(
          (row) =>
            row.envelope.project_id === identity.project_id &&
            row.envelope.instance_id === identity.instance_id &&
            (!row.response ||
              ["submitted", "accepted", "outcome_unknown"].includes(
                row.response.status,
              )),
        );
        for (const row of pending) {
          if (controller.signal.aborted) return;
          try {
            await api.reconcile(row.envelope, controller.signal);
            queries.invalidateQueries({ queryKey: ["local-requests"] });
          } catch {
            /* unknown stays in journal */
          }
        }
      } catch {
        // The journal query displays storage errors; keep this loop bounded.
      } finally {
        if (!controller.signal.aborted)
          timer = setTimeout(() => void poll(), 2000);
      }
    };
    void poll().catch(() => {});
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [online, api, queries]);
  const send = async (
    action: string,
    target: string,
    version: number,
    payload: Record<string, unknown>,
    extra: Record<string, number> = {},
  ) => {
    if (!online) throw new Error("当前连接不能变更业务状态");
    try {
      const result = await mutation.mutateAsync(
        api.envelope(action, target, version, payload, extra),
      );
      if (result.status === "rejected" || result.status === "outcome_unknown") {
        setError(result.diagnostic || "操作结果待核实，请查询原请求");
        setErrorRequest(result.request_id);
        return false;
      }
      return true;
    } catch {
      return false;
    }
  };
  const diagnostic = session.error
    ? String(session.error)
    : snapshot.error
      ? String(snapshot.error)
      : identity.diagnostic || snapshot.data?.connection.diagnostic;
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
        busy: mutation.isPending,
        refresh: () => queries.invalidateQueries(),
      }}
    >
      <div className={"app-shell " + (collapsed ? "sidebar-collapsed" : "")}>
        <aside className="sidebar">
          <div className="brand">
            <span className="brand-symbol">◆</span>
            <strong>SteerLine</strong>
            <span className="brand-caption">WORKSPACE</span>
          </div>
          <div className="project-card">
            <span className="project-avatar">S</span>
            <div>
              <strong>
                {identity.repository_identity
                  .replace(/[\\/]\.git$/, "")
                  .split(/[\\/]/)
                  .pop()}
              </strong>
              <span>本机工作空间</span>
            </div>
          </div>
          <p className="nav-heading">项目</p>
          <nav aria-label="主导航">
            <NavLink to="/dag">
              <GitBranch size={17} />
              <span>任务 DAG</span>
            </NavLink>
            <NavLink to="/runs">
              <Layers size={17} />
              <span>执行与待处理</span>
              {counts.questions > 0 && (
                <span className="nav-count">{counts.questions}</span>
              )}
            </NavLink>
          </nav>
          <div className="sidebar-footer">
            <NavLink to="/settings">
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
                onClick={() => queries.invalidateQueries()}
              >
                <RefreshCw size={15} />
              </Button>
            </div>
          </header>
          <main>
            {!online && (
              <div className="notice amber" role="status">
                <strong>
                  {session.error instanceof ApiError &&
                  session.error.status === 401
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
                <Button size="sm" onClick={() => setError("")}>
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
                <Route path="/dag" element={<Dag />} />
                <Route path="/runs" element={<Runs />} />
                <Route path="/settings" element={<SettingsPage api={api} />} />
                <Route path="*" element={<Navigate to="/dag" replace />} />
              </Routes>
            </Suspense>
            <Operations
              requests={(requests.data ?? []).filter(
                (r) => r.envelope.project_id === identity.project_id,
              )}
              api={api}
              online={!!online}
              onError={(message) => {
                setError(message);
                setErrorRequest("");
              }}
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
