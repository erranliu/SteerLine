import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import { projectQueries } from "../../shared/api/queries";
import type { Candidate } from "../../shared/api/types";
export function RecentActivity({ run }: { run: Candidate }) {
  const { api, online } = useWorkspace();
  const [expanded, setExpanded] = useState(false);
  const activity = useQuery({
    ...projectQueries(api).activity(run),
    enabled: online && !!run.execution_id && !!run.attempt_id,
    refetchInterval: 2500,
  });
  const events = [...(activity.data?.page.activity ?? [])]
    .filter((e) => e.kind !== "runner_event")
    .sort((a, b) => b.seq - a.seq);
  return (
    <div className="activity-list">
      {(!online || activity.error) && (
        <p className="notice amber">
          活动暂不可核实，保留上次已知状态。
          {activity.error ? String(activity.error) : ""}
        </p>
      )}
      {activity.data?.page.diagnostics?.map((d, i) => (
        <p className="notice amber" key={i}>
          {d}
        </p>
      ))}
      {(expanded ? events : events.slice(0, 5)).map((event) => (
        <div className="activity-item" key={event.seq}>
          <span className="activity-dot" />
          <time dateTime={event.ts}>
            {new Date(event.ts).toLocaleTimeString("zh-CN")}
          </time>
          <div>
            {event.detail ? (
              <details>
                <summary>{event.title || "状态变化"}</summary>
                <pre>{event.detail}</pre>
              </details>
            ) : (
              <strong>{event.title || "状态变化"}</strong>
            )}
          </div>
        </div>
      ))}
      {!events.length && (
        <p className="muted">
          {activity.isPending && online
            ? "正在读取近期活动…"
            : "暂无已记录的阶段或状态变化"}
        </p>
      )}
      {events.length > 5 && (
        <button onClick={() => setExpanded(!expanded)} aria-expanded={expanded}>
          {expanded ? "收起活动" : "更多活动"}
        </button>
      )}
    </div>
  );
}
