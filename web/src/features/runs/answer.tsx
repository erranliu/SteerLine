import { useEffect, useRef, useState } from "react";
import { useWorkspace } from "../../app/context";
import {
  readDraftValue,
  requestByID,
  writeDraft,
} from "../../shared/api/journal";
import type { Candidate, Envelope, Receipt } from "../../shared/api/types";
import { Button } from "../../shared/ui/button";
import { Status } from "../../shared/ui/status";
import { businessOpen, requiresAnswer, optionalAnswer } from "./state";
import {
  ContinuationSuggestion,
  type AdvisoryDetail,
} from "./continuation-suggestion";
import { sameNotice } from "./presentation";

export function draftKey(project: string, instance: string, run: Candidate) {
  return [
    project,
    instance,
    run.run_id,
    run.execution_id,
    run.attempt_id,
    run.decision?.id,
  ].join("/");
}
export function Answer({ run }: { run: Candidate }) {
  const { api, online, submit: submitCommand, busy, requests } = useWorkspace();
  const decision = run.decision!;
  const key =
    (api.home ? api.home + "/" : "") +
    draftKey(api.identity.project_id, api.identity.instance_id, run);
  const [note, setNote] = useState<string>();
  const [suggestion, setSuggestion] = useState<AdvisoryDetail>();
  const answer = note ?? suggestion?.prompt ?? "";
  const [option, setOption] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [storageError, setStorageError] = useState("");
  const [opened, setOpened] = useState(true);
  const [pending, setPending] = useState<{
    envelope: Envelope;
    receipt?: Receipt;
  }>();
  const restored = requests?.find(
    (row) =>
      row.envelope.project_id === api.identity.project_id &&
      row.envelope.instance_id === api.identity.instance_id &&
      row.envelope.action === "answer_decision" &&
      row.envelope.target_id === decision.id &&
      row.envelope.payload.run_id === run.run_id &&
      row.envelope.payload.source_attempt_id === run.attempt_id &&
      row.response?.status !== "rejected" &&
      (row.response?.status !== "applied" ||
        decision.request.interaction_kind !== "business_feedback"),
  );
  const envelope = pending?.envelope ?? restored?.envelope;
  const receipt =
    requests?.find((row) => row.envelope.request_id === envelope?.request_id)
      ?.response ??
    pending?.receipt ??
    restored?.response;
  const answered = !!envelope && receipt?.status !== "rejected";
  const rejected =
    receipt?.status === "rejected"
      ? receipt.diagnostic || "回答未成功，请核对原问题后重新提交。"
      : "";
  const [composing, setComposing] = useState(false);
  const [checking, setChecking] = useState(false);
  const eligibility = useRef<AbortController | null>(null);
  const submitting = useRef(false);
  useEffect(
    () => () => eligibility.current?.abort(),
    [
      key,
      online,
      run.control_revision,
      run.recovery_version,
      run.workflow_status,
      run.task_status,
      decision.id,
      decision.version,
      decision.status,
      run.participation,
      run.stop_requested,
    ],
  );
  useEffect(() => {
    let active = true;
    readDraftValue(key)
      .then((value) => {
        if (active) {
          setNote(value);
          setLoaded(true);
        }
      })
      .catch((e) => active && setStorageError(String(e)));
    return () => {
      active = false;
    };
  }, [key]);
  const update = (value: string) => {
    setNote(value);
    writeDraft(key, value)
      .then(() => setStorageError(""))
      .catch((e) => setStorageError(String(e)));
  };
  const submit = async () => {
    if (submitting.current || answered) return;
    submitting.current = true;
    eligibility.current?.abort();
    const controller = new AbortController();
    eligibility.current = controller;
    setChecking(true);
    setError("");
    try {
      await api.get(
        "/runs/" +
          run.run_id +
          "/decision-eligibility?" +
          new URLSearchParams({
            execution_id: run.execution_id,
            attempt_id: run.attempt_id,
          }),
        controller.signal,
      );
      controller.signal.throwIfAborted();
      const payload: Record<string, unknown> = {
        run_id: run.run_id,
        decision_id: decision.id,
        option_id: option,
        note: answer,
        expected_run_revision: run.control_revision,
        source_attempt_id: run.attempt_id,
        expected_recovery_version: run.recovery_version,
      };
      if (
        decision.request.has_native_questions &&
        answer.trim().startsWith("{")
      ) {
        const answers = JSON.parse(answer);
        if (
          !answers ||
          Array.isArray(answers) ||
          Object.values(answers).some(
            (value) =>
              !Array.isArray(value) ||
              value.some((item) => typeof item !== "string"),
          )
        )
          throw new Error("多题答案须为问题 ID 对应字符串数组的 JSON");
        payload.answers = answers;
        delete payload.note;
        delete payload.option_id;
      }
      const original = api.envelope(
        "answer_decision",
        decision.id,
        decision.version,
        payload,
      );
      setPending({ envelope: original });
      let result: Receipt | undefined;
      try {
        result = await submitCommand(original);
      } catch (e) {
        setError(String(e));
      }
      if (!result) {
        const saved = await requestByID(original.request_id);
        if (!saved) {
          setPending(undefined);
          setError((value) => value || "回答尚未提交，请核对操作错误后重试。");
          return;
        }
        result = saved.response;
      }
      setPending({ envelope: original, receipt: result });
    } catch (e) {
      if (!controller.signal.aborted) setError(String(e));
    } finally {
      setChecking(false);
      submitting.current = false;
    }
  };
  return (
    <section className="answer-panel">
      <div className="section-heading">
        <h3>
          {!businessOpen(run)
            ? "历史问题"
            : requiresAnswer(run)
              ? "需要你的决定"
              : optionalAnswer(run)
                ? "可稍后回答"
                : "追加反馈"}
        </h3>
        <Status value={decision.status} />
      </div>
      <h4>{decision.request.question}</h4>
      {decision.request.context &&
        !sameNotice(decision.request.question, decision.request.context) && (
          <pre className="context-text">{decision.request.context}</pre>
        )}
      <p className="muted">
        {!businessOpen(run)
          ? "候选已结束，问题与回答仅供回看。"
          : requiresAnswer(run)
            ? "回答后按原问题继续。"
            : optionalAnswer(run)
              ? "当前执行可继续，方便时回答这个问题。"
              : "可选反馈；等待 PR 合并不要求继续执行。"}
      </p>
      {(decision.status !== "pending" || !businessOpen(run)) && (
        <ContinuationSuggestion run={run} />
      )}
      {decision.status === "pending" && businessOpen(run) && (
        <>
          {!answered && (
            <ContinuationSuggestion
              run={run}
              inline
              onSuggestion={setSuggestion}
            />
          )}
          {answered ? (
            <>
              <p role="status">
                {!receipt || receipt.status === "outcome_unknown"
                  ? "回答结果待核实，请在操作记录中核实原请求。"
                  : "回答已提交，等待状态更新。"}
              </p>
              {receipt?.status === "applied" &&
                decision.request.interaction_kind === "business_feedback" && (
                  <Button
                    onClick={() => {
                      setPending(undefined);
                      setOption("");
                      update("");
                      setOpened(true);
                    }}
                  >
                    继续追加反馈
                  </Button>
                )}
            </>
          ) : !opened ? (
            <Button
              disabled={!online || busy || !loaded}
              onClick={() => setOpened(true)}
            >
              打开回答
            </Button>
          ) : (
            <>
              <div className="options">
                {decision.request.options?.map((o) => (
                  <label key={o.ID}>
                    <input
                      type="radio"
                      name={"decision-" + decision.id}
                      value={o.ID}
                      checked={option === o.ID}
                      onChange={() => setOption(o.ID)}
                    />
                    <span>
                      {o.Label}
                      <small>{o.Detail}</small>
                    </span>
                  </label>
                ))}
              </div>
              <label htmlFor="answer-note">回答 / 补充说明</label>
              <textarea
                id="answer-note"
                aria-label="回答内容"
                value={answer}
                placeholder={
                  decision.request.has_native_questions
                    ? "输入回答；多题可按问题 ID 填写 JSON。"
                    : "输入补充说明…"
                }
                rows={5}
                disabled={!loaded || checking || busy}
                onChange={(e) => update(e.target.value)}
                onCompositionStart={() => setComposing(true)}
                onCompositionEnd={() => setComposing(false)}
              />
              <div className="input-footer">
                <span>Enter 换行 · 点击提交 · 绑定原候选与问题</span>
                <div className="actions">
                  <Button
                    onClick={() => {
                      eligibility.current?.abort();
                      setOpened(false);
                    }}
                  >
                    收起
                  </Button>
                  <Button
                    variant="default"
                    disabled={
                      !online ||
                      busy ||
                      checking ||
                      composing ||
                      !loaded ||
                      !!storageError ||
                      (!option && !answer.trim())
                    }
                    onClick={submit}
                  >
                    {checking
                      ? "正在回复…"
                      : requiresAnswer(run)
                        ? "回复并继续"
                        : "提交回答"}
                  </Button>
                </div>
              </div>
            </>
          )}
        </>
      )}
      {(error || storageError || rejected) && (
        <p role="alert" className="notice amber">
          {error || storageError || rejected}
        </p>
      )}
    </section>
  );
}
