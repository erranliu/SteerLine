import { useEffect, useMemo, useRef, useState } from "react";
import { createPortal } from "react-dom";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useLocation, useNavigate } from "react-router-dom";
import { Activity as ActivityIcon, X, AlertCircle } from "lucide-react";
import { useWorkspace } from "../../app/context";
import { usePageVisible } from "../../shared/query-visibility";
import { Button } from "../../shared/ui/button";
import {
  activityActions,
  activityDestination,
  ended,
  matchesActivity,
  provisionalActivities,
  type Activity,
  type ActivityFilter,
  type ActivityPage,
  type ActivityLinks,
} from "./model";
import "./activities.css";
import { useActivityFeedback } from "./feedback/feedback";

export type ActivityUI = {
  open: boolean;
  filters: Record<string, ActivityFilter>;
};
export const initialActivityUI: ActivityUI = { open: false, filters: {} };
export type ActivityControl = {
  value: ActivityUI;
  change: (value: ActivityUI) => void;
};
function useNarrow() {
  const [narrow, setNarrow] = useState(
    () => window.matchMedia?.("(max-width: 640px)").matches ?? false,
  );
  useEffect(() => {
    const m = window.matchMedia?.("(max-width: 640px)");
    if (!m) return;
    const update = () => setNarrow(m.matches);
    update();
    m.addEventListener("change", update);
    return () => m.removeEventListener("change", update);
  }, []);
  return narrow;
}
function clock(value?: string) {
  return value ? new Date(value).toLocaleString("zh-CN") : "时间未记录";
}

