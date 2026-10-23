import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { useWorkspace } from "../../app/context";
import { usePageVisible } from "../../shared/query-visibility";
import { Button } from "../../shared/ui/button";
import type { Catalogue, RecordRow, RecordPage } from "./types";

const labels: Record<string, string> = {
  passed: "通过",
  failed: "失败",
  incomplete: "未完成",
};
const date = (v?: string) => (v ? new Date(v).toLocaleString("zh-CN") : "无");
export function Tests() {
  const { api, identity, online } = useWorkspace();
  const [params, setParams] = useSearchParams();
  const visible = usePageVisible();
  const [view, setView] = useState("runs");
  const enabled = online && visible;
  const base = [
    api.home,
    identity.project_id,
    identity.instance_id,
    "test-records",
  ];
  const catalogue = useQuery({
    queryKey: [...base, "catalogue"],
    queryFn: ({ signal }) => api.get<Catalogue>("/tests/catalogue", signal),
    enabled,
    retry: false,
    refetchInterval: enabled ? 30000 : false,
  });
  const testID = params.get("test") ?? catalogue.data?.tests[0]?.id ?? "";
  const recordID = params.get("record") ?? "";
  const update = (key: string, value: string) => {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    next.delete("cursor");
    if (key !== "record") next.delete("record");
    setParams(next);
  };
  const query = new URLSearchParams({ test_id: testID, limit: "30" });
  for (const key of ["outcome", "environment", "source", "cursor"])
    if (params.get(key)) query.set(key, params.get(key)!);
  for (const key of ["from", "to"]) {
    const value = params.get(key);
    if (value) {
      const d = new Date(value + "T00:00:00");
      if (key === "to") d.setDate(d.getDate() + 1);
      if (Number.isFinite(d.getTime())) query.set(key, d.toISOString());
    }
  }
  const records = useQuery({
    queryKey: [...base, "list", catalogue.data?.commit, query.toString()],
    queryFn: ({ signal }) =>
      api.get<RecordPage>("/tests/records?" + query, signal),
    enabled: enabled && !!testID && view === "runs",
    retry: false,
    refetchInterval: enabled && view === "runs" ? 10000 : false,
  });
  const detail = useQuery({
    queryKey: [...base, "detail", catalogue.data?.commit, recordID],
    queryFn: ({ signal }) =>
      api.get<RecordRow>(
        "/tests/records/" + encodeURIComponent(recordID),
        signal,
      ),
    enabled: enabled && !!recordID,
    retry: false,
  });
  const search = params.get("search") ?? "",
    level = params.get("level") ?? "";
  const suites =
    catalogue.data?.tests.filter(
      (d) =>
        (!level || d.level === level) &&
        `${d.id} ${d.name}`.toLowerCase().includes(search.toLowerCase()),
    ) ?? [];
  const definition = catalogue.data?.tests.find((d) => d.id === testID);
  const environments = [
    ...new Set([
      ...(catalogue.data?.tests.map((d) => d.environment) ?? []),
      ...(records.data?.records.map((r) => r.environment) ?? []),
    ]),
  ].sort();
  const filter = (
    name: string,
    title: string,
    options: Record<string, string>,
  ) => (
    <label>
      {title}
      <select
        aria-label={title}
        value={params.get(name) ?? ""}
        onChange={(e) => update(name, e.target.value)}
      >
        <option value="">全部</option>
        {Object.entries(options).map(([id, text]) => (
          <option key={id} value={id}>
            {text}
          </option>
        ))}
      </select>
    </label>
  );
  return (
    <section className="tests-page" aria-label="测试管理">
      <div className="page-heading">
        <div>
          <h1>测试管理</h1>
          <p className="muted">Git 中的测试历史 · 通过仅证明当次受测提交</p>
        </div>
        <Button
          disabled={!online}
          onClick={() => {
            void catalogue.refetch();
            if (params.has("cursor")) {
              const next = new URLSearchParams(params);
              next.delete("cursor");
              setParams(next);
            } else void records.refetch();
            if (recordID) void detail.refetch();
          }}
        >
          刷新记录
        </Button>
      </div>
      {!online && (
        <p className="notice amber">
          离线 · 以下为最后观察，不能确认是否有新记录
        </p>
      )}
      {catalogue.error && (
        <p className="notice amber">清单查询失败：{String(catalogue.error)}</p>
      )}
      {catalogue.data?.diagnostic && (
        <p className="notice amber">{catalogue.data.diagnostic}</p>
      )}
      <p className="muted">
        {catalogue.data?.path} · {catalogue.data?.commit || "基线未核实"} · 观察{" "}
        {date(catalogue.data?.observed_at)}
      </p>
      <div className="test-catalogue-filters">
        <label>
          搜索测试集
          <input
            aria-label="搜索测试集"
            value={search}
            onChange={(e) => update("search", e.target.value)}
            placeholder="名称 / ID"
          />
        </label>
        {filter("level", "测试级别", {
          L1: "L1",
          L2: "L2",
          L3: "L3",
          L4: "L4",
        })}
      </div>
      <div className="tests-workspace">
        <aside className="test-suites" aria-label="测试集">
          {suites.map((d) => (
            <button
              key={d.id}
              className={d.id === testID ? "test-suite selected" : "test-suite"}
              onClick={() => update("test", d.id)}
            >
              <strong>{d.name}</strong>
              <span>
                {d.id} · {d.level} / {d.environment}
              </span>
              {!d.current && <span>当前清单未登记 · 仅有历史结果</span>}
            </button>
          ))}
          {!suites.length && <p className="empty">没有符合条件的测试集</p>}
        </aside>
        <div className="test-runs">
          <div className="button-row">
            <Button variant="ghost" onClick={() => setView("runs")}>
              执行记录
            </Button>
            <Button variant="ghost" onClick={() => setView("definition")}>
              测试定义
            </Button>
          </div>
          {view === "definition" ? (
            definition?.current ? (
              <section className="settings-panel">
                <h3>{definition.name}</h3>
                <p>
                  当前主分支定义 · {definition.level} / {definition.environment}
                </p>
                <pre>{definition.command}</pre>
                <p>{definition.check}</p>
                {definition.note && <p>{definition.note}</p>}
              </section>
            ) : (
              <p className="muted">
                当前清单未登记；历史文件只保留测试 ID、级别、环境和结果。
              </p>
            )
          ) : (
            <>
              <div className="test-record-filters">
                {filter("outcome", "原始结果", labels)}
                {filter(
                  "environment",
                  "环境",
                  Object.fromEntries(environments.map((e) => [e, e])),
                )}
                <label>
                  受测 SHA
                  <input
                    aria-label="受测 SHA"
                    value={params.get("source") ?? ""}
                    onChange={(e) => update("source", e.target.value)}
                  />
                </label>
                <label>
                  开始日期
                  <input
                    type="date"
                    aria-label="开始日期"
                    value={params.get("from") ?? ""}
                    onChange={(e) => update("from", e.target.value)}
                  />
                </label>
                <label>
                  结束日期
                  <input
                    type="date"
                    aria-label="结束日期"
                    value={params.get("to") ?? ""}
                    onChange={(e) => update("to", e.target.value)}
                  />
                </label>
              </div>
              <p className="muted">
                日期时区：{Intl.DateTimeFormat().resolvedOptions().timeZone} ·
                按记录时间筛选
              </p>
              <Button
                variant="ghost"
                onClick={() => setParams(testID ? { test: testID } : {})}
              >
                清除筛选
              </Button>
              {records.error && (
                <p className="notice amber">
                  记录查询失败：{String(records.error)}
                </p>
              )}
              {records.data && (
                <div className="notice">
                  <strong>
                    所选代码 / 环境 / 日期范围：{records.data.summary.total}{" "}
                    次执行 · 原始失败 {records.data.summary.failed}
                  </strong>
                  <p>
                    最近原始结果：
                    {records.data.summary.latest
                      ? labels[records.data.summary.latest.status]
                      : "无"}{" "}
                    · 最近原始通过：{date(records.data.summary.last_passed?.at)}
                  </p>
                  <p className="muted">
                    结果筛选只控制列表；失败和后续通过分别保留。读取{" "}
                    {records.data.path} · 主分支快照 {records.data.commit} ·
                    观察 {date(records.data.observed_at)}
                  </p>
                </div>
              )}
              {records.data?.records.map((r) => (
                <article key={r.id} className="test-record-card">
                  <button
                    className="test-record-open"
                    onClick={() => update("record", r.id)}
                  >
                    <strong className={"record-" + r.status}>
                      {labels[r.status]}
                    </strong>
                    <span>
                      {date(r.at)} · {r.commit.slice(0, 12)} · {r.level} /{" "}
                      {r.environment}
                    </span>
                    {r.note && <span>{r.note}</span>}
                  </button>
                </article>
              ))}
              {records.data?.records.length === 0 && (
                <p className="empty">
                  {records.data.summary.total
                    ? "没有符合筛选条件的记录"
                    : "尚无执行记录"}
                </p>
              )}
              {records.isPending && testID && online && (
                <p className="muted">正在查询记录…</p>
              )}
              {params.has("cursor") && (
                <Button variant="ghost" onClick={() => update("cursor", "")}>
                  返回第一页
                </Button>
              )}
              {records.data?.cursor && (
                <Button
                  onClick={() => {
                    const next = new URLSearchParams(params);
                    next.set("cursor", records.data!.cursor);
                    setParams(next);
                  }}
                >
                  下一页
                </Button>
              )}
            </>
          )}
          {recordID && (
            <section className="settings-panel" aria-label="记录详情">
              <div className="button-row">
                <h3>记录详情</h3>
                <Button variant="ghost" onClick={() => update("record", "")}>
                  关闭详情
                </Button>
              </div>
              {detail.error && (
                <p className="notice amber">
                  详情查询失败：{String(detail.error)}
                </p>
              )}
              {detail.data && (
                <>
                  <p>
                    {detail.data.test_id} · {labels[detail.data.status]} ·{" "}
                    {detail.data.level} / {detail.data.environment}
                  </p>
                  <p>{date(detail.data.at)}</p>
                  <p>受测 SHA：{detail.data.commit}</p>
                  <p>Git 文件：{detail.data.path}</p>
                  {detail.data.note && <p>{detail.data.note}</p>}
                  <p className="muted">
                    完整日志保留在原测试报告目录。历史通过不代表当前主分支通过。
                  </p>
                </>
              )}
            </section>
          )}
        </div>
      </div>
    </section>
  );
}
