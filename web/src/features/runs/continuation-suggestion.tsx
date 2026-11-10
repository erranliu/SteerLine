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
export function ContinuationSuggestion({ run }: { run: Candidate }) {
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
  const list = useQuery({
    queryKey: [...key, "list"],
    queryFn: ({ signal }) =>
      api.get<{ records: Summary[]; has_more: boolean }>(
        "/main-agent?candidate_id=" + encodeURIComponent(run.run_id),
        signal,
      ),
    enabled: online && open,
    retry: false,
    refetchInterval: online && open ? 2500 : false,
  });
  const id = target || list.data?.records[0]?.id;
  const detail = useQuery({
    queryKey: [...key, id],
    queryFn: ({ signal }) =>
      api.get<AdvisoryDetail>("/main-agent/" + id, signal),
    enabled: online && open && !!id,
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
  const record =
    detail.data?.candidate_id === run.run_id ? detail.data : undefined;
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
  return (
    <details
      className="detail-section"
      open={open}
      onToggle={(e) => setOpen(e.currentTarget.open)}
    >
      <summary>对话续作 · 提示词建议</summary>
      {open && (
        <>
          <p className="muted">
            收到候选明确待答问题后自动准备建议；仅供外部继续对话使用。
          </p>
          {(list.error || detail.error) && (
            <p role="alert">{String(list.error || detail.error)}</p>
          )}
          {!id && !list.isLoading && (
            <p>
              暂无已登记建议。候选有明确待答问题时会自动生成，原问题仍通过原入口处理。
            </p>
          )}
          {detail.data && !record && (
            <p role="alert">建议不属于当前候选，未显示其他范围的结果。</p>
          )}
          {record && (
            <article>
              <p>生成状态：{states[record.state] ?? "状态待核实"}</p>
              {record.diagnostic && <p role="alert">{record.diagnostic}</p>}
              {!record.context && (
                <p className="muted">
                  本次未形成完整上下文，请查看失败或阻断原因。
                </p>
              )}
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
                </details>
              )}
              {record.prompt && (
                <>
                  <pre aria-label="提示词建议正文">{record.prompt}</pre>
                  <Button onClick={() => void copy()}>复制提示词</Button>
                </>
              )}
              {[
                "queued",
                "preparing",
                "running",
                "unknown",
                "settling",
              ].includes(record.state) && (
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
      )}
    </details>
  );
}
