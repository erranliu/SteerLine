import { useEffect, useState } from "react";
import { useLocation } from "react-router-dom";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import type { Candidate } from "../../shared/api/types";
import { Button } from "../../shared/ui/button";

type Material = {
  id: string;
  label: string;
  body: string;
  source?: string;
  revision?: string;
};
export type AdvisoryDetail = {
  id: string;
  revision: number;
  candidate_id: string;
  occurrence: string;
  state: string;
  settled: boolean;
  diagnostic?: string;
  prompt?: string;
  context?: { goal: string; user_input: string; materials: Material[] };
};
type Summary = { id: string; state: string; diagnostic?: string };
const states: Record<string, string> = {
  queued: "等待准备",
  preparing: "准备上下文",
  running: "生成提示词建议",
  unknown: "核实原生成请求",
  cancelling: "取消并结算中",
  settling: "等待资源结算",
  failed: "生成失败",
  ready: "建议已生成",
  cancelled: "已取消",
};
export function ContinuationSuggestion({
  run,
  inline = false,
  onSuggestion,
}: {
  run: Candidate;
  inline?: boolean;
  onSuggestion?: (record: AdvisoryDetail) => void;
}) {
  const { api, online, submit, busy } = useWorkspace();
  const client = useQueryClient();
  const requested =
    new URLSearchParams(useLocation().search).get("advisory") || "";
  const target = /^[a-f0-9]{32}$/.test(requested) ? requested : "";
  const [open, setOpen] = useState(!!target);
  useEffect(() => {
    if (target) setOpen(true);
  }, [target]);
  const [working, setWorking] = useState(false);
  const [message, setMessage] = useState("");
  const key = [
    api.home,
    api.identity.project_id,
    api.identity.instance_id,
    "advisory",
    run.run_id,
  ];
  const occurrence = run.attempt_id + ":" + run.decision?.id;
  const current = useQuery({
    queryKey: [...key, "current", occurrence],
    queryFn: ({ signal }) =>
      api.get<{ record: AdvisoryDetail | null }>(
        "/main-agent?" +
          new URLSearchParams({ candidate_id: run.run_id, occurrence }),
        signal,
      ),
    enabled: online && inline,
    retry: false,
    refetchInterval: (query) =>
      online &&
      inline &&
      (!query.state.data?.record?.settled ||
        ["queued", "preparing"].includes(query.state.data.record.state))
        ? 2500
        : false,
  });
  const currentRecord = current.data?.record;
  const matched =
    currentRecord?.candidate_id === run.run_id &&
    currentRecord.occurrence === occurrence
      ? currentRecord
      : undefined;
  useEffect(() => {
    if (matched?.state === "ready" && matched.settled && matched.prompt)
      onSuggestion?.(matched);
  }, [matched, onSuggestion]);
  const list = useQuery({
    queryKey: [...key, "list"],
    queryFn: ({ signal }) =>
      api.get<{ records: Summary[]; has_more: boolean }>(
        "/main-agent?candidate_id=" + encodeURIComponent(run.run_id),
        signal,
      ),
    enabled: online && open && !inline,
    retry: false,
    refetchInterval: online && open && !inline ? 2500 : false,
  });
  const id = target || list.data?.records[0]?.id;
  const detail = useQuery({
    queryKey: [...key, id],
    queryFn: ({ signal }) =>
      api.get<AdvisoryDetail>("/main-agent/" + id, signal),
    enabled: online && open && !!id && !inline,
    retry: false,
    refetchInterval: (query) =>
      online &&
      open &&
      query.state.data &&
      (!query.state.data.settled ||
        ["queued", "preparing"].includes(query.state.data.state))
        ? 2500
        : false,
  });
  const record = inline
    ? matched
    : detail.data?.candidate_id === run.run_id
      ? detail.data
      : undefined;
  async function copy() {
    if (!record?.prompt) return;
    try {
      await navigator.clipboard.writeText(record.prompt);
      setMessage("提示词已复制，可在外部修改和使用。");
    } catch {
      setMessage("复制未成功，请选择正文手动复制。");
    }
  }
  async function cancel() {
    if (!record) return;
    setWorking(true);
    try {
      const receipt = await submit(
        api.envelope("main_agent_cancel", record.id, record.revision, {}),
      );
      setMessage(
        receipt?.diagnostic ||
          (receipt?.status === "applied"
            ? "取消请求已保存"
            : "原取消结果待核实，请沿原请求查询。"),
      );
      await client.invalidateQueries({ queryKey: key });
    } catch (e) {
      setMessage(String(e));
    } finally {
      setWorking(false);
    }
  }
  const content = (
    <>
      {!inline && (
        <p className="muted">
          历史建议可查看、复制；当前问题的建议默认填入上方回答框。
        </p>
      )}
      {(current.error || list.error || detail.error) && (
        <p role="alert">
          {String(current.error || list.error || detail.error)}
        </p>
      )}
      {inline && !record && (
        <p className="muted">
          {current.isPending
            ? "正在读取当前问题的建议…"
            : "当前问题暂无建议，可直接填写回答。"}
        </p>
      )}
      {!inline && !id && !list.isLoading && <p>暂无已登记建议。</p>}
      {!inline && detail.data && !record && (
        <p role="alert">建议不属于当前候选，未显示其他范围的结果。</p>
      )}
      {record && (
        <article>
          <p className="muted">
            生成状态：{states[record.state] ?? "状态待核实"}
          </p>
          {record.diagnostic && <p role="alert">{record.diagnostic}</p>}
          {record.context && (
            <details>
              <summary>查看本次已提供的上下文</summary>
              <p>{record.context.goal}</p>
              {record.context.user_input && (
                <pre>{record.context.user_input}</pre>
              )}
              {record.context.materials.map((m) => (
                <section key={m.id}>
                  <h4>{m.label}</h4>
                  <p className="muted">
                    {m.source} {m.revision}
                  </p>
                  <pre>{m.body}</pre>
                </section>
              ))}
              {inline && record.prompt && (
                <Button onClick={() => void copy()}>复制提示词</Button>
              )}
            </details>
          )}
          {!inline && record.prompt && (
            <>
              <pre aria-label="提示词建议正文">{record.prompt}</pre>
              <Button onClick={() => void copy()}>复制提示词</Button>
            </>
          )}
          {["queued", "preparing", "running", "unknown", "settling"].includes(
            record.state,
          ) && (
            <Button
              variant="outline"
              disabled={!online || busy || working}
              onClick={() => void cancel()}
            >
              取消本次生成
            </Button>
          )}
        </article>
      )}
      <p role="status">{message}</p>
    </>
  );
  if (inline)
    return (
      <div className="answer-suggestion" aria-label="回答建议">
        {content}
        {target && target !== matched?.id && (
          <ContinuationSuggestion run={run} />
        )}
      </div>
    );
  return (
    <details
      className="detail-section"
      open={open}
      onToggle={(e) => setOpen(e.currentTarget.open)}
    >
      <summary>对话续作 · 提示词建议</summary>
      {open && content}
    </details>
  );
}
