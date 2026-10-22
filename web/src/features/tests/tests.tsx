import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { useWorkspace } from "../../app/context";
import { usePageVisible } from "../../shared/query-visibility";
import { Button } from "../../shared/ui/button";
import type {
  Catalogue,
  RecordRow,
  RecordPage,
  RecordDetail,
  RecordEvidence,
} from "./types";

const labels: Record<string, string> = {
  passed: "通过",
  failed: "失败",
  incomplete: "未完成",
  pending: "失败待复验",
  partial: "部分复验通过",
  multiple: "多次复验，分别查看",
  definition_changed: "定义变化，待另行核对",
  environment_changed: "环境变化，待另行核对",
  independent: "独立运行",
};
const retest = (r: RecordRow) =>
  r.retest === "passed" ? "复验通过" : (labels[r.retest] ?? r.retest);
const date = (v?: string) => (v ? new Date(v).toLocaleString("zh-CN") : "无");
const source = (r: RecordRow) =>
  r.source_fixed ? "固定源码记录" : "源码未固定（工作区有修改、变更或未知）";
const partial = (r: RecordRow) => (r.partial ? " · 部分范围" : "");

export function Tests() {
  const { api, identity, online } = useWorkspace();
  const [params, setParams] = useSearchParams();
  const visible = usePageVisible();
  const [view, setView] = useState("runs"),
    [showEvidence, setShowEvidence] = useState(false);
  const base = [
    api.home,
    identity.project_id,
    identity.instance_id,
    "test-records",
  ];
  const enabled = online && visible;
  const catalog = useQuery({
    queryKey: [...base, "catalogue"],
    queryFn: ({ signal }) => api.get<Catalogue>("/tests/catalogue", signal),
    enabled,
    retry: false,
    refetchInterval: enabled ? 30000 : false,
  });
  const testID = params.get("test") ?? catalog.data?.tests[0]?.id ?? "";
  const recordID = params.get("record") ?? "";
  const update = (key: string, value: string) => {
    const next = new URLSearchParams(params);
    if (value) next.set(key, value);
    else next.delete(key);
    next.delete("cursor");
    if (key !== "record") next.delete("record");
    setParams(next);
    setShowEvidence(false);
  };
  const query = new URLSearchParams({ test_id: testID, limit: "30" });
  for (const key of ["outcome", "retest", "environment", "source", "cursor"])
    if (params.get(key)) query.set(key, params.get(key)!);
  for (const key of ["from", "to"]) {
    const v = params.get(key);
    if (v) {
      const d = new Date(v + "T00:00:00");
      if (key === "to") d.setDate(d.getDate() + 1);
      if (Number.isFinite(d.getTime())) query.set(key, d.toISOString());
    }
  }
  const records = useQuery({
    queryKey: [...base, "list", query.toString()],
    queryFn: ({ signal }) =>
      api.get<RecordPage>("/tests/records?" + query, signal),
    enabled: enabled && !!testID && view === "runs",
    retry: false,
    refetchInterval: enabled && view === "runs" ? 10000 : false,
  });
  const detail = useQuery({
    queryKey: [...base, "detail", recordID],
    queryFn: ({ signal }) =>
      api.get<RecordDetail>(
        "/tests/records/" + encodeURIComponent(recordID),
        signal,
      ),
    enabled: enabled && !!recordID,
    retry: false,
  });
  const evidence = useQuery({
    queryKey: [...base, "evidence", recordID],
    queryFn: ({ signal }) =>
      api.get<RecordEvidence>(
        "/tests/records/" + encodeURIComponent(recordID) + "/evidence",
        signal,
      ),
    enabled: enabled && !!recordID && showEvidence,
    retry: false,
  });
  const search = params.get("search") ?? "",
    level = params.get("level") ?? "";
  const suites =
    catalog.data?.tests.filter(
      (d) =>
        (!level || d.level === level) &&
        `${d.id} ${d.name}`.toLowerCase().includes(search.toLowerCase()),
    ) ?? [];
  const definition = catalog.data?.tests.find((d) => d.id === testID);
  const environments = [
    ...new Set([
      ...(catalog.data?.tests.map((d) => d.environment) ?? []),
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
  const open = (r: RecordRow) => {
    setView("runs");
    update("record", r.id);
  };
  return (
    <section className="tests-page" aria-label="测试管理">
      <div className="page-heading">
        <div>
          <h1>测试管理</h1>
          <p className="muted">
            脚本登记的测试集历史与显式复验 · 通过仅证明当次范围
          </p>
        </div>
        <Button
          onClick={() => {
            void catalog.refetch();
            if (params.has("cursor")) {
              const next = new URLSearchParams(params);
              next.delete("cursor");
              setParams(next);
            } else void records.refetch();
            if (recordID) void detail.refetch();
            if (recordID && showEvidence) void evidence.refetch();
          }}
          disabled={!online}
        >
          刷新记录
        </Button>
      </div>
      {!online && (
        <p className="notice amber">
          离线 · 以下为最后观察，不能确认是否有新记录
        </p>
      )}
      {catalog.error && (
        <p className="notice amber">
          清单查询失败：{String(catalog.error)}。历史记录仍可查询。
        </p>
      )}
      {catalog.data?.diagnostic && (
        <p className="notice amber">{catalog.data.diagnostic}</p>
      )}
      <p className="muted">
        {catalog.data?.path} · {catalog.data?.commit || "基线未核实"} · 观察{" "}
        {date(catalog.data?.observed_at)}
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
              {!d.current && <span>当前清单未登记 · 历史定义</span>}
            </button>
          ))}
          {!suites.length && (
            <p className="muted">
              {catalog.isPending
                ? "正在读取目录…"
                : "没有匹配的测试集 / 该级别未登记"}
            </p>
          )}
        </aside>
        <div className="test-record-panel">
          <h2>{definition?.name || testID || "选择一个测试集"}</h2>
          <div className="button-row">
            <Button
              variant={view === "runs" ? "default" : "ghost"}
              onClick={() => setView("runs")}
            >
              运行记录
            </Button>
            <Button
              variant={view === "definition" ? "default" : "ghost"}
              onClick={() => setView("definition")}
            >
              测试定义
            </Button>
          </div>
          {view === "definition" && definition && (
            <section className="settings-panel">
              <p>
                {definition.id} · {definition.level} / {definition.environment}
              </p>
              <pre>{definition.command}</pre>
              <p>{definition.check}</p>
              {definition.note && <p>{definition.note}</p>}
              <p className="muted">
                命令仅供查看。清单存在不代表入口已实现或测试已通过。
              </p>
            </section>
          )}
          {view === "runs" && (
            <>
              <div className="test-record-filters">
                {filter("outcome", "原始结果", {
                  passed: "通过",
                  failed: "失败",
                  incomplete: "未完成",
                })}
                {filter("retest", "复验情况", {
                  pending: "失败待复验",
                  passed: "复验通过",
                  partial: "部分复验通过",
                  multiple: "多次复验",
                  definition_changed: "定义变化",
                  environment_changed: "环境变化",
                })}
                {filter(
                  "environment",
                  "环境",
                  Object.fromEntries(environments.map((e) => [e, e])),
                )}
                <label>
                  分支 / SHA
                  <input
                    aria-label="分支 / SHA"
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
                时间范围按开始时间筛选
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
                    次执行 · 失败待完整复验 {records.data.summary.pending}
                  </strong>
                  <p>
                    最近原始结果：
                    {records.data.summary.latest
                      ? labels[records.data.summary.latest.status] +
                        partial(records.data.summary.latest)
                      : "无"}{" "}
                    · 最近原始通过：
                    {date(records.data.summary.last_passed?.started_at)}
                    {records.data.summary.last_passed &&
                      ` · ${source(records.data.summary.last_passed)}${partial(records.data.summary.last_passed)} · 证据${records.data.summary.last_passed.evidence_status === "available" ? "可用" : "未核实"}`}
                  </p>
                  <p className="muted">
                    原始结果 / 复验筛选只控制列表；同链失败分别计数。观察{" "}
                    {date(records.data.observed_at)}
                    ，可刷新采用新增记录及证据状态。
                  </p>
                </div>
              )}
              {records.data?.records.map((r) => (
                <article key={r.id} className="test-record-card">
                  <button className="test-record-open" onClick={() => open(r)}>
                    <strong className={"record-" + r.status}>
                      {labels[r.status]}
                    </strong>{" "}
                    · {retest(r)}
                    {r.retest_source_fixed === false
                      ? "（复验源码未固定）"
                      : ""}
                    <span>
                      {date(r.started_at)} · {r.branch || "detached / 未知"} /{" "}
                      {r.commit?.slice(0, 12) || "源码未知"} · {r.environment}
                      {partial(r)}
                    </span>
                    <span>
                      {source(r)} · 证据
                      {r.evidence_status === "available" ? "可用" : "未核实"}
                    </span>
                    <code>{r.id}</code>
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
                <RecordDetails
                  detail={detail.data}
                  project={identity.project_id}
                  open={open}
                />
              )}
              {detail.data && (
                <Button
                  variant="ghost"
                  onClick={() => setShowEvidence(!showEvidence)}
                >
                  {showEvidence ? "收起证据" : "查看证据摘要"}
                </Button>
              )}
              {showEvidence && (
                <>
                  {evidence.error && (
                    <p className="notice amber">
                      证据查询失败：{String(evidence.error)}
                    </p>
                  )}
                  {evidence.data &&
                    (evidence.data.status === "available" ? (
                      <>
                        <p className="muted">
                          {evidence.data.path} · SHA256 {evidence.data.sha256}
                        </p>
                        <pre>{evidence.data.text}</pre>
                      </>
                    ) : (
                      <p className="notice amber">
                        证据不可用：{evidence.data.diagnostic} · 原引用{" "}
                        {evidence.data.path}
                      </p>
                    ))}
                </>
              )}
            </section>
          )}
        </div>
      </div>
    </section>
  );
}
function RecordDetails({
  detail: d,
  project,
  open,
}: {
  detail: RecordDetail;
  project: string;
  open: (r: RecordRow) => void;
}) {
  return (
    <>
      <p>
        {d.id} · 原始结果 {labels[d.status]} · {retest(d)}
        {d.retest_source_fixed === false ? "（复验源码未固定）" : ""}
      </p>
      <p>
        {date(d.started_at)} → {date(d.finished_at)} · {source(d)}
        {partial(d)}
      </p>
      <p>{d.message}</p>
      <p className="muted">{d.diagnostic}</p>
      <p>
        测试前 SHA：{d.report.before_commit || "未知"} · clean=
        {String(d.report.before_clean)}
      </p>
      <p>
        测试后 SHA：{d.report.after_commit || "未知"} · clean=
        {String(d.report.after_clean)}
      </p>
      <p>当次定义摘要：{d.report.definitions[d.test_id]}</p>
      <pre>{d.report.command}</pre>
      <p>{d.report.definition.check}</p>
      {d.report.scope?.items.map((item) => (
        <p key={item.id}>
          {item.id} · {labels[item.status]} · {item.message}
        </p>
      ))}
      <h4>显式复验过程</h4>
      {d.chain.map((r) => (
        <p key={r.id}>
          <button className="test-chain-link" onClick={() => open(r)}>
            {r.id} · {labels[r.status]} · {source(r)}
          </button>
          {r.rerun_of && ` ← ${r.rerun_of}`}
        </p>
      ))}
      <p className="muted">
        原失败保留；多次子集/不同 SHA
        的通过不拼成完整复验通过。历史通过不代表当前分支通过。
      </p>
      {d.source.kind === "runner" &&
      d.source.candidate_id &&
      d.source.task_id ? (
        <Link
          to={`/projects/${project}/runs?task=${encodeURIComponent(d.source.task_id!)}&run=${encodeURIComponent(d.source.candidate_id)}`}
        >
          查看来源候选
        </Link>
      ) : (
        <p className="muted">独立脚本记录，无已核验候选绑定</p>
      )}
    </>
  );
}
