import { useLayoutEffect, useRef } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useSearchParams } from "react-router-dom";
import { useWorkspace } from "../../app/context";
import {
  usePageVisible,
  useQueryVisibility,
} from "../../shared/query-visibility";
import { Button } from "../../shared/ui/button";
import {
  activityDestination,
  ended,
  formatActivityTime,
  type Activity,
  type ActivityListFilter,
} from "./model";
import { useActivityList } from "./use-activity-list";
import { ActivityConversation } from "./activity-conversation";
import { CopyExecutionInfo } from "./execution-info";
import { readingRoot } from "./reading-api";
import "./activity-page.css";

function Message({ activity }: { activity: Activity }) {
  const message = activity.message;
  return (
    <div className="reading-message">
      {message?.text ? (
        <>
          <small>
            {message.kind === "current_question"
              ? "当前问题"
              : message.kind === "verified_final"
                ? "已核实的最终回复"
                : message.kind === "assistant"
                  ? "最后观察到的可读消息"
                  : "已保存结果"}{" "}
            ·{" "}
            {message.at
              ? formatActivityTime(message.at)
              : message.observed_at
                ? "观察于 " + formatActivityTime(message.observed_at)
                : "时间未记录"}
            {message.observed_at &&
            Date.now() - Date.parse(message.observed_at) > 60000
              ? " · 样本已陈旧"
              : ""}
          </small>
          <p className="reading-preview">{message.text}</p>
          {message.truncated && <small>有界预览</small>}
          {message.diagnostic && (
            <p className="notice amber">
              消息观察暂不可用 · {message.diagnostic}
            </p>
          )}
        </>
      ) : (
        <p className="muted">暂无已观察到的可读消息</p>
      )}
      {!message?.text && message?.diagnostic && (
        <p className="notice amber">消息观察暂不可用 · {message.diagnostic}</p>
      )}
    </div>
  );
}

export function ActivityPage() {
  const { api, online, identity } = useWorkspace();
  const [params, setParams] = useSearchParams();
  const text = params.get("filter");
  const filter: ActivityListFilter =
    text === "attention" || text === "history" ? text : "ongoing";
  const selected = params.get("activity") || "";
  const listElement = useRef<HTMLDivElement>(null);
  const detailElement = useRef<HTMLDivElement>(null);
  const listPosition = useRef<{ id: string; y: number } | null>(null);
  const previousSelection = useRef("");
  useLayoutEffect(() => {
    const previous = previousSelection.current;
    previousSelection.current = selected;
    if (window.innerWidth > 640 || previous === selected) return;
    if (selected) {
      detailElement.current
        ?.querySelector<HTMLButtonElement>(".reading-back")
        ?.focus({ preventScroll: true });
      window.scrollTo({ top: 0 });
    } else if (previous) {
      const position = listPosition.current;
      const button = [
        ...(listElement.current?.querySelectorAll<HTMLButtonElement>(
          ".reading-select",
        ) || []),
      ].find((node) => node.dataset.readingId === position?.id);
      (
        button || listElement.current?.querySelector<HTMLHeadingElement>("h1")
      )?.focus({ preventScroll: true });
      window.scrollTo({ top: position?.y || 0 });
    }
  }, [selected]);
  const visible = usePageVisible();
  const list = useActivityList(api, filter, online && visible, 2000);
  const activities = new Map<string, Activity>();
  for (const page of list.data?.pages ?? []) {
    for (const item of page.items) {
      if (!activities.has(item.activity_id))
        activities.set(item.activity_id, item);
    }
  }
  const items = [...activities.values()];
  const first = list.data?.pages[0];
  const setSelection = (id: string) =>
    setParams((current) => {
      const next = new URLSearchParams(current);
      if (id) next.set("activity", id);
      else next.delete("activity");
      return next;
    });
  return (
    <section
      className={"activity-reading " + (selected ? "reading-selected" : "")}
      aria-label="活动中心独立页"
    >
      <div className="reading-list" ref={listElement}>
        <h1 tabIndex={-1}>活动中心</h1>
        <div className="reading-filters" aria-label="独立页活动筛选">
          {(
            [
              ["ongoing", "进行中"],
              ["attention", "需处理"],
              ["history", "历史"],
            ] as const
          ).map(([key, label]) => (
            <Button
              key={key}
              variant={filter === key ? "default" : "ghost"}
              aria-pressed={filter === key}
              onClick={() =>
                setParams((current) => {
                  listPosition.current = null;
                  const next = new URLSearchParams(current);
                  next.set("filter", key);
                  next.delete("activity");
                  return next;
                })
              }
            >
              {label}{" "}
              {first?.counts_known ? (first.counts[key] ?? "待核实") : "待核实"}
            </Button>
          ))}
        </div>
        <Button
          size="sm"
          variant="ghost"
          disabled={!online || list.isFetching}
          onClick={() => void list.refetch().catch(() => {})}
        >
          刷新活动列表
        </Button>
        {(list.error ||
          (list.latest ?? first)?.sources.some((s) => !s.complete)) && (
          <p className="notice amber">
            活动信息不完整，保留上次已知内容。
            {String(list.error || "部分来源未能确认")}
          </p>
        )}
        {!online && <p className="muted">服务离线 · 已暂停刷新</p>}
        {list.hasUpdates && <p role="status">有更新，刷新后读取新快照。</p>}
        {!items.length && (
          <p className="muted">
            {list.isPending && online
              ? "正在读取活动…"
              : "此筛选下暂无已确认的活动"}
          </p>
        )}
        <div className="reading-list-items">
          {items.map((activity) => (
            <article
              key={activity.activity_id}
              className={
                selected === activity.activity_id ? "reading-active" : ""
              }
            >
              <button
                className="reading-select"
                data-reading-id={activity.activity_id}
                onClick={() => {
                  listPosition.current = {
                    id: activity.activity_id,
                    y: window.scrollY,
                  };
                  setSelection(activity.activity_id);
                }}
                aria-pressed={selected === activity.activity_id}
              >
                <strong>{activity.title}</strong>
                <span>
                  {activity.phase || activity.state}
                  {activity.needs_attention ? " · 需处理" : ""}
                </span>
              </button>
              <Message activity={activity} />
              {filter === "history" && (
                <small>
                  {activity.finished_at
                    ? "结束于 " + formatActivityTime(activity.finished_at)
                    : "结束时间未记录"}
                </small>
              )}
            </article>
          ))}
        </div>
        {list.hasNextPage && (
          <Button
            disabled={!online || list.isFetchingNextPage || list.hasUpdates}
            onClick={() => void list.fetchNextPage()}
          >
            加载更多活动
          </Button>
        )}
      </div>
      <div className="reading-detail" ref={detailElement}>
        {selected ? (
          <ActivityDetail
            key={
              api.home + identity.project_id + identity.instance_id + selected
            }
            id={selected}
            onBack={() => setSelection("")}
          />
        ) : (
          <p className="empty">选择活动查看摘要，需要时再展开已记录的交互。</p>
        )}
      </div>
    </section>
  );
}

