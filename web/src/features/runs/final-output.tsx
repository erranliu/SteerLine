import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import { projectQueries } from "../../shared/api/queries";
import type { Candidate } from "../../shared/api/types";
import { executionEnded } from "./presentation";
export function FinalOutput({ run }: { run: Candidate }) {
  const { api, online } = useWorkspace();
  const [choice, setChoice] = useState<boolean>();
  const open = choice ?? executionEnded(run);
  const final = useQuery({
    ...projectQueries(api).final(run),
    enabled: online && open && !!run.execution_id && !!run.attempt_id,
    refetchInterval: 5000,
  });
  return (
    <div className="final-output">
      <button onClick={() => setChoice(!open)} aria-expanded={open}>
        最终回复
      </button>
      {run.runner_delivery?.available &&
        !run.runner_delivery.final_verified && (
          <p className="muted">本轮尚未取得完整最终回复。</p>
        )}
      {open && (
        <div className="final-body">
          <p className="output-hint">回复历史，包含先前轮次。</p>
          {(!online || final.error) && (
            <p className="notice amber">
              回复暂不可核实，保留上次已知内容。
              {final.error ? String(final.error) : ""}
              {final.dataUpdatedAt ? (
                <time dateTime={new Date(final.dataUpdatedAt).toISOString()}>
                  最后成功读取于{" "}
                  {new Date(final.dataUpdatedAt).toLocaleString("zh-CN", {
                    hour12: false,
                  })}
                </time>
              ) : (
                "尚无可信读取时间"
              )}
            </p>
          )}
          {final.data?.page.events
            .filter((e) => e.type === "gap" || e.diagnostic)
            .map((e, i) => (
              <p className="notice amber" key={i}>
                会话采集缺口：{e.diagnostic || "来源未完整确认"}
              </p>
            ))}
          {final.data?.bodies.map((body) => (
            <article key={body.item_key + "/" + body.revision}>
              {final.data.bodies.length === 1 ? (
                <>
                  <p className="muted">
                    {body.complete
                      ? "正文完整"
                      : "正文不完整：" + body.completeness}
                  </p>
                  <pre>{body.text}</pre>
                </>
              ) : (
                <details>
                  <summary>
                    最终回复 ·{" "}
                    {body.complete
                      ? "正文完整"
                      : "正文不完整：" + body.completeness}
                  </summary>
                  <pre>{body.text}</pre>
                </details>
              )}
            </article>
          ))}
          {final.data?.page.has_more && (
            <p className="notice amber">最终事件页未完整，请继续读取原会话。</p>
          )}
          {!final.data?.bodies.length && (
            <p className="muted">
              {final.isPending && online
                ? "正在读取最终回复…"
                : "尚无可用最终回复"}
            </p>
          )}
        </div>
      )}
    </div>
  );
}
