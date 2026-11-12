import { useEffect, useLayoutEffect, useRef, useState } from "react";
import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
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
  type Activity,
  type ActivityListFilter,
} from "./model";
import { useActivityList } from "./use-activity-list";
import { CandidateConversation } from "./candidate-conversation";
import { CopyExecutionInfo } from "./execution-info";
import {
  readBody,
  readInteractions,
  readingRoot,
  type ReadingBody,
  type ReadingItem,
} from "./reading-api";
import "./activity-page.css";

function time(value?: string) {
  return value ? new Date(value).toLocaleString("zh-CN") : "时间未记录";
}
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
              ? time(message.at)
              : message.observed_at
                ? "观察于 " + time(message.observed_at)
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
                    ? "结束于 " + time(activity.finished_at)
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
  const [open, setOpen] = useState(false);
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
    refetchOnWindowFocus: false,
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
          {activity.source === "task_candidate" &&
            activity.execution_ref?.run_id &&
            activity.execution_ref.attempt_id && (
              <CandidateConversation
                key={JSON.stringify([
                  activity.activity_id,
                  activity.execution_ref.run_id,
                  activity.execution_ref.attempt_id,
                ])}
                activityID={activity.activity_id}
                runID={activity.execution_ref.run_id}
                attemptID={activity.execution_ref.attempt_id}
              />
            )}
          <Button aria-expanded={open} onClick={() => setOpen(!open)}>
            {open ? "收起已记录交互" : "查看已记录交互"}
          </Button>
          {open && <Interactions activity={activity} />}
        </>
      )}
    </section>
  );
}

