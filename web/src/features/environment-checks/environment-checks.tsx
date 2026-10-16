import { useEffect, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { NavLink } from "react-router-dom";
import { useWorkspace } from "../../app/context";
import { projectQueries } from "../../shared/api/queries";
import { Button } from "../../shared/ui/button";
import type { CheckExecution, CheckRecord } from "./types";

const labels: Record<string, string> = {
  passed: "通过",
  failed: "失败",
  unverified: "未核实",
  not_applicable: "不适用",
  pending: "待处理",
  running: "执行中",
  stopped: "已停止",
  queued: "等待执行",
  residual: "资源残留",
  completed: "已完成",
  unsupported: "机制不支持",
};
const label = (value: string) => labels[value] ?? value;
const freshExecution = (): CheckExecution => ({
  adapter: "codex-app-server",
  target: { kind: "local" },
  settings: { executable: "codex" },
});
function unresolved(row: CheckRecord) {
  return (
    row.operations[0]?.status !== "rejected" &&
    (!row.report ||
      row.report.cleanup_status !== "completed" ||
      row.operations.some((op) => op.status === "outcome_unknown"))
  );
}
function sameExecution(a: CheckExecution, b: CheckExecution) {
  const canonical = (v: unknown): string =>
    v && typeof v === "object"
      ? JSON.stringify(
          Object.entries(v)
            .sort(([x], [y]) => x.localeCompare(y))
            .map(([k, value]) => [k, canonical(value)]),
        )
      : JSON.stringify(v);
  return canonical(a) === canonical(b);
}

function Report({
  row,
  execution,
  online,
  pending,
  stop,
  refresh,
}: {
  row: CheckRecord;
  execution: CheckExecution;
  online: boolean;
  pending: boolean;
  stop: () => void;
  refresh: () => void;
}) {
  const report = row.report;
  const unknown = row.operations.some((op) => op.status === "outcome_unknown");
  const repeatedStop = row.operations.some(
    (op) =>
      op.action === "stop_environment_check" &&
      op.status !== "rejected" &&
      (op.status === "outcome_unknown" ||
        (op.runner_request.input.expected_revision ?? Infinity) >=
          (report?.revision ?? 0)),
  );
  return (
    <section className="settings-panel" aria-label="检查报告">
      <h2>检查报告</h2>
      <p>
        原请求 <code>{row.id}</code> · 检查 {row.check_id || "尚无可信绑定"}
      </p>
      <p>
        Runner {row.runner.version} · {row.input.execution.adapter} /{" "}
        {row.input.execution.target.kind}{" "}
        {row.input.execution.target.environment}
      </p>
      <p>
        权限模式：
        {row.input.execution.settings.access_mode === "full_access"
          ? "完全访问"
          : "沙箱"}
      </p>
      {!sameExecution(row.input.execution, execution) && (
        <p className="notice amber">结果不适用于当前选择</p>
      )}
      {!online && (
        <p className="notice amber">执行连接待核实，继续只读查询原检查。</p>
      )}
      {row.diagnostic && (
        <p className="notice amber">
          源状态未核实：{row.diagnostic}。保留最后可信报告。
        </p>
      )}
      {row.operations
        .filter((op) => op.diagnostic)
        .map((op) => (
          <p className="notice amber" key={op.request_id}>
            {op.action}：{op.diagnostic}
          </p>
        ))}
      {unknown && (
        <p className="notice amber">
          原操作结果未知，只查询原请求，不重新派发。
        </p>
      )}
      {report ? (
        <>
          <p>
            执行：{label(report.status)} · 能力结论：
            {label(report.result)}
          </p>
          {report.diagnostic && (
            <div className="notice amber" aria-label="检查诊断">
              {report.diagnostic ===
                "native_preparation_failed: native_executable_unavailable" && (
                <p>
                  未能启动 Codex：Worker
                  无法找到或运行配置的程序。请沿用已有执行方案的 Agent
                  executable，或填写 Worker 上可用的 Codex
                  命令或绝对路径后重新检查。
                </p>
              )}
              <p>{report.diagnostic}</p>
            </div>
          )}
          <p>
            源更新时间：
            <time dateTime={report.updated_at}>
              {new Date(report.updated_at).toLocaleString("zh-CN", {
                hour12: false,
              })}
            </time>{" "}
            · revision {report.revision}
          </p>
          <p className="muted">
            历史证据仅代表本次上下文；认证和权限可能已变化。
          </p>
          <dl>
            {Object.entries(report.actual ?? {}).map(([key, value]) => (
              <div key={key}>
                <dt>{key}</dt>
                <dd>{value === "" ? "尚未取得" : value}</dd>
              </div>
            ))}
          </dl>
          {report.items.map((item) => (
            <article className="profile" key={item.id}>
              <strong>
                {item.id} · {label(item.status)}
              </strong>
              <p>{item.stage}</p>
              <details>
                <summary>查看证据</summary>
                {(item.evidence ?? []).map((entry, index) => (
                  <div key={index}>
                    <p>{entry.source}</p>
                    <pre>{JSON.stringify(entry.facts ?? {}, null, 2)}</pre>
                  </div>
                ))}
              </details>
            </article>
          ))}
          {report.resources.pr_url && (
            <p>
              测试 PR：
              {report.resources.pr_url.startsWith("https://") ? (
                <a
                  href={report.resources.pr_url}
                  target="_blank"
                  rel="noreferrer"
                >
                  {report.resources.pr_url}
                </a>
              ) : (
                report.resources.pr_url
              )}
            </p>
          )}
          <p
            className={
              report.cleanup_status === "completed" ? "muted" : "notice amber"
            }
          >
            资源：{label(report.cleanup_status)}
            {report.cleanup_status !== "completed" ? " · 资源待处理" : ""}{" "}
          </p>
        </>
      ) : (
        <p>
          尚无可信报告
          {row.operations[0]?.status === "rejected"
            ? "，原请求已拒绝。"
            : "，请查询原请求。"}
        </p>
      )}
      <div className="check-actions">
        <Button disabled={pending} onClick={refresh}>
          查询原请求
        </Button>
        <Button
          disabled={
            !online ||
            pending ||
            !row.check_id ||
            !report?.allowed_actions.includes("stop") ||
            repeatedStop ||
            unknown ||
            !!row.diagnostic
          }
          onClick={stop}
        >
          停止 / 收尾原检查
        </Button>
      </div>
      <p className="muted">
        未检查构建、测试和业务资源。关闭页面会停止刷新，检查仍由 Runner 管理。
      </p>
    </section>
  );
}

export function EnvironmentChecks() {
  const { api, identity, online, send, busy } = useWorkspace();
  const client = useQueryClient();
  const queries = projectQueries(api);
  const [visible, setVisible] = useState(document.visibilityState !== "hidden");
  // Read-only settings/history remain accessible while execution is unready.
  const enabled = visible;
  const catalog = useQuery({ ...queries.checkOptions(), enabled });
  const history = useQuery({ ...queries.checks(), enabled });
  const [profileID, setProfileID] = useState("");
  const [draft, setDraft] = useState<CheckExecution | null>(null);
  const targetDrafts = useRef<Record<string, CheckExecution>>({});
  const [mode, setMode] = useState("sandbox");
  const [selected, setSelected] = useState("");
  const selectedID = selected || history.data?.checks[0]?.id || "";
  const detail = useQuery({
    ...queries.check(selectedID),
    enabled: enabled && !!selectedID,
    // Execution readiness does not determine whether history can be read.
    refetchInterval: 3000,
    refetchIntervalInBackground: false,
  });
  const rows = history.data?.checks ?? [];
  const row = detail.data?.check ?? rows.find((r) => r.id === selectedID);
  useEffect(() => {
    const updated = detail.data?.check;
    if (!updated) return;
    client.setQueryData<{ checks: CheckRecord[] }>(
      projectQueries(api).checks().queryKey,
      (history) =>
        history && {
          ...history,
          checks: history.checks.map((r) =>
            r.id === updated.id ? updated : r,
          ),
        },
    );
  }, [api, client, detail.data]);
  const [pending, setPending] = useState(false);
  const locked = useRef(false);
  const [error, setError] = useState("");
  const profile =
    catalog.data?.profiles.find((p) => p.id === profileID) ??
    catalog.data?.profiles[0];
  const source = draft ?? profile?.execution ?? freshExecution();
  const execution = {
    ...source,
    settings: { ...source.settings, access_mode: mode },
  };
  const remote = source.target.kind === "orca_environment";
  const agent = remote
    ? ((source.settings.agent_launch ?? {}) as Record<string, unknown>)
    : source.settings;
  const supported =
    catalog.data?.capabilities?.contract ===
      "steerrunner-environment-check/v1" &&
    catalog.data.capabilities.targets?.some(
      (p) =>
        p.mechanism === "implemented" &&
        p.adapter === execution.adapter &&
        p.target === execution.target.kind &&
        p.access_modes.includes(mode),
    );
  const unsettled = rows.some((r) => unresolved(r.id === row?.id ? row : r));
  const unavailable = catalog.error
    ? String(catalog.error)
    : catalog.data?.unavailable_reason ||
      (!supported ? "Runner 未声明支持此目标与权限模式，机制不可用。" : "");
  const updateAgent = (key: string, value: string) => {
    const values = { ...agent, [key]: value };
    if (!value && key !== "executable") delete values[key];
    setDraft({
      ...source,
      settings: remote ? { ...source.settings, agent_launch: values } : values,
    });
  };
  useEffect(() => {
    const scope = [api.home, identity.project_id, identity.instance_id];
    const cancel = () =>
      client.cancelQueries({
        predicate: (q) =>
          scope.every((v, i) => q.queryKey[i] === v) &&
          ["check", "checks", "check-options"].includes(String(q.queryKey[3])),
      });
    const update = () => {
      const shown = document.visibilityState !== "hidden";
      setVisible(shown);
      if (!shown) void cancel();
    };
    document.addEventListener("visibilitychange", update);
    return () => {
      document.removeEventListener("visibilitychange", update);
      void cancel();
    };
  }, [api, client, identity.project_id, identity.instance_id]);
  const act = async (
    action: string,
    target: string,
    version: number,
    payload: Record<string, unknown>,
  ) => {
    if (locked.current) return;
    locked.current = true;
    setPending(true);
    setError("");
    try {
      await send(action, target, version, payload);
      await client.invalidateQueries({ queryKey: queries.checks().queryKey });
      if (action === "start_environment_check") setSelected("");
      else await detail.refetch();
    } catch (e) {
      setError(String(e));
    } finally {
      setPending(false);
      locked.current = false;
    }
  };
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">PROJECT / ENVIRONMENT</div>
          <h1>环境检查</h1>
          <p>使用实际 Agent 验证预先准备好的执行权限、Git 和项目发布能力。</p>
        </div>
        <NavLink to={"/projects/" + identity.project_id + "/settings"}>
          执行设置
        </NavLink>
      </div>
      <section className="settings-panel">
        <h2>本次检查</h2>
        <div className="check-fields">
          <label>
            预填执行方案
            <select
              value={profileID || profile?.id || ""}
              onChange={(e) => {
                setProfileID(e.target.value);
                setDraft(null);
                targetDrafts.current = {};
              }}
            >
              <option value="" disabled>
                选择方案
              </option>
              {catalog.data?.profiles.map((p) => (
                <option key={p.id} value={p.id}>
                  {p.name}
                </option>
              ))}
            </select>
          </label>
          <label>
            执行目标
            <select
              value={source.target.kind}
              onChange={(e) => {
                const kind = e.target.value;
                if (kind === source.target.kind) return;
                targetDrafts.current[source.target.kind] = source;
                const configured = catalog.data?.profiles.find(
                  (p) => p.execution.target.kind === kind,
                )?.execution;
                setDraft(
                  targetDrafts.current[kind] ??
                    configured ??
                    (kind === "local"
                      ? freshExecution()
                      : {
                          adapter: "orca-terminal",
                          target: { kind },
                          settings: {
                            transport: { executable: "orca" },
                            agent_launch: { executable: "codex" },
                          },
                        }),
                );
              }}
            >
              <option value="local">本机 Codex</option>
              <option value="orca_environment">Orca 远端 Codex</option>
            </select>
          </label>
          {remote && (
            <label>
              Orca environment
              <input
                type="text"
                value={source.target.environment ?? ""}
                onChange={(e) =>
                  setDraft({
                    ...source,
                    target: { ...source.target, environment: e.target.value },
                  })
                }
              />
            </label>
          )}
          <label>
            Agent executable
            <input
              type="text"
              aria-describedby="check-executable-help"
              value={String(agent.executable ?? "")}
              onChange={(e) => updateAgent("executable", e.target.value)}
            />
          </label>
          <label>
            模型
            <input
              type="text"
              placeholder="Runner 默认模型"
              value={String(agent.model ?? "")}
              onChange={(e) => updateAgent("model", e.target.value)}
            />
          </label>
          <label>
            推理强度
            <input
              type="text"
              placeholder="Runner 默认强度"
              value={String(agent.reasoning_effort ?? "")}
              onChange={(e) => updateAgent("reasoning_effort", e.target.value)}
            />
          </label>
          <label>
            权限模式
            <select value={mode} onChange={(e) => setMode(e.target.value)}>
              <option value="sandbox">沙箱</option>
              <option value="full_access">完全访问</option>
            </select>
          </label>
        </div>
        <p id="check-executable-help" className="muted">
          Agent executable 使用 Worker 上可用的命令或绝对路径，优先沿用已有执行方案。
        </p>
        <div className="notice" aria-label="执行预览">
          <strong>执行预览</strong>
          <p>
            项目：{identity.repository_identity} · 仓库：
            {catalog.data?.repository || "正在读取"}
          </p>
          <p>
            {execution.adapter} / {execution.target.kind}{" "}
            {execution.target.environment} · 模型{" "}
            {String(agent.model || "Runner 默认")}
          </p>
          <p>
            {mode === "sandbox"
              ? "使用 Worker 提前准备的沙箱。"
              : "完全访问：直接使用 Worker 沙箱外权限。"}
            两种模式均不依赖执行中审批，失败不自动切换模式。
          </p>
          <p>
            开始会调用模型并消耗额度；使用本次专属
            steerline-check/&lt;check_id&gt;/base 和 head 创建测试分支、提交与
            PR，再关闭 PR 并收尾资源。不会合并 PR。
          </p>
          <p>未检查构建、测试和业务资源。</p>
        </div>
        {unavailable && <p className="notice amber">{unavailable}</p>}
        {unsettled && (
          <p className="notice amber">
            已有检查或资源未核实，请先查询原请求并处理。
          </p>
        )}
        {error && (
          <p role="alert" className="notice amber">
            {error}
          </p>
        )}
        <Button
          variant="default"
          disabled={
            !online ||
            !enabled ||
            busy ||
            pending ||
            !!unavailable ||
            unsettled ||
            !history.data ||
            !!history.error ||
            (remote && !source.target.environment) ||
            !agent.executable
          }
          onClick={() =>
            void act("start_environment_check", identity.project_id, 0, {
              execution,
              access_mode: mode,
            })
          }
        >
          开始实际检查
        </Button>
      </section>
      <section className="settings-panel">
        <h2>检查历史</h2>
        {history.error && (
          <p className="notice amber">历史读取失败：{String(history.error)}</p>
        )}
        {!rows.length && <p className="muted">暂无检查记录。</p>}
        <div className="check-actions">
          {rows.map((r) => (
            <Button
              key={r.id}
              aria-pressed={r.id === selectedID}
              onClick={() => setSelected(r.id)}
            >
              {new Date(r.created_at).toLocaleString("zh-CN")} ·{" "}
              {r.report ? label(r.report.result) : "未核实"}
            </Button>
          ))}
        </div>
      </section>
      {detail.error && (
        <p className="notice amber">
          源查询失败，保留缓存：{String(detail.error)}
        </p>
      )}
      {row && (
        <Report
          row={row}
          execution={execution}
          online={online}
          pending={pending || busy || detail.isFetching}
          refresh={() => void detail.refetch()}
          stop={() =>
            void act("stop_environment_check", row.id, row.report!.revision, {})
          }
        />
      )}
    </>
  );
}
