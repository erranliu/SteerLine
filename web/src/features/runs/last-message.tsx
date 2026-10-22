import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import { projectQueries } from "../../shared/api/queries";
import { useQueryVisibility } from "../../shared/query-visibility";
import type { Candidate } from "../../shared/api/types";

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
export function LastMessage({ run }: { run: Candidate }) {
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
  return (
    <section className="last-message notice" aria-label="最近消息">
      <strong>最后一条消息</strong>
      {sample ? (
        <>
          <p className="muted">
            <time dateTime={sample.observed_at}>
              观察于{" "}
              {new Date(sample.observed_at).toLocaleString("zh-CN", {
                hour12: false,
              })}
            </time>{" "}
            · {age(sample.observed_at, now)}
          </p>
          <pre>{sample.text}</pre>
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
      <p className="output-hint">
        活动参考，可能遗漏消息；长时间无新消息不表示执行已卡住。
        {!online ? "服务离线，已暂停刷新。" : ""}
      </p>
    </section>
  );
}
