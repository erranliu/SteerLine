import { useState } from "react";
import {
  useInfiniteQuery,
  useQuery,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import { useQueryVisibility } from "../../shared/query-visibility";
import { Button } from "../../shared/ui/button";
import type { Api } from "../../shared/api/client";
import type { Activity } from "./model";
import { readingRoot } from "./reading-api";
import { RecordedInteractions } from "./recorded-interactions";

type ConversationPage = {
  activity_id: string;
  target_id: string;
  run_id: string;
  attempt_id: string;
  session_id: string;
  generation: string;
  next_cursor?: string;
  partial: boolean;
  body_offset?: number;
  messages: {
    id: string;
    role: "user" | "assistant";
    text: string;
    truncated?: boolean;
    body_cursor?: string;
  }[];
};

type ConversationTarget = {
  id: string;
  label: string;
  run_id?: string;
  attempt_id?: string;
  state: "available" | "unbound" | "unavailable" | "unsupported";
};

type ConversationTargets = {
  activity_id: string;
  targets: ConversationTarget[];
  diagnostic?: string;
  operation?: {
    label: string;
    request_id: string;
    status: string;
    diagnostic?: string;
  };
};

const targetDiagnostic = {
  unbound: "尚无可核实的原会话绑定。",
  unavailable: "原 Runner 状态目录或当前实例暂不可读。",
  unsupported: "当前 Runner 构建没有公开原会话历史。",
};

function getConversation(
  api: Api,
  activityID: string,
  targetID: string,
  cursor: string,
  signal: AbortSignal,
) {
  const params = new URLSearchParams({ session: targetID });
  if (cursor) params.set("cursor", cursor);
  return api.get<ConversationPage>(
    readingRoot(activityID) + "/conversation?" + params.toString(),
    signal,
  );
}

export function ActivityConversation({ activity }: { activity: Activity }) {
  const [open, setOpen] = useState(false);
  return (
    <div className="reading-conversation">
      <Button
        variant={open || activity.needs_attention ? "outline" : "default"}
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        {open ? "收起对话" : "查看对话"}
      </Button>
      {open && <ConversationPanel activity={activity} />}
    </div>
  );
}

function ConversationPanel({ activity }: { activity: Activity }) {
  const { api, identity, online } = useWorkspace();
  const [selectedID, setSelectedID] = useState("");
  const [revision, setRevision] = useState(0);
  const activityID = activity.activity_id;
  const runID = activity.execution_ref?.run_id;
  const attemptID = activity.execution_ref?.attempt_id;
  const key = [
    api.home,
    identity.project_id,
    identity.instance_id,
    activityID,
    "activity-conversation-targets",
    runID,
    attemptID,
    revision,
  ];
  const active = useQueryVisibility(online, key);
  const result = useQuery({
    queryKey: key,
    queryFn: async ({ signal }) => {
      const list = await api.get<ConversationTargets>(
        readingRoot(activityID) + "/conversation",
        signal,
      );
      if (list.activity_id !== activityID || !Array.isArray(list.targets)) {
        throw new Error("会话列表与原活动不符，请刷新");
      }
      return list;
    },
    enabled: active,
    staleTime: Infinity,
    retry: false,
    refetchOnReconnect: false,
    refetchOnWindowFocus: false,
  });
  const targets = result.data?.targets ?? [];
  const target =
    targets.find((item) => item.id === selectedID) ??
    targets.find(
      (item) =>
        runID &&
        item.run_id === runID &&
        (!attemptID || item.attempt_id === attemptID),
    ) ??
    targets.at(-1);
  return (
    <section className="reading-interactions" aria-label="活动对话">
      {!online && <p role="status">连接后可继续读取对话。</p>}
      {result.isFetching && <p role="status">正在读取会话列表…</p>}
      {result.error && <p role="alert">{result.error.message}</p>}
      {result.data?.diagnostic && <p>{result.data.diagnostic}</p>}
      {result.data?.operation && (
        <>
          <p>
            {result.data.operation.label} ·{" "}
            {activity.phase || result.data.operation.status}
          </p>
          <p>原请求 {result.data.operation.request_id}</p>
          {result.data.operation.diagnostic && (
            <p>{result.data.operation.diagnostic}</p>
          )}
        </>
      )}
      {result.data && !targets.length && !result.data.diagnostic && (
        <p>暂无可定位的原会话。</p>
      )}
      {targets.length > 1 && (
        <label className="reading-session">
          执行会话{" "}
          <select
            value={target?.id}
            onChange={(event) => setSelectedID(event.target.value)}
          >
            {targets.map((item) => (
              <option value={item.id} key={item.id}>
                {item.label}
              </option>
            ))}
          </select>
        </label>
      )}
      {target?.state === "available" ? (
        <Conversation
          key={target.id}
          activityID={activityID}
          target={target}
          revision={revision}
          refresh={() => setRevision((value) => value + 1)}
        />
      ) : (
        <>
          {target && <p>{targetDiagnostic[target.state]}</p>}
          <Button
            variant="ghost"
            disabled={!active || result.isFetching}
            onClick={() => setRevision((value) => value + 1)}
          >
            刷新会话列表
          </Button>
        </>
      )}
      <RecordedInteractions activity={activity} />
    </section>
  );
}

function Conversation({
  activityID,
  target,
  revision,
  refresh,
}: {
  activityID: string;
  target: ConversationTarget;
  revision: number;
  refresh: () => void;
}) {
  const { api, identity, online } = useWorkspace();
  const client = useQueryClient();
  const key = [
    api.home,
    identity.project_id,
    identity.instance_id,
    activityID,
    "activity-conversation",
    target.id,
    revision,
  ];
  const active = useQueryVisibility(online, key);
  const history = useInfiniteQuery({
    queryKey: key,
    enabled: active,
    initialPageParam: "",
    queryFn: async ({ pageParam, signal }): Promise<ConversationPage> => {
      const page = await getConversation(
        api,
        activityID,
        target.id,
        pageParam,
        signal,
      );
      const first =
        client.getQueryData<InfiniteData<ConversationPage>>(key)?.pages[0];
      if (
        page.activity_id !== activityID ||
        page.target_id !== target.id ||
        page.run_id !== target.run_id ||
        page.attempt_id !== target.attempt_id ||
        !page.session_id ||
        !page.generation ||
        !Array.isArray(page.messages) ||
        page.body_offset !== undefined ||
        (pageParam &&
          first &&
          (page.session_id !== first.session_id ||
            page.generation !== first.generation))
      ) {
        throw new Error("对话与原执行会话不符，请刷新");
      }
      return page;
    },
    getNextPageParam: (last) => last.next_cursor || undefined,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    retry: false,
  });
  const pages = history.data?.pages ?? [];
  const messages = new Map<string, ConversationPage["messages"][number]>();
  const first = pages[0];
  for (const page of [...pages].reverse()) {
    for (const message of page.messages) messages.set(message.id, message);
  }
  return (
    <section className="reading-interactions" aria-label="原会话对话">
      {history.isFetching && <p role="status">正在读取对话…</p>}
      {history.error && (
        <p role="alert">
          {history.error.message}。可重试，也可查看已记录交互。
        </p>
      )}
      {pages.some((page) => page.partial) && (
        <p className="notice amber">原会话仅有部分历史可读。</p>
      )}
      {pages.length > 0 && !messages.size && (
        <p className="muted">
          {history.hasNextPage
            ? "这一页没有可读消息。"
            : "未读到可显示的对话消息。"}
        </p>
      )}
      {history.hasNextPage && (
        <Button
          variant="ghost"
          disabled={!active || history.isFetching}
          onClick={() => void history.fetchNextPage()}
        >
          {history.isFetchNextPageError ? "重试读取更早消息" : "加载更早消息"}
        </Button>
      )}
      {[...messages.values()].map((message) => (
        <article
          key={message.id}
          className={
            message.role === "user"
              ? "reading-interaction reading-role-user"
              : "reading-interaction"
          }
        >
          <strong className="reading-role">
            {message.role === "user" ? "用户" : "助手"}
          </strong>
          {message.body_cursor && first ? (
            <ConversationBody
              key={JSON.stringify([
                ...key,
                first.session_id,
                first.generation,
                message.body_cursor,
              ])}
              scope={key}
              binding={first}
              message={message}
            />
          ) : (
            <>
              <p className="reading-conversation-text">{message.text}</p>
              {message.truncated && <small>此条消息正文有截断。</small>}
            </>
          )}
        </article>
      ))}
      <Button
        variant="ghost"
        disabled={!active || history.isFetching}
        onClick={refresh}
      >
        {history.isLoadingError ? "重试读取对话" : "刷新对话"}
      </Button>
    </section>
  );
}

function ConversationBody({
  scope,
  binding,
  message,
}: {
  scope: readonly unknown[];
  binding: ConversationPage;
  message: ConversationPage["messages"][number];
}) {
  const { api, online } = useWorkspace();
  const client = useQueryClient();
  const [open, setOpen] = useState(false);
  const key = [
    ...scope,
    binding.session_id,
    binding.generation,
    "body",
    message.body_cursor,
  ];
  const active = useQueryVisibility(online && open, key);
  const body = useInfiniteQuery({
    queryKey: key,
    enabled: active,
    initialPageParam: message.body_cursor || "",
    queryFn: async ({ pageParam, signal }): Promise<ConversationPage> => {
      const page = await getConversation(
        api,
        binding.activity_id,
        binding.target_id,
        pageParam,
        signal,
      );
      const last = client
        .getQueryData<InfiniteData<ConversationPage>>(key)
        ?.pages.at(-1);
      const offset =
        pageParam === message.body_cursor || !last
          ? 0
          : (last.body_offset ?? 0) +
            new TextEncoder().encode(last.messages[0].text).length;
      if (
        page.activity_id !== binding.activity_id ||
        page.target_id !== binding.target_id ||
        page.run_id !== binding.run_id ||
        page.attempt_id !== binding.attempt_id ||
        page.session_id !== binding.session_id ||
        page.generation !== binding.generation ||
        page.body_offset !== offset ||
        page.messages?.length !== 1 ||
        page.messages[0].id !== message.id ||
        page.messages[0].role !== message.role
      ) {
        throw new Error("消息正文与原会话不符，请刷新对话");
      }
      return page;
    },
    getNextPageParam: (last) => last.next_cursor || undefined,
    staleTime: Infinity,
    refetchOnWindowFocus: false,
    refetchOnReconnect: false,
    retry: false,
  });
  const text =
    open && body.data
      ? body.data.pages.map((page) => page.messages[0].text).join("")
      : message.text;
  return (
    <>
      <p className="reading-conversation-text">{text}</p>
      <Button
        variant="ghost"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
      >
        {open ? "收起消息正文" : "展开消息正文"}
      </Button>
      {open && body.isFetching && <p role="status">正在读取消息正文…</p>}
      {open && body.error && <p role="alert">{body.error.message}</p>}
      {open && body.isLoadingError && (
        <Button
          variant="ghost"
          disabled={!active}
          onClick={() => void body.refetch()}
        >
          重试读取正文
        </Button>
      )}
      {open && body.hasNextPage && (
        <Button
          variant="ghost"
          disabled={!active || body.isFetching}
          onClick={() => void body.fetchNextPage()}
        >
          继续读取消息正文
        </Button>
      )}
    </>
  );
}