function ActivityDetail({ id, onBack }: { id: string; onBack: () => void }) {
  const { api, online, identity } = useWorkspace();
  const key = [
    api.home,
    identity.project_id,
    identity.instance_id,
    "activity-summary",
    id,
  ];
  const enabled = useQueryVisibility(online, key);
  const summary = useQuery({
    queryKey: key,
    queryFn: async ({ signal }) => {
      const value = await api.get<Activity>(readingRoot(id), signal);
      if (value.activity_id !== id || value.project_id !== identity.project_id)
        throw new Error("活动摘要目标已变化");
      return value;
    },
    enabled,
    retry: false,
    refetchInterval: (query) =>
      enabled && query.state.data && !ended(query.state.data) ? 2000 : false,
    refetchOnWindowFocus: (query) =>
      query.state.data && ended(query.state.data) ? false : "always",
    refetchOnReconnect: false,
  });
  const activity = summary.data;
  return (
    <section aria-label="所选活动摘要">
      <Button className="reading-back" variant="ghost" onClick={onBack}>
        返回活动列表
      </Button>
      {summary.error && (
        <p className="notice amber">
          摘要读取失败，保留上次已知内容。{String(summary.error)}
        </p>
      )}
      {!activity ? (
        <p className="muted">
          {summary.isPending && online ? "正在读取摘要…" : "暂无可确认摘要"}
        </p>
      ) : (
        <>
          <h2>{activity.title}</h2>
          <p>{activity.phase || activity.state}</p>
          {activity.summary && <p>{activity.summary}</p>}
          {activity.diagnostic && (
            <p className="notice amber">{activity.diagnostic}</p>
          )}
          <Message activity={activity} />
          <div className="reading-actions">
            <Link
              className="button button-ghost"
              to={activityDestination(identity.project_id, activity)}
            >
              {activity.needs_attention ? "去处理" : "查看原详情"}
            </Link>
            <CopyExecutionInfo
              info={{
                title: activity.title,
                project: identity.repository_identity,
                requestID: activity.execution_ref?.request_id,
                record: { id: activity.source_id, label: "业务记录" },
                taskID: activity.details_ref.task_id,
                runner: activity.execution_ref,
              }}
            />
          </div>
          <ActivityConversation
            key={activity.activity_id}
            activity={activity}
          />
        </>
      )}
    </section>
  );
}
