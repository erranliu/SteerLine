import { useEffect, useRef, useState } from "react";
import { useWorkspace } from "../../app/context";
import { readDraft, writeDraft } from "../../shared/api/journal";
import type { Candidate } from "../../shared/api/types";
import { Button } from "../../shared/ui/button";
import { Status } from "../../shared/ui/status";
import { businessOpen, requiresAnswer } from "./state";

export function draftKey(project: string, instance: string, run: Candidate) {
  return [
    project,
    instance,
    run.run_id,
    run.execution_id,
    run.attempt_id,
    run.decision?.id,
    run.decision?.version,
  ].join("/");
}
export function Answer({ run }: { run: Candidate }) {
  const { api, online, send, busy } = useWorkspace();
  const decision = run.decision!;
  const key =
    (api.home ? api.home + "/" : "") +
    draftKey(api.identity.project_id, api.identity.instance_id, run);
  const [note, setNote] = useState("");
  const [option, setOption] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState("");
  const [storageError, setStorageError] = useState("");
  const [opened, setOpened] = useState(false);
  const [composing, setComposing] = useState(false);
  const [checking, setChecking] = useState(false);
  const eligibility = useRef<AbortController | null>(null);
  useEffect(
    () => () => eligibility.current?.abort(),
    [
      key,
      online,
      run.control_revision,
      run.recovery_version,
      run.workflow_status,
      run.task_status,
    ],
  );
  useEffect(() => {
    let active = true;
    readDraft(key)
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
        note,
        expected_run_revision: run.control_revision,
        source_attempt_id: run.attempt_id,
        expected_recovery_version: run.recovery_version,
      };
      if (
        decision.request.has_native_questions &&
        note.trim().startsWith("{")
      ) {
        const answers = JSON.parse(note);
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
      if (await send("answer_decision", decision.id, decision.version, payload))
        setOpened(false);
    } catch (e) {
      if (!controller.signal.aborted) setError(String(e));
    } finally {
      setChecking(false);
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
              : "追加反馈"}
        </h3>
        <Status value={decision.status} />
      </div>
      <h4>{decision.request.question}</h4>
      {decision.request.context && (
        <pre className="context-text">{decision.request.context}</pre>
      )}
      <p className="muted">
        {!businessOpen(run)
          ? "候选已结束，问题与回答仅供回看。"
          : requiresAnswer(run)
            ? "回答后按原问题继续。"
            : "可选反馈；等待 PR 合并不要求继续执行。"}
      </p>
      {decision.status === "pending" && businessOpen(run) && (
        <>
          {!opened ? (
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
                value={note}
                placeholder={
                  decision.request.has_native_questions
                    ? "输入回答；多题可按问题 ID 填写 JSON。"
                    : "输入补充说明…"
                }
                rows={5}
                disabled={!loaded}
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
                      (!option && !note.trim())
                    }
                    onClick={submit}
                  >
                    {checking ? "核验原问题…" : "提交回答"}
                  </Button>
                </div>
              </div>
            </>
          )}
        </>
      )}
      {(error || storageError) && (
        <p role="alert" className="notice amber">
          {error || storageError}
        </p>
      )}
      <p className="target-note">
        Decision {decision.id.slice(0, 8)} · v{decision.version} · Attempt{" "}
        {run.attempt_id.slice(0, 8)}
      </p>
    </section>
  );
}
