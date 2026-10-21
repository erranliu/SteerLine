import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import type { Presentation, TaskPage } from "../../shared/api/types";

type Item = {
  plan: string;
  target: string;
  status: string;
  diagnostic?: string;
  review_digest: string;
  approval_revision: number;
  tests: string[];
};
type View = {
  tasks?: { task_id: string; plan: string; diagnostic: string }[];
  local_commit?: string;
  generations?: { id: string; plan: string; state: string }[];
  observation: {
    commit: string;
    previous?: string;
    checked_at: string;
    last_success?: string;
    diagnostic?: string;
    rewritten: boolean;
  };
  assessment: { commit: string; items: Item[] };
  hint?: string;
};
type Generation = {
  applied?: boolean;
  stop_status?: string;
  plan?: string;
  profile?: string;
  source_commit?: string;
  id: string;
  revision: number;
  state: string;
  target: string;
  diagnostic?: string;
  result?: unknown;
  review?: {
    review_digest: string;
    before: string;
    after: string;
    diagnostic?: string;
  };
};
type TaskReview = {
  task_id: string;
  version: number;
  contract: unknown;
  review_digest: string;
  diffs: { path: string; before: string; after: string; truncated: boolean }[];
};
const stateNames: Record<string, string> = {
  ready: "材料已核对",
  needs_review: "待核对",
  invalid: "材料缺失或无效",
  generated: "待审阅",
  needs_review_result: "空安排待确认",
  applied: "已应用到本地",
  cancelled: "已取消",
};
export function ProjectMaintenance() {
  const { api, identity, online, refresh, requests } = useWorkspace();
  const [open, setOpen] = useState(false),
    [mode, setMode] = useState("baseline"),
    [plan, setPlan] = useState(""),
    [profile, setProfile] = useState(""),
    [busy, setBusy] = useState(false),
    [error, setError] = useState("");
  const [generation, setGeneration] = useState(""),
    [task, setTask] = useState(""),
    [taskReview, setTaskReview] = useState<TaskReview>(),
    [emptyReview, setEmptyReview] = useState("");
  const base = ["maintenance", identity.project_id, identity.instance_id];
  const view = useQuery({
    queryKey: [...base, mode, plan],
    queryFn: () =>
      api.get<View>(
        "/maintenance?mode=" + mode + "&plan=" + encodeURIComponent(plan),
      ),
    enabled: online,
    refetchInterval: online ? 60000 : false,
    retry: false,
  });
  const profiles = useQuery({
    queryKey: [...base, "profiles"],
    queryFn: () => api.get<{ presentation: Presentation }>("/presentation"),
    enabled: online && open,
  });
  const tasks = useQuery({
    queryKey: [...base, "tasks"],
    queryFn: () => api.get<TaskPage>("/tasks?limit=500"),
    enabled: online && open,
  });
  const gen = useQuery({
    queryKey: [...base, "generation", generation],
    queryFn: () => api.get<Generation>("/tests/generations/" + generation),
    enabled: online && open && !!generation,
    refetchInterval: (q) =>
      online &&
      open &&
      !!generation &&
      q.state.data?.stop_status !== "stopped" &&
      q.state.data?.state !== "rejected"
        ? 5000
        : false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    retry: false,
  });
  const allowEmpty =
    !!gen.data?.review?.review_digest &&
    emptyReview === gen.data.review.review_digest;
  async function action(
    action: string,
    target: string,
    payload: Record<string, unknown>,
    version = 1,
  ) {
    setBusy(true);
    setError("");
    try {
      const result = await api.submit(
        api.envelope(action, target, Math.max(1, version), payload),
      );
      if (result.status !== "applied")
        throw new Error(result.diagnostic || "原操作结果待核实，请查询原请求");
      await view.refetch();
      if (
        generation &&
        ["maintenance_apply", "maintenance_stop_generation"].includes(action)
      )
        await gen.refetch();
      refresh();
      return result.maintenance;
    } catch (e) {
      setError(String(e));
      return undefined;
    } finally {
      setBusy(false);
    }
  }
  async function checkTask() {
    setBusy(true);
    setTaskReview(undefined);
    setError("");
    try {
      setTaskReview(
        await api.get<TaskReview>(
          "/maintenance/tasks/" + encodeURIComponent(task),
        ),
      );
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }
  const pendingRequests =
    requests?.filter(
      (r) =>
        r.envelope.action.startsWith("maintenance_") &&
        r.envelope.project_id === identity.project_id &&
        r.envelope.instance_id === identity.instance_id &&
        !["applied", "rejected"].includes(r.response?.status || ""),
    ) || [];
  const o = view.data?.observation;
  const pending = view.data?.assessment?.items.some(
    (i) => i.status !== "ready",
  );
  const title = o?.diagnostic
    ? "检查失败"
    : !o?.commit
      ? "基线待检查"
      : pending
        ? "材料待更新"
        : view.data?.local_commit && view.data.local_commit !== o.commit
          ? "本地与远端基线不同"
          : "公共材料已就绪";
  return (
    <section aria-label="项目维护" className="settings-panel maintenance-panel">
      <button onClick={() => setOpen(!open)} aria-expanded={open}>
        项目维护 · {title}
      </button>
      {o?.last_success && (
        <small> · 最近检查 {new Date(o.last_success).toLocaleString()}</small>
      )}
      {open && (
        <div>
          <p>
            目标基线 <code>{o?.commit || "尚无成功观察"}</code>
          </p>
          {view.data?.local_commit && view.data.local_commit !== o?.commit && (
            <p>
              本地与远端基线不同 · 本地 HEAD {view.data.local_commit}，请使用
              Git 核对并更新。
            </p>
          )}
          <p>
            请使用正常 Git
            流程更新本地仓库。仅源码变化建议核对测试适用性；必要材料待办只阻断相关任务首次启动。
          </p>
          {o?.rewritten && (
            <p className="notice amber">
              远端历史非快进，请核对 Git 历史；已有执行保持原基线。
            </p>
          )}
          {o?.diagnostic && <p role="alert">{o.diagnostic}</p>}
          {(error || view.error) && (
            <p role="alert">{error || String(view.error)}</p>
          )}
          <button
            disabled={!online || busy}
            onClick={() =>
              void action("maintenance_check", identity.project_id, {})
            }
          >
            检查远端
          </button>
          <label>
            维护范围{" "}
            <select
              value={mode}
              onChange={(e) => {
                setMode(e.target.value);
                setTaskReview(undefined);
              }}
            >
              <option value="baseline">基线更新</option>
              <option value="initialize">初始化检查</option>
              <option value="local">局部草稿</option>
            </select>
          </label>
          <label>
            Plan 路径{" "}
            <input
              value={plan}
              onChange={(e) => setPlan(e.target.value)}
              placeholder="留空查看全部任务 Plan"
            />
          </label>
          <p>
            {mode === "local"
              ? "工作区草稿 · 未提交或合入不计远端就绪"
              : "按远端固定提交核验"}
          </p>
          {(view.data?.assessment?.items || []).map((item, i) => (
            <article key={item.plan || i}>
              <strong>
                {item.plan || "公共材料"} ·{" "}
                {stateNames[item.status] || item.status}
              </strong>
              {item.diagnostic && <p>{item.diagnostic}</p>}
              <p>{item.tests?.join("、")}</p>
              {item.plan && item.status !== "invalid" && (
                <button
                  disabled={busy || !online}
                  onClick={() =>
                    void action(
                      "maintenance_review",
                      item.plan,
                      {
                        plan: item.plan,
                        mode,
                        review_digest: item.review_digest,
                      },
                      item.approval_revision,
                    )
                  }
                >
                  已审阅，采用当前安排
                </button>
              )}
              {item.plan && (
                <button onClick={() => setPlan(item.plan)}>选择此 Plan</button>
              )}
            </article>
          ))}
          <label>
            本机生成方案{" "}
            <select
              value={
                profile || profiles.data?.presentation.default_profile_id || ""
              }
              onChange={(e) => setProfile(e.target.value)}
            >
              {profiles.data?.presentation.profiles.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </label>
          <p>
            一次生成一个 Plan
            的测试安排；不运行测试，不自动应用或提交。提交/合入后重新核对远端材料状态。
          </p>
          <button
            disabled={
              busy ||
              !online ||
              !plan ||
              mode === "initialize" ||
              (mode !== "local" && !o?.commit)
            }
            onClick={() =>
              void action("maintenance_generate", plan, {
                plan,
                profile:
                  profile || profiles.data?.presentation.default_profile_id,
                commit: mode === "local" ? "" : o?.commit,
              }).then((v) => {
                if (v) setGeneration((v as Generation).id);
              })
            }
          >
            生成测试安排
          </button>
          <label>
            已有生成记录{" "}
            <select
              aria-label="已有生成记录"
              value={generation}
              onChange={(e) => setGeneration(e.target.value)}
            >
              <option value="">选择原生成</option>
              {view.data?.generations?.map((g) => (
                <option key={g.id} value={g.id}>
                  {g.plan} · {stateNames[g.state] || g.state} · {g.id}
                </option>
              ))}
            </select>
          </label>
          <button
            disabled={!online || busy || !generation}
            onClick={() => void gen.refetch()}
          >
            重新读取生成预览
          </button>
          <label>
            原生成请求{" "}
            <input
              value={generation}
              onChange={(e) => setGeneration(e.target.value)}
              placeholder="原生成 ID，可在重启后继续查询"
            />
          </label>
          {gen.data && (
            <div aria-label="生成预览">
              <p>
                {stateNames[gen.data.state] || gen.data.state} · {gen.data.id}
              </p>
              {gen.data.applied && gen.data.state !== "applied" && (
                <p>文件已应用；取消不会回滚已经发生的写入。</p>
              )}
              <p>
                输入 {gen.data.plan} · {gen.data.source_commit || "工作区草稿"}{" "}
                · {gen.data.profile}
              </p>
              <p>
                应用目标 <code>{gen.data.target}</code>
              </p>
              {gen.data.diagnostic && <p>{gen.data.diagnostic}</p>}
              {gen.data.stop_status !== "stopped" &&
                gen.data.state !== "rejected" && (
                  <p>原生成资源尚未核实停止；资源结算另行核对。</p>
                )}
              {gen.data.review && (
                <>
                  <p>{gen.data.review.diagnostic}</p>
                  <h4>当前文件</h4>
                  <pre>{gen.data.review.before}</pre>
                  <h4>拟应用内容</h4>
                  <pre>{gen.data.review.after}</pre>
                  <label>
                    <input
                      type="checkbox"
                      checked={allowEmpty}
                      onChange={(e) =>
                        setEmptyReview(
                          e.target.checked
                            ? gen.data?.review?.review_digest || ""
                            : "",
                        )
                      }
                    />
                    已确认空安排，Plan 已说明无需自动测试
                  </label>
                  <button
                    disabled={busy || !online || !gen.data.review.review_digest}
                    onClick={() =>
                      void action(
                        "maintenance_apply",
                        gen.data!.id,
                        {
                          generation_id: gen.data!.id,
                          expected_revision: gen.data!.revision,
                          review_digest: gen.data!.review!.review_digest,
                          allow_empty: allowEmpty,
                        },
                        gen.data!.revision,
                      )
                    }
                  >
                    应用已审阅内容
                  </button>
                </>
              )}
              <button
                disabled={
                  busy ||
                  !online ||
                  gen.data.state === "cancelled" ||
                  (gen.data.state === "applied" &&
                    gen.data.stop_status === "stopped")
                }
                onClick={() =>
                  void action("maintenance_stop_generation", gen.data!.id, {
                    generation_id: gen.data!.id,
                  })
                }
              >
                取消生成/禁止后续应用
              </button>
            </div>
          )}
          {gen.error && <p role="alert">{String(gen.error)}</p>}
          {pendingRequests.map((r) => (
            <p key={r.envelope.request_id}>
              原操作 {r.envelope.request_id} · {r.envelope.action}
              <button
                disabled={busy || !online}
                onClick={() =>
                  void api
                    .reconcile(r.envelope)
                    .then(() => refresh())
                    .catch((e) => setError(String(e)))
                }
              >
                核对原请求
              </button>
              {r.envelope.action === "maintenance_apply" && (
                <button
                  disabled={busy || !online}
                  onClick={() =>
                    void api
                      .continueMaintenanceApply(r.envelope)
                      .then(() => {
                        refresh();
                        void view.refetch();
                        void gen.refetch();
                      })
                      .catch((e) => setError(String(e)))
                  }
                >
                  显式继续原文件应用
                </button>
              )}
            </p>
          ))}
          <h4>未启动任务材料确认</h4>
          {view.data?.tasks?.map((t) => (
            <p key={t.task_id}>
              #{t.task_id} · {t.plan} · {t.diagnostic}
              <button
                onClick={() => {
                  setTask(t.task_id);
                  setTaskReview(undefined);
                }}
              >
                选择待更新任务 #{t.task_id}
              </button>
            </p>
          ))}
          <select
            aria-label="待确认任务"
            value={task}
            onChange={(e) => {
              setTask(e.target.value);
              setTaskReview(undefined);
            }}
          >
            <option value="">选择任务</option>
            {tasks.data?.tasks
              .filter((t) => t.status === "open")
              .map((t) => (
                <option key={t.task_id} value={t.task_id}>
                  #{t.task_id} {t.title}
                </option>
              ))}
          </select>
          <button
            disabled={busy || !online || !task}
            onClick={() => void checkTask()}
          >
            查看最新材料差异
          </button>
          {taskReview && (
            <div>
              {taskReview.diffs.map((d) => (
                <article key={d.path}>
                  <strong>{d.path}</strong>
                  {d.truncated && (
                    <p>仅显示前 8 KiB，请在仓库中核对完整差异。</p>
                  )}
                  <pre>{d.before}</pre>
                  <pre>{d.after}</pre>
                </article>
              ))}
              <button
                disabled={busy || !online}
                onClick={() =>
                  void action(
                    "maintenance_confirm_task",
                    taskReview.task_id,
                    {
                      task_id: taskReview.task_id,
                      version: taskReview.version,
                      contract: taskReview.contract,
                      review_digest: taskReview.review_digest,
                    },
                    taskReview.version,
                  ).then((v) => {
                    if (v) setTaskReview(undefined);
                  })
                }
              >
                已审阅，确认这些任务材料
              </button>
            </div>
          )}
        </div>
      )}
    </section>
  );
}