function Interactions({ activity }: { activity: Activity }) {
  const { api, online, identity } = useWorkspace();
  const client = useQueryClient();
  const scope = [
    api.home,
    identity.project_id,
    identity.instance_id,
    activity.activity_id,
  ];
  const key = [...scope, "reading-index"];
  const active = useQueryVisibility(online, key);
  const firstKey = [...scope, "reading-first"];
  const firstActive = useQueryVisibility(online, firstKey);
  const completedReads = useRef(0);
  const refreshGeneration = useRef(0);
  const mounted = useRef(true);
  const activeNow = useRef(firstActive);
  activeNow.current = firstActive;
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      refreshGeneration.current++;
    };
  }, []);
  useEffect(() => {
    if (!firstActive) refreshGeneration.current++;
  }, [firstActive]);
  const firstRead = useQuery({
    queryKey: firstKey,
    queryFn: async ({ signal }) => {
      const page = await readInteractions(
        api,
        activity.activity_id,
        "",
        signal,
      );
      if (signal.aborted) throw new Error("交互读取已取消");
      completedReads.current++;
      return page;
    },
    enabled: firstActive,
    retry: false,
    staleTime: "static",
    refetchOnMount: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const pages = useInfiniteQuery({
    queryKey: key,
    initialPageParam: "",
    queryFn: ({ signal, pageParam }) =>
      readInteractions(api, activity.activity_id, pageParam, signal),
    getNextPageParam: (last) => (last.has_more ? last.next_cursor : undefined),
    enabled: false,
    retry: false,
    refetchOnMount: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const index = {
    ...pages,
    error: firstRead.error || pages.error,
    isFetching: firstRead.isFetching || pages.isFetching,
  };
  const [bodyEpoch, setBodyEpoch] = useState(0);
  useEffect(() => {
    if (firstRead.data)
      client.setQueryData<typeof pages.data>(key, (old) =>
        old ? old : { pages: [firstRead.data!], pageParams: [""] },
      );
  }, [firstRead.data]);
  const first = index.data?.pages[0];
  const changesKey = [...scope, "reading-changes"];
  const changeActive = useQueryVisibility(
    online && !ended(activity) && !!first,
    changesKey,
  );
  const changes = useQuery({
    queryKey: changesKey,
    queryFn: ({ signal }) =>
      api.get<{ version: string }>(
        readingRoot(activity.activity_id) + "/changes",
        signal,
      ),
    enabled: changeActive,
    retry: false,
    refetchInterval: changeActive ? 5000 : false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  const firstVersion = first?.version;
  useEffect(() => {
    if (firstVersion)
      client.setQueryData(changesKey, { version: firstVersion });
  }, [firstVersion]);
  const items = [
    ...new Map(
      index.data?.pages
        .flatMap((page) => page.items)
        .map((item) => [item.id, item]),
    ).values(),
  ];
  return (
    <section className="reading-interactions" aria-label="已记录的交互">
      <p className="muted">
        {first?.diagnostic ||
          "只读取已保存的用户输入、问题上下文和回复结果，不是完整原生会话。"}
      </p>
      <p className="muted">
        本次读取快照 ·{" "}
        {index.dataUpdatedAt
          ? time(new Date(index.dataUpdatedAt).toISOString())
          : "尚未读取"}
      </p>
      {changes.data && first && changes.data.version !== first.version && (
        <p role="status">有更新，刷新后读取新快照。</p>
      )}
      {changes.error && <p className="muted">变化检查暂不可用，可手动刷新。</p>}
      <Button
        size="sm"
        disabled={!online || index.isFetching}
        onClick={() => {
          const generation = ++refreshGeneration.current;
          const completed = completedReads.current;
          void firstRead.refetch().then((result) => {
            if (
              mounted.current &&
              activeNow.current &&
              generation === refreshGeneration.current &&
              completedReads.current > completed &&
              result.isSuccess
            ) {
              client.setQueryData(key, {
                pages: [result.data],
                pageParams: [""],
              });
              setBodyEpoch((epoch) => epoch + 1);
            }
          });
        }}
      >
        刷新交互
      </Button>
      {index.error && (
        <p className="notice amber">
          交互读取失败，保留上次快照。{String(index.error)}
        </p>
      )}
      {!items.length && (
        <p className="muted">
          {index.isPending && online
            ? "正在读取交互索引…"
            : "此活动暂无已记录的交互正文"}
        </p>
      )}
      {items.map((item) => (
        <ReadingMessage
          key={bodyEpoch + "/" + item.id + "/" + item.sha256}
          activityID={activity.activity_id}
          item={item}
        />
      ))}
      {index.hasNextPage && (
        <Button
          disabled={!online || index.isFetchingNextPage}
          onClick={() => void index.fetchNextPage()}
        >
          加载更多交互
        </Button>
      )}
    </section>
  );
}

function ReadingMessage({
  activityID,
  item,
}: {
  activityID: string;
  item: ReadingItem;
}) {
  const { api, online, identity } = useWorkspace();
  const [open, setOpen] = useState(false);
  const [offset, setOffset] = useState(0);
  const [chunks, setChunks] = useState<Record<number, ReadingBody>>({});
  const fullPreview =
    new TextEncoder().encode(item.preview).length === item.bytes;
  const key = [
    api.home,
    identity.project_id,
    identity.instance_id,
    activityID,
    "reading-body",
    item.id,
    item.sha256,
    offset,
  ];
  const enabled = useQueryVisibility(online && open && !fullPreview, key);
  const body = useQuery({
    queryKey: key,
    queryFn: ({ signal }) => readBody(api, activityID, item, offset, signal),
    enabled,
    retry: false,
    staleTime: "static",
    refetchOnMount: false,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
  });
  useEffect(() => {
    if (body.data)
      setChunks((old) => ({ ...old, [body.data.offset]: body.data }));
  }, [body.data]);
  const pages = Object.values(chunks).sort((a, b) => a.offset - b.offset);
  const last = pages.at(-1);
  return (
    <article className={"reading-interaction reading-role-" + item.role}>
      <header>
        <strong>{item.label}</strong>
        <small>
          {item.group} · {time(item.at)}
          {item.state ? " · " + item.state : ""}
        </small>
      </header>
      <pre>
        {fullPreview
          ? item.preview
          : open && pages.length
            ? pages.map((page) => page.text).join("")
            : item.preview}
      </pre>
      {!fullPreview && (
        <Button
          size="sm"
          variant="ghost"
          aria-expanded={open}
          onClick={() => setOpen(!open)}
        >
          {open ? "收起正文" : "展开正文"}
        </Button>
      )}
      {open && body.error && (
        <p className="notice amber">
          正文读取失败，保留已读分段。{String(body.error)}
          <Button
            size="sm"
            disabled={!online || body.isFetching}
            onClick={() => void body.refetch()}
          >
            重试当前分段
          </Button>
        </p>
      )}
      {open && body.isFetching && <p className="muted">正在读取正文分段…</p>}
      {open && last && !last.complete && (
        <Button
          size="sm"
          disabled={!online || body.isFetching || body.isError}
          onClick={() => setOffset(last.next_offset)}
        >
          继续读取正文
        </Button>
      )}
      {open && last?.complete && <small>此条正文完整 · 不代表完整会话</small>}
    </article>
  );
}
