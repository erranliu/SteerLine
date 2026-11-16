import { useEffect, useRef, useState } from "react";

type ConversationEvent = {
  id: string;
  turn_id: string;
  item_id?: string;
  kind:
    | "assistant_started"
    | "assistant_delta"
    | "assistant_completed"
    | "tool"
    | "turn_settled";
  text?: string;
  tool?: string;
  status?: string;
};

type ConversationPage = {
  events: ConversationEvent[];
  next_cursor?: string;
  has_more: boolean;
  active: boolean;
};

type PlanningMessage = {
  text: string;
  complete: boolean;
  started?: boolean;
  mirrored?: boolean;
  outstanding?: string[];
};
type PlanningTool = { tool: string; status: string };
export type PlanningTurn = {
  messages: Map<string, PlanningMessage>;
  tools: Map<string, PlanningTool>;
  outcome?: string;
};
const emptyTurns = new Map<string, PlanningTurn>();

function groupEvents(
  previous: Map<string, PlanningTurn> | undefined,
  events: ConversationEvent[],
) {
  const turns = new Map(previous);
  const edited = new Set<string>();
  for (const event of events) {
    if (!edited.has(event.turn_id)) {
      const old = turns.get(event.turn_id);
      turns.set(event.turn_id, {
        messages: new Map(old?.messages),
        tools: new Map(old?.tools),
        outcome: old?.outcome,
      });
      edited.add(event.turn_id);
    }
    const turn = turns.get(event.turn_id)!;
    const item = event.item_id;
    if (event.kind === "turn_settled") {
      turn.outcome = event.status;
    } else if (event.kind === "tool" && item && event.tool && event.status) {
      const previous = turn.tools.get(item);
      if (event.status !== "running" || !previous) {
        turn.tools.set(item, { tool: event.tool, status: event.status });
      }
    } else if (item && event.kind === "assistant_started") {
      const message = turn.messages.get(item);
      if (!message?.complete) {
        turn.messages.set(item, {
          text: message?.text ?? "",
          complete: false,
          started: true,
          mirrored: message?.started,
          outstanding: message?.outstanding,
        });
      }
    } else if (item && event.kind === "assistant_completed") {
      turn.messages.set(item, { text: event.text ?? "", complete: true });
    } else if (item && event.kind === "assistant_delta" && event.text) {
      const previous = turn.messages.get(item);
      if (!previous?.complete) {
        const waiting = previous?.outstanding ?? [];
        // A repeated native item-start identifies the mirrored Codex feed.
        // The completed item always supplies the authoritative text.
        const echo = previous?.mirrored && waiting[0] === event.text;
        turn.messages.set(item, {
          ...previous,
          text: (previous?.text ?? "") + (echo ? "" : event.text),
          complete: false,
          outstanding: previous?.mirrored
            ? echo
              ? waiting.slice(1)
              : [...waiting.slice(-31), event.text]
            : undefined,
        });
      }
    }
  }
  return turns;
}

export function usePlanningEvents(
  key: string,
  enabled: boolean,
  api: { get<T>(path: string, signal?: AbortSignal): Promise<T> },
  route: string,
) {
  const [state, setState] = useState<{
    key: string;
    turns: Map<string, PlanningTurn>;
    failed: boolean;
  }>();
  const position = useRef<{ key: string; cursor: string }>(undefined);
  const [visible, setVisible] = useState(() => !document.hidden);
  useEffect(() => {
    const refresh = () => setVisible(!document.hidden);
    document.addEventListener("visibilitychange", refresh);
    return () => document.removeEventListener("visibilitychange", refresh);
  }, []);

  useEffect(() => {
    if (!enabled || !visible) return;
    const controller = new AbortController();
    if (position.current?.key !== key) position.current = { key, cursor: "" };
    const checkpoint = position.current;
    let timer: ReturnType<typeof setTimeout>;
    const poll = async () => {
      let delay = 1500;
      try {
        const page = await api.get<ConversationPage>(
          route +
            (checkpoint.cursor
              ? "?after=" + encodeURIComponent(checkpoint.cursor)
              : ""),
          controller.signal,
        );
        if (controller.signal.aborted) return;
        checkpoint.cursor = page.next_cursor ?? checkpoint.cursor;
        delay = page.has_more ? 0 : page.active ? 650 : 2000;
        setState((previous) => {
          if (!page.events.length && previous?.key === key && !previous.failed)
            return previous;
          return {
            key,
            turns: groupEvents(
              previous?.key === key ? previous.turns : undefined,
              page.events,
            ),
            failed: false,
          };
        });
      } catch {
        if (controller.signal.aborted) return;
        setState((previous) => ({
          key,
          turns: previous?.key === key ? previous.turns : new Map(),
          failed: true,
        }));
      }
      if (!controller.signal.aborted)
        timer = setTimeout(() => void poll(), delay);
    };
    void poll();
    return () => {
      controller.abort();
      clearTimeout(timer);
    };
  }, [api, enabled, key, route, visible]);

  const current = state?.key === key ? state : undefined;
  return { turns: current?.turns ?? emptyTurns, failed: current?.failed };
}

const toolNames: Record<string, string> = {
  commandExecution: "运行命令",
  fileChange: "修改文件",
  mcpToolCall: "调用 MCP 工具",
  dynamicToolCall: "调用工具",
  webSearch: "搜索",
};
const toolStatuses: Record<string, string> = {
  running: "进行中",
  completed: "已完成",
  failed: "失败",
  cancelled: "已停止",
  unconfirmed: "结束状态未确认",
};

export function PlanningTurnActivity({
  turn,
  finalItem,
}: {
  turn: PlanningTurn;
  finalItem?: string;
}) {
  return (
    <>
      {Array.from(turn.messages, ([id, message]) =>
        id !== finalItem && message.text ? (
          <p
            key={id}
            data-planning-reply={message.complete ? "complete" : "streaming"}
          >
            {message.text}
          </p>
        ) : null,
      )}
      {turn.tools.size > 0 && (
        <details className="planning-tool-activity">
          <summary>工具活动 · {turn.tools.size}</summary>
          <ul>
            {Array.from(turn.tools, ([id, tool]) => {
              const status =
                tool.status === "running" && turn.outcome
                  ? turn.outcome === "cancelled"
                    ? "cancelled"
                    : "unconfirmed"
                  : tool.status;
              return (
                <li key={id}>
                  {toolNames[tool.tool] ?? "调用工具"} ·{" "}
                  {toolStatuses[status] ?? toolStatuses.unconfirmed}
                </li>
              );
            })}
          </ul>
        </details>
      )}
    </>
  );
}
