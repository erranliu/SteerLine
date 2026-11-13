import { useEffect, useRef, useState } from "react";
import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import { useQueryVisibility } from "../../shared/query-visibility";
import { Button } from "../../shared/ui/button";
import { ended, formatActivityTime, type Activity } from "./model";
import {
  readBody,
  readInteractions,
  readingRoot,
  type ReadingBody,
  type ReadingItem,
} from "./reading-api";

export function RecordedInteractions({ activity }: { activity: Activity }) {
  const [open, setOpen] = useState(false);
  return (
    <>
      <Button aria-expanded={open} onClick={() => setOpen(!open)}>
        {open ? "收起已记录交互" : "查看已记录交互"}
      </Button>
      {open && <Interactions activity={activity} />}
    </>
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
          ? formatActivityTime(new Date(index.dataUpdatedAt).toISOString())
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
          {item.group} · {formatActivityTime(item.at)}
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