export function Activities({
  control,
  projectName,
}: {
  control?: ActivityControl;
  projectName?: string;
}) {
  const feedback = useActivityFeedback();
  const {
    api,
    identity,
    online,
    requests = [],
    candidateStarts,
  } = useWorkspace();
  const [local, setLocal] = useState(initialActivityUI);
  const ui = control?.value ?? local;
  const change = control?.change ?? setLocal;
  const preference = api.home + "/" + identity.project_id;
  const filter = ui.filters[preference] || "ongoing";
  const queriedFilter = ui.open ? filter : "ongoing";
  const narrow = useNarrow(),
    visible = usePageVisible();
  const [expanded, setExpanded] = useState("");
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    if (!ui.open || !visible) return;
    setNow(Date.now());
    const timer = setInterval(() => setNow(Date.now()), 15000);
    return () => clearInterval(timer);
  }, [ui.open, visible]);
  const opener = useRef<HTMLButtonElement>(null),
    drawer = useRef<HTMLElement>(null);
  const navigate = useNavigate(),
    location = useLocation();
  const destination = useRef<string | undefined>(undefined);
  const navigated = useRef(false);
  const close = () => {
    const restore =
      !navigated.current && !!drawer.current?.contains(document.activeElement);
    change({ ...ui, open: false });
    queueMicrotask(() => {
      if (restore && opener.current?.isConnected) opener.current.focus();
    });
  };
  const ids = requests
    .filter(
      (r) =>
        r.envelope.project_id === identity.project_id &&
        r.envelope.instance_id === identity.instance_id &&
        activityActions.has(r.envelope.action),
    )
    .map((r) => r.envelope.request_id)
    .sort();
  const idsKey = ids.join(",");
  const query = useInfiniteQuery({
    queryKey: [
      api.home,
      identity.project_id,
      identity.instance_id,
      "async-activities",
      queriedFilter,
    ],
    initialPageParam: "",
    queryFn: async ({ signal, pageParam }) => {
      const params = new URLSearchParams({
        filter: queriedFilter,
        limit: "50",
      });
      if (pageParam) params.set("cursor", pageParam);
      const page = await api.get<ActivityPage>("/activities?" + params, signal);
      if (
        !Array.isArray(page.items) ||
        !Array.isArray(page.sources) ||
        !page.request_links ||
        !page.counts ||
        page.project_id !== identity.project_id ||
        page.instance_id !== identity.instance_id
      )
        throw new Error("活动范围已变化，等待重新读取");
      return page;
    },
    getNextPageParam: (last) => (last.has_more ? last.next_cursor : undefined),
    enabled: online && visible,
    retry: false,
    refetchInterval: visible && online ? (ui.open ? 2000 : 10000) : false,
  });
  // Request associations change independently of the paginated list. A new
  // journal request must not discard pages already loaded in the drawer.
  const associationQuery = useQuery({
    queryKey: [
      api.home,
      identity.project_id,
      identity.instance_id,
      "activity-links",
      idsKey,
    ],
    queryFn: async ({ signal }) => {
      const links: Record<string, Activity> = {};
      const diagnostics: string[] = [];
      for (let offset = 0; offset < ids.length; offset += 100) {
        const extra = new URLSearchParams({
          mode: "links",
          request_ids: ids.slice(offset, offset + 100).join(","),
        });
        const linked = await api.get<ActivityLinks>(
          "/activities?" + extra,
          signal,
        );
        if (
          linked.project_id !== identity.project_id ||
          linked.instance_id !== identity.instance_id ||
          !linked.request_links ||
          !Array.isArray(linked.sources)
        )
          throw new Error("活动关联范围已变化");
        Object.assign(links, linked.request_links);
        for (const s of linked.sources)
          if (s.diagnostic) diagnostics.push(s.diagnostic);
      }
      return { links, diagnostics };
    },
    enabled: online && visible && ids.length > 0,
    retry: false,
    refetchInterval: visible && online ? (ui.open ? 2000 : 10000) : false,
  });
  const saved = useRef<Record<string, Activity[]>>({});
  const page = query.data?.pages[0];
  const official = useMemo(() => {
    const pages = query.data?.pages;
    if (!pages) return saved.current[queriedFilter] || [];
    const failed = new Set(
      pages.flatMap((p) =>
        p.sources.filter((s) => s.diagnostic).map((s) => s.source),
      ),
    );
    const items = new Map(
      (saved.current[queriedFilter] || [])
        .filter((a) => failed.has(a.source))
        .map((a) => [a.activity_id, a]),
    );
    for (const p of pages) for (const a of p.items) items.set(a.activity_id, a);
    const result = [...items.values()];
    saved.current[queriedFilter] = result;
    return result;
  }, [query.data, queriedFilter]);
  const retainedLinks = useRef<Record<string, Activity>>({});
  const links = Object.assign(
    retainedLinks.current,
    ...(query.data?.pages.map((p) => p.request_links) || []),
    associationQuery.data?.links,
  );
  const provisional = provisionalActivities(
    identity.project_id,
    identity.instance_id,
    requests,
    candidateStarts?.items || [],
    links,
    official,
  );
  const ordered = [...official, ...provisional]
    .filter((a) => matchesActivity(a, filter))
    .sort((a, b) => {
      const left = filter === "recent" ? a.finished_at : a.created_at,
        right = filter === "recent" ? b.finished_at : b.created_at;
      if (!left || !right)
        return left
          ? -1
          : right
            ? 1
            : a.activity_id.localeCompare(b.activity_id);
      const diff = Date.parse(left) - Date.parse(right);
      return diff
        ? filter === "recent"
          ? -diff
          : diff
        : a.activity_id.localeCompare(b.activity_id);
    });
  const items = filter === "recent" ? ordered.slice(0, 50) : ordered;
  const faults = [
    ...new Set([
      ...(page?.sources.filter((s) => s.diagnostic).map((s) => s.diagnostic) ||
        []),
      ...(associationQuery.data?.diagnostics || []),
    ]),
  ];
  const uncertain =
    !online ||
    query.isError ||
    associationQuery.isError ||
    !!faults.length ||
    !page?.counts_known;
  const count = (f: ActivityFilter) =>
    page && page.counts_known && online && !query.isError && !faults.length
      ? page.counts[f]
      : "待核实";
  const pending = provisional.filter((a) => !ended(a)).length;

  useEffect(() => {
    if (visible && online) void query.refetch();
  }, [visible, online]); // refetch only the read observation on visibility return
  useEffect(() => {
    if (!destination.current) return;
    const actual = location.pathname + location.search + location.hash;
    if (actual !== destination.current) return;
    navigated.current = true;
    destination.current = undefined;
    close();
    queueMicrotask(() => {
      const heading = document.querySelector<HTMLElement>("main h1,main h2");
      if (heading) {
        heading.tabIndex = -1;
        heading.focus();
      }
    });
  }, [location]);
  useEffect(() => {
    if (!ui.open) {
      setExpanded("");
      return;
    }
    const panel = drawer.current;
    panel?.focus();
    return () => {
      navigated.current = false;
    };
  }, [ui.open]);
  useEffect(() => {
    if (!ui.open || !narrow) return;
    const root = document.getElementById("root"),
      before = root?.inert,
      beforeOverflowX = root?.style.overflowX;
    const overflow = document.body.style.overflow;
    if (root) {
      root.inert = true;
      root.style.overflowX = "clip";
    }
    document.body.style.overflow = "hidden";
    if (!drawer.current?.contains(document.activeElement))
      drawer.current?.focus();
    return () => {
      if (root) {
        root.inert = before ?? false;
        root.style.overflowX = beforeOverflowX || "";
      }
      document.body.style.overflow = overflow;
    };
  }, [ui.open, narrow]);
  function keyboard(event: React.KeyboardEvent) {
    if (event.key === "Escape") {
      event.stopPropagation();
      close();
    }
    if (event.key !== "Tab" || !narrow) return;
    const nodes = [
      ...(drawer.current?.querySelectorAll<HTMLElement>(
        'button:not(:disabled),a[href],[tabindex="0"]',
      ) || []),
    ];
    if (!nodes.length) {
      event.preventDefault();
      return;
    }
    if (
      event.shiftKey &&
      (document.activeElement === nodes[0] ||
        document.activeElement === drawer.current)
    ) {
      event.preventDefault();
      nodes.at(-1)?.focus();
    } else if (
      !event.shiftKey &&
      (document.activeElement === nodes.at(-1) ||
        document.activeElement === drawer.current)
    ) {
      event.preventDefault();
      nodes[0].focus();
    }
  }
  function details(a: Activity) {
    if (a.details_ref.kind === "preparation" && a.details_ref.historical) {
      setExpanded(expanded === a.activity_id ? "" : a.activity_id);
      return;
    }
    const target = activityDestination(identity.project_id, a);
    destination.current = target;
    navigate(target);
  }
  return (
    <>
      <button
        ref={(node) => { opener.current = node; if (feedback.entry) feedback.entry.current = node; }}
        type="button"
        className="button button-ghost activity-entry"
        aria-label={`活动 · ${count("ongoing")} 项进行中 · 需处理 ${count("attention")}${pending ? ` · ${pending} 项提交待确认` : ""}`}
        aria-expanded={ui.open}
        aria-controls="async-activity-center"
        onClick={() => change({ ...ui, open: !ui.open })}
      >
        <ActivityIcon size={17} />
        <span>活动</span>
        <span className="activity-entry-count">{count("ongoing")}</span>
        {page?.counts_known && page.counts.attention > 0 && (
          <span className="activity-attention">
            <AlertCircle size={14} />
            需处理 {count("attention")}
          </span>
        )}
        {pending > 0 && (
          <span className="activity-entry-count">提交待确认 {pending}</span>
        )}
      </button>
      {ui.open &&
        createPortal(
          <>
            {narrow && (
              <div
                className="activity-backdrop"
                onClick={close}
                aria-hidden="true"
              />
            )}
            <section
              id="async-activity-center"
              ref={drawer}
              tabIndex={-1}
              role={narrow ? "dialog" : "region"}
              aria-modal={narrow ? true : undefined}
              aria-labelledby="async-activity-title"
              className={`activity-drawer ${narrow ? "activity-drawer-narrow" : ""}`}
              onKeyDown={keyboard}
            >
              <header>
                <h2 id="async-activity-title">
                  活动中心 ·{" "}
                  {projectName ||
                    identity.repository_identity ||
                    identity.project_id}
                </h2>
                <Button
                  aria-label="关闭活动中心"
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    close();
                  }}
                >
                  <X size={18} />
                </Button>
              </header>
              <div className="activity-filters" aria-label="活动筛选">
                {(
                  [
                    ["ongoing", "进行中"],
                    ["attention", "需处理"],
                    ["recent", "最近结束"],
                  ] as const
                ).map(([key, label]) => (
                  <button
                    key={key}
                    type="button"
                    aria-pressed={filter === key}
                    onClick={() => {
                      change({
                        ...ui,
                        filters: { ...ui.filters, [preference]: key },
                      });
                      drawer.current
                        ?.querySelector(".activity-scroll")
                        ?.scrollTo?.(0, 0);
                    }}
                  >
                    {label} {count(key)}
                  </button>
                ))}
              </div>
              <div className="activity-observation" aria-live="polite">
                {!online || query.isError
                  ? "连接或查询中断，保留上次已知状态"
                  : faults.length
                    ? "部分来源不可用，保留上次已知状态"
                    : !page?.counts_known && page
                      ? "部分结果 · 数量待核实"
                      : "当前项目所有已接入活动"}
              </div>
              {faults.length > 0 && (
                <details className="activity-source-errors">
                  <summary>查看来源诊断</summary>
                  {faults.map((f) => (
                    <p key={f}>{f}</p>
                  ))}
                </details>
              )}
              <div className="activity-scroll">
                {!page && !query.isError && online && (
                  <p role="status">正在读取活动…</p>
                )}
                {!page && (query.isError || !online) && <p>尚未取得进度</p>}
                {page && !items.length && !uncertain && (
                  <p>
                    {filter === "ongoing"
                      ? "暂无进行中的活动"
                      : filter === "attention"
                        ? "暂无需要处理的活动"
                        : "暂无最近结束的活动"}
                  </p>
                )}
                {page && !items.length && uncertain && (
                  <p>当前结果不完整，等待重新核实</p>
                )}
                {items.map((a) => (
                  <article
                    className="activity-card"
                    key={a.activity_id}
                    data-activity-id={a.activity_id}
                  >
                    <strong>{a.title}</strong>
                    <small>
                      {(
                        {
                          detection: "检测",
                          task_execution: "任务执行",
                          submission: "提交确认",
                        } as Record<string, string>
                      )[a.kind] || "活动"}
                      {a.needs_attention && " · 需处理"}
                    </small>
                    <p>{a.phase || "状态待核实"}</p>
                    {a.summary && <p className="muted">{a.summary}</p>}
                    {a.progress && (
                      <p>
                        {a.progress.total
                          ? `${a.progress.label} ${a.progress.succeeded}/${a.progress.total} · 未完成 ${a.progress.pending}${a.progress.failed ? ` · 失败 ${a.progress.failed}` : ""}`
                          : "无需新增准备"}
                        {a.progress.diagnostic && (
                          <span className="muted">
                            {" "}
                            · {a.progress.diagnostic}
                          </span>
                        )}
                      </p>
                    )}
                    {!ended(a) &&
                      (uncertain || a.observation !== "current") && (
                        <p className="muted">上次已知状态 · 当前待核实</p>
                      )}
                    <small>
                      {ended(a)
                        ? "结束于 " + clock(a.finished_at)
                        : "最后确认于 " + clock(a.source_observed_at)}
                      {!ended(a) &&
                        a.created_at &&
                        ` · 已持续 ${Math.max(0, Math.floor((now - Date.parse(a.created_at)) / 60000))} 分钟`}
                    </small>
                    {a.diagnostic && (
                      <details>
                        <summary>查看诊断</summary>
                        <p>{a.diagnostic}</p>
                      </details>
                    )}
                    {expanded === a.activity_id && (
                      <div className="activity-history">
                        <p>历史批次：{a.source_id}</p>
                        <p>
                          {a.phase} ·{" "}
                          {a.diagnostic || a.summary || "以上为本批次结果"}
                        </p>
                        <small>原项目维护区域展示当前批次</small>
                        <Button
                          variant="ghost"
                          onClick={() => {
                            destination.current = activityDestination(
                              identity.project_id,
                              a,
                            );
                            navigate(destination.current);
                          }}
                        >
                          前往当前项目维护
                        </Button>
                      </div>
                    )}
                    <Button
                      variant="ghost"
                      size="sm"
                      onClick={() => details(a)}
                    >
                      {a.details_ref.kind === "preparation" &&
                      a.details_ref.historical
                        ? "查看历史摘要"
                        : a.needs_attention
                          ? "去处理"
                          : "查看详情"}
                    </Button>
                  </article>
                ))}
                {query.hasNextPage && (
                  <Button
                    disabled={query.isFetchingNextPage}
                    onClick={() => void query.fetchNextPage()}
                  >
                    {query.isFetchingNextPage ? "正在加载…" : "加载更多"}
                  </Button>
                )}
              </div>
              <footer>
                <Button
                  variant="ghost"
                  disabled={!online || query.isFetching}
                  onClick={() => void query.refetch()}
                >
                  重新读取活动
                </Button>
                {page && <small>查询采样于 {clock(page.sampled_at)}</small>}
              </footer>
            </section>
          </>,
          document.body,
        )}
    </>
  );
}
