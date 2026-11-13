import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import { projectQueries } from "../../shared/api/queries";
import { useQueryVisibility } from "../../shared/query-visibility";
import type { Candidate } from "../../shared/api/types";
import { sameNotice } from "./presentation";

function age(at: string, now: number) {
  const seconds = Math.max(0, Math.floor((now - Date.parse(at)) / 1000));
  return seconds < 60
    ? `${seconds} 秒前`
    : seconds < 3600
      ? `${Math.floor(seconds / 60)} 分钟前`
      : `${Math.floor(seconds / 3600)} 小时前`;
}

// Mounted only for the selected execution. Visibility controls requests as well
// as timers, including cancellation of an already in-flight request.
export function LastMessage({
  run,
  answerVisible = !!run.decision,
}: {
  run: Candidate;
  answerVisible?: boolean;
}) {
  const { api, online } = useWorkspace();
  const [now, setNow] = useState(Date.now());
  const options = projectQueries(api).lastMessage(run);
  const enabled = useQueryVisibility(
    online && !!run.execution_id && !!run.attempt_id,
    options.queryKey,
  );
  const message = useQuery({
    ...options,
    enabled,
    refetchInterval: 3000,
    refetchIntervalInBackground: false,
  });
  useEffect(() => {
    if (!enabled) return;
    const timer = window.setInterval(() => setNow(Date.now()), 3000);
    return () => window.clearInterval(timer);
  }, [enabled]);
  const sample = message.data?.message;
  const body = useRef<HTMLPreElement>(null);
  const [expanded, setExpanded] = useState(false);
  const [overflow, setOverflow] = useState(false);
  useEffect(() => {
    const measure = () => {
      const node = body.current;
      if (node && !expanded)
        setOverflow(
          node.scrollHeight > node.clientHeight ||
            (sample?.text.split("\n").length ?? 0) > 6,
        );
    };
    measure();
    const observer =
      typeof ResizeObserver !== "undefined"
        ? new ResizeObserver(measure)
        : undefined;
    if (body.current) observer?.observe(body.current);
    window.addEventListener("resize", measure);
    return () => {
      observer?.disconnect();
      window.removeEventListener("resize", measure);
    };
  }, [sample?.text, expanded]);
  return (
    <section className="last-message notice" aria-label="最近消息">
      <div className="last-message-header">
        <strong>最后一条消息</strong>
        {sample && (
          <p className="muted">
            <time dateTime={sample.observed_at}>
              观察于{" "}
              {new Date(sample.observed_at).toLocaleString("zh-CN", {
                hour12: false,
              })}
            </time>{" "}
            · {age(sample.observed_at, now)}
          </p>
        )}
      </div>
      {sample ? (
        <>
          <pre ref={body} className={expanded ? "" : "message-preview"}>
            {answerVisible &&
            [
              run.decision?.request.question,
              run.decision?.request.context,
            ].some((text) => sameNotice(text, sample.text))
              ? "与上方原问题相同，请在回答区查看。"
              : sample.text}
          </pre>
          {(overflow || expanded) && (
            <button
              onClick={() => setExpanded(!expanded)}
              aria-expanded={expanded}
            >
              {expanded ? "收起消息" : "展开完整消息"}
            </button>
          )}
        </>
      ) : (
        <p className="muted">
          {message.isPending && enabled
            ? "正在读取最近消息…"
            : "暂无已观察到的消息"}
        </p>
      )}
      {message.error && (
        <p className="notice amber">
          消息读取失败，保留最后观察：{String(message.error)}
        </p>
      )}
      {!online && (
        <p className="muted">服务离线 · 上次已知消息，已暂停刷新。</p>
      )}
    </section>
  );
}
