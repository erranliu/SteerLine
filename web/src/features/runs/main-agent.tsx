import { useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import type { Candidate, Receipt } from "../../shared/api/types";
import { Button } from "../../shared/ui/button";
import { SafeLink } from "../../shared/ui/status";

type Document = { path: string; commit: string; sha256: string; body: string };
type Answer = {
  note: string;
  option_id: string;
  answers: Record<string, string[]>;
};
export type MaterialAnalysis = {
  id: string;
  revision: number;
  created_at: string;
  state: string;
  diagnostic?: string;
  review: string;
  input: { kind: string; note: string; parent_id?: string };
  context: {
    candidate_id: string;
    candidate_revision: number;
    runner_revision: number;
    repository_commit: string;
    original_base: string;
    question?: {
      id: string;
      text: string;
      native_questions?: { id: string; text: string }[];
    };
    original: Document[];
    current: Document[];
  };
  result?: {
    disposition: string;
    summary: string;
    recommendation: string;
    questions: string[];
    evidence: { path: string; commit: string; quote: string }[];
    answer: Answer | null;
    changes: {
      path: string;
      before_sha256: string;
      body: string;
      reason: string;
    }[];
  };
  answer_action?: { caller_id: string; request_id: string; review: string };
  projection?: { stop_status: string };
  submission: string;
  publication?: {
    request_id: string;
    state: string;
    url: string;
    diagnostic?: string;
  };
};
type Options = {
  commit: string;
  rules_ready: boolean;
  diagnostic: string;
  model: string;
  reasoning_effort: string;
};
type Summary = {
  id: string;
  state: string;
  created_at: string;
  note: string;
  kind: string;
};

export function MainAgent({ run }: { run: Candidate }) {
  const { api, online, submit, busy } = useWorkspace();
  const client = useQueryClient();
  const [open, setOpen] = useState(false);
  const [kind, setKind] = useState("clarification");
  const [note, setNote] = useState("");
  const [selected, setSelected] = useState("");
  const [follow, setFollow] = useState(false);
  const [working, setWorking] = useState(false);
  const [message, setMessage] = useState("");
  const [unverified, setUnverified] = useState("");
  const key = [
    api.home,
    api.identity.project_id,
    api.identity.instance_id,
    "main-agent",
    run.run_id,
  ];
  const options = useQuery({
    queryKey: [...key, "options"],
    queryFn: ({ signal }) => api.get<Options>("/main-agent/options", signal),
    enabled: online && open,
    retry: false,
  });
  const list = useQuery({
    queryKey: [...key, "list"],
    queryFn: ({ signal }) =>
      api.get<{ records: Summary[]; has_more: boolean }>(
        "/main-agent?candidate_id=" + encodeURIComponent(run.run_id),
        signal,
      ),
    enabled: online && open,
    refetchInterval: open && online ? 2500 : false,
    retry: false,
  });
  const rows = [...(list.data?.records ?? [])].sort((a, b) =>
    b.created_at.localeCompare(a.created_at),
  );
  const id = selected || rows[0]?.id;
  const detail = useQuery({
    queryKey: [...key, id],
    queryFn: ({ signal }) =>
      api.get<MaterialAnalysis>("/main-agent/" + id, signal),
    enabled: online && open && !!id,
    retry: false,
    refetchInterval: online && open ? 2500 : false,
  });
  const record = detail.data;
  const result = record?.result;
  const writable =
    online &&
    run.participation === "active" &&
    run.task_status === "in_progress" &&
    !run.stop_requested;
  const fresh =
    run.decision?.status === "pending" &&
    run.runner_delivery?.controls.can_reply === true &&
    !!record &&
    record.context.candidate_id === run.run_id &&
    record.context.candidate_revision === run.control_revision &&
    record.context.runner_revision === run.decision?.version &&
    record.context.question?.id === run.decision?.id;
  const settled =
    !!record &&
    (record.submission === "cancelled" ||
      record.submission === "rejected" ||
      record.projection?.stop_status === "stopped");
  const active = !!record && !settled;
  const disabled = working || busy || !!unverified;
  async function command(
    action: string,
    target: string,
    version: number,
    payload: Record<string, unknown>,
  ) {
    const envelope = api.envelope(action, target, version, payload);
    setWorking(true);
    setMessage("");
    try {
      const receipt = await submit(envelope);
      if (
        !receipt ||
        receipt.status === "outcome_unknown" ||
        receipt.status === "submitted"
      ) {
        setUnverified(envelope.request_id);
        setMessage("原请求结果待核实，请查询原请求，不要重复提交。");
        return undefined;
      }
      if (receipt.status === "rejected") {
        setMessage(receipt.diagnostic || receipt.code || "操作被拒绝");
        return undefined;
      }
      return envelope.request_id;
    } catch (e) {
      setUnverified(envelope.request_id);
      setMessage(String(e));
      return undefined;
    } finally {
      setWorking(false);
      await client.invalidateQueries({ queryKey: key });
    }
  }
  async function analyze() {
    const next = await command(
      "main_agent_analyze",
      run.run_id,
      run.control_revision,
      { kind, note, parent_id: follow ? (record?.id ?? "") : "" },
    );
    if (next) {
      setSelected(next);
      setNote("");
      setFollow(false);
    }
  }
  async function answer() {
    if (!record || !result?.answer || !record.context.question || !fresh)
      return;
    await command("main_agent_answer", record.id, record.revision, {
      review: record.review,
    });
  }

  async function inspect(requestId: string) {
    setWorking(true);
    try {
      const r = await api.get<Receipt>("/commands/" + requestId);
      setMessage(r.diagnostic || r.status);
      if (r.status === "applied" || r.status === "rejected") {
        setUnverified("");
        if (r.action === "main_agent_analyze" && r.status === "applied")
          setSelected(requestId);
      }
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
      onToggle={(e) => setOpen(e.currentTarget.open)}
    >
      <summary>主 Agent · 分析材料问题</summary>
      {open && (
        <>
          <p className="muted">
            仅分析任务要求不明确和要求冲突。模型：{options.data?.model ?? "—"} /{" "}
            {options.data?.reasoning_effort ?? "—"} · 不自动执行。
          </p>
          {(options.error || list.error || detail.error) && (
            <p role="alert">
              {String(options.error || list.error || detail.error)}
            </p>
          )}
          {options.data && !options.data.rules_ready && (
            <div className="notice amber">
              <p>{options.data.diagnostic}</p>
              <p>
                准备只生成固定规则文件；请审阅、提交并合入目标分支后再分析。
              </p>
              <Button
                disabled={!writable || disabled || !options.data.commit}
                onClick={() =>
                  void command(
                    "main_agent_prepare",
                    api.identity.project_id,
                    1,
                    { commit: options.data!.commit },
                  )
                }
              >
                准备主 Agent 规则
              </Button>
            </div>
          )}
          {list.data?.has_more && (
            <p className="muted">
              仅显示最近 50 条分析，旧记录仍可按原 ID 查询。
            </p>
          )}
          {rows.length > 0 && (
            <label>
              分析记录
              <select
                aria-label="分析记录"
                value={id ?? ""}
                onChange={(e) => {
                  setSelected(e.target.value);
                  setFollow(false);
                }}
              >
                {rows.map((r) => (
                  <option key={r.id} value={r.id}>
                    {r.note.slice(0, 50)} · {r.state}
                  </option>
                ))}
              </select>
            </label>
          )}
          {record && (
            <article>
              <p>分析状态：{record.state}</p>
              {record.diagnostic && <p role="alert">{record.diagnostic}</p>}
              <p>
                原候选基线：<code>{record.context.original_base}</code>
                ；本次材料基线：<code>{record.context.repository_commit}</code>
              </p>
              <pre>{record.input.note}</pre>
              {result && (
                <>
                  <h4>{result.summary}</h4>
                  <pre>{result.recommendation}</pre>
                  {result.questions.length > 0 && (
                    <>
                      <h4>需要补充的决定</h4>
                      <ul>
                        {result.questions.map((q, i) => (
                          <li key={i}>{q}</li>
                        ))}
                      </ul>
                    </>
                  )}
                  <details>
                    <summary>原文依据</summary>
                    {result.evidence.map((e, i) => (
                      <div key={i}>
                        <code>
                          {e.path} @ {e.commit}
                        </code>
                        <pre>{e.quote}</pre>
                      </div>
                    ))}
                  </details>
                  {result.answer && record.context.question && (
                    <section>
                      <h4>待提交的澄清回答</h4>
                      {record.answer_action && (
                        <p>此建议已登记原回答请求，请沿原问题观察送达结果。</p>
                      )}
                      <pre>{record.context.question.text}</pre>
                      {record.context.question.native_questions?.map((q) => (
                        <p key={q.id}>
                          {q.id}：{q.text}
                        </p>
                      ))}
                      <pre>
                        {Object.keys(result.answer.answers ?? {}).length
                          ? JSON.stringify(result.answer.answers, null, 2)
                          : [result.answer.option_id, result.answer.note]
                              .filter(Boolean)
                              .join("\n")}
                      </pre>
                      {!fresh && (
                        <p className="notice amber">
                          原问题或候选已变化，请重新分析；旧建议不能直接投递。
                        </p>
                      )}
                      <Button
                        disabled={
                          !writable ||
                          disabled ||
                          !fresh ||
                          !!record.answer_action
                        }
                        onClick={() => void answer()}
                      >
                        提交此澄清回答
                      </Button>
                    </section>
                  )}
                  {result.changes.map((change) => (
                    <section key={change.path}>
                      <h4>{change.path}</h4>
                      <p>{change.reason}</p>
                      <details>
                        <summary>查看完整原文与修订正文</summary>
                        <h5>原文</h5>
                        <pre>
                          {
                            record.context.current.find(
                              (d) => d.path === change.path,
                            )?.body
                          }
                        </pre>
                        <h5>修订正文</h5>
                        <pre>{change.body}</pre>
                      </details>
                    </section>
                  ))}
                  {result.changes.length > 0 && !record.publication && (
                    <Button
                      disabled={!writable || disabled}
                      onClick={() =>
                        void command(
                          "main_agent_publish",
                          record.id,
                          record.revision,
                          { review: record.review },
                        )
                      }
                    >
                      创建材料修订 PR
                    </Button>
                  )}
                  {record.publication && (
                    <div>
                      <p>材料发布：{record.publication.state}</p>
                      {record.publication.diagnostic && (
                        <p role="alert">{record.publication.diagnostic}</p>
                      )}
                      {record.publication.url ? (
                        <SafeLink url={record.publication.url}>
                          查看材料修订 PR
                        </SafeLink>
                      ) : (
                        <Button
                          disabled={working || !online}
                          onClick={() =>
                            void inspect(record.publication!.request_id)
                          }
                        >
                          查询原发布请求
                        </Button>
                      )}
                    </div>
                  )}
                  <Button
                    variant="outline"
                    disabled={!writable || disabled}
                    onClick={() => {
                      setFollow(true);
                      setKind(record.input.kind);
                    }}
                  >
                    补充信息并修订建议
                  </Button>
                </>
              )}
              {active && (
                <Button
                  variant="outline"
                  disabled={!writable || disabled}
                  onClick={() =>
                    void command(
                      "main_agent_stop",
                      record.id,
                      record.revision,
                      {},
                    )
                  }
                >
                  停止本次分析
                </Button>
              )}
            </article>
          )}
          <h4>{follow ? "补充本次讨论" : "发起材料分析"}</h4>
          <label>
            问题类型
            <select
              aria-label="材料问题类型"
              value={kind}
              disabled={disabled || active}
              onChange={(e) => setKind(e.target.value)}
            >
              <option value="clarification">任务要求不够明确</option>
              <option value="conflict">已有要求相互冲突</option>
            </select>
          </label>
          <label>
            问题或补充信息
            <textarea
              aria-label="材料问题描述"
              value={note}
              disabled={disabled || active}
              onChange={(e) => setNote(e.target.value)}
            />
          </label>
          <Button
            disabled={
              !writable ||
              disabled ||
              active ||
              !options.data?.rules_ready ||
              !note.trim()
            }
            onClick={() => void analyze()}
          >
            {follow ? "按补充信息重新分析" : "分析并给建议"}
          </Button>
          {unverified && (
            <Button
              disabled={!online || working}
              onClick={() => void inspect(unverified)}
            >
              查询原请求
            </Button>
          )}
          <p role="status">{message}</p>
        </>
      )}
    </details>
  );
}
