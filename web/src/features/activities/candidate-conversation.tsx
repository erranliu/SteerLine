import { useState } from "react";
import {
  useInfiniteQuery,
  useQueryClient,
  type InfiniteData,
} from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import { useQueryVisibility } from "../../shared/query-visibility";
import { Button } from "../../shared/ui/button";
import type { Api } from "../../shared/api/client";
import { readingRoot } from "./reading-api";

type ConversationPage = {
  activity_id: string;
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

function getConversation(
  api: Api,
  activityID: string,
  cursor: string,
  signal: AbortSignal,
) {
  const path = readingRoot(activityID) + "/conversation";
  return api.get<ConversationPage>(
    cursor ? path + "?cursor=" + encodeURIComponent(cursor) : path,
    signal,
  );
}

export function CandidateConversation({
  activityID,
  runID,
  attemptID,
}: {
  activityID: string;
  runID: string;
  attemptID: string;
}) {
  const [open, setOpen] = useState(false);
  const [revision, setRevision] = useState(0);
  return (
    <>
      <Button aria-expanded={open} onClick={() => setOpen(!open)}>
        {open ? "收起对话" : "查看对话"}
      </Button>
      {open && (
        <Conversation
          activityID={activityID}
          runID={runID}
          attemptID={attemptID}
          revision={revision}
          refresh={() => setRevision((value) => value + 1)}
        />
      )}
    </>
  );
}

function Conversation({
  activityID,
  runID,
  attemptID,
  revision,
  refresh,
}: {
  activityID: string;
  runID: string;
  attemptID: string;
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
    runID,
    attemptID,
    "candidate-conversation",
    revision,
  ];
  const active = useQueryVisibility(online, key);
  const history = useInfiniteQuery({
    queryKey: key,
    enabled: active,
    initialPageParam: "",
    queryFn: async ({ pageParam, signal }): Promise<ConversationPage> => {
      const page = await getConversation(api, activityID, pageParam, signal);
      const first =
        client.getQueryData<InfiniteData<ConversationPage>>(key)?.pages[0];
      if (
        page.activity_id !== activityID ||
        page.run_id !== runID ||
        page.attempt_id !== attemptID ||
        !page.session_id ||
        !page.generation ||
        !Array.isArray(page.messages) ||
        page.body_offset !== undefined ||
        (pageParam &&
          first &&
          (page.session_id !== first.session_id ||
            page.generation !== first.generation))
      ) {
        throw new Error("对话与原候选会话不符，请刷新");
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
      {!online && <p role="status">连接后可继续读取对话。</p>}
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
          <small>{message.role === "user" ? "用户" : "助手"}</small>
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
