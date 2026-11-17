import type { MaintenanceRecord } from "./refactoring-maintenance";

type Operation = NonNullable<MaintenanceRecord["operations"]>[number];

const purposes: Record<string, string> = {
  first: "首轮选题",
  select: "后续选题",
  audit: "成果审计",
  finish: "收尾决策",
  report: "PR 合入评审",
  implement: "实施",
  repair: "修复或补证",
  publish: "交付",
  conflict: "处理冲突",
};

const operationStates: Record<string, string> = {
  pending: "待提交",
  submitted: "已提交",
  complete: "已完成",
  rejected: "已拒绝",
  cancelled: "已取消",
};

export function operationPurposeLabel(purpose: string) {
  return purposes[purpose] ?? purpose;
}

export function operationStateLabel(state: string) {
  return operationStates[state] ?? state;
}

const recommendations = {
  merge: "建议合入",
  revise: "建议修改后再审",
  insufficient_evidence: "证据不足",
};

function MessageTime({ value }: { value?: string | null }) {
  return (
    value && <time dateTime={value}>{new Date(value).toLocaleString()}</time>
  );
}

function DecisionPrompt({
  decision,
  record,
}: {
  decision?: Operation;
  record: MaintenanceRecord;
}) {
  return (
    <details>
      <summary>{decision ? "一级提示词" : "维护要求"}</summary>
      <dl>
        {[
          ["方向提示词", record.direction_prompt],
          ["补充要求", record.input.requirements],
          ["决策短指令", decision?.prompt],
          ["阶段规则", decision?.decision_rules?.rules],
          ["策略规则", decision?.decision_rules?.strategy],
        ].map(([label, body]) =>
          body ? (
            <div key={label}>
              <dt>{label}</dt>
              <dd>
                <pre tabIndex={0}>{body}</pre>
              </dd>
            </div>
          ) : null,
        )}
      </dl>
    </details>
  );
}

function AgentMessage({
  record,
  operation,
  decision,
}: {
  record: MaintenanceRecord;
  operation: Operation;
  decision?: Operation;
}) {
  const execution = operation.workflow === "execute";
  const report =
    operation.report_index === undefined
      ? undefined
      : record.reports?.[operation.report_index];
  const body = execution
    ? operation.prompt
    : report?.body || operation.feedback || operation.decision_summary;
  const summary = execution
    ? decision?.decision_summary || operation.prompt
    : operation.decision_summary || report?.body;
  const purpose = operationPurposeLabel(operation.purpose);
  return (
    <li className="refactoring-message" data-speaker="agent">
      <h4>
        SteerAgent ·{" "}
        {execution && decision?.purpose === "audit" && "成果审计 → "}
        {purpose}
      </h4>
      <MessageTime
        value={
          execution
            ? operation.created_at
            : operation.completed_at || operation.created_at
        }
      />
      {report && (
        <p>
          {recommendations[report.recommendation]}
          {record.report_stale &&
            operation.report_index === record.reports!.length - 1 &&
            " · 此评审已过期或正在复核"}
        </p>
      )}
      {summary && <p className="refactoring-message-summary">{summary}</p>}
      {!execution && !summary && (
        <p>
          {operation.state === "complete" && operation.feedback
            ? "未形成已采纳的结论"
            : operation.workflow === "generate" &&
                operation.state === "submitted"
              ? "等待 SteerAgent 决策"
              : operationStateLabel(operation.state)}
        </p>
      )}
      <div className="refactoring-message-actions">
        {body && (
          <details>
            <summary>{execution ? "展开指令" : "展开结论"}</summary>
            {report && (
              <p>
                head <code>{report.head}</code> · base{" "}
                <code>{report.base}</code>
              </p>
            )}
            <pre tabIndex={0}>{body}</pre>
          </details>
        )}
        {decision && <DecisionPrompt decision={decision} record={record} />}
      </div>
    </li>
  );
}

function RunnerMessage({ operation }: { operation: Operation }) {
  const state = operationStateLabel(operation.state);
  const result = operation.result;
  const summary =
    result?.summary ||
    result?.question ||
    (operation.state === "complete"
      ? operation.feedback
        ? "回执未通过核验"
        : "未收到文字回执"
      : operation.state === "submitted"
        ? "等待 Runner 回执"
        : state);
  return (
    <li className="refactoring-message" data-speaker="runner">
      <h4>Runner · {operationPurposeLabel(operation.purpose)}反馈</h4>
      <MessageTime value={operation.completed_at} />
      <p className="refactoring-message-summary">{summary}</p>
      {(operation.feedback || operation.run_id) && (
        <div className="refactoring-message-actions">
          <details>
            <summary>展开回执</summary>
            <p>
              {state}
              {operation.run_status && (
                <>
                  {" "}
                  · <code>{operation.run_status}</code>
                </>
              )}
              {operation.run_id && (
                <>
                  {" "}
                  · Run ID <code>{operation.run_id}</code>
                </>
              )}
            </p>
            {operation.feedback && <pre tabIndex={0}>{operation.feedback}</pre>}
          </details>
        </div>
      )}
    </li>
  );
}

export function ExecutionConversation({
  record,
}: {
  record: MaintenanceRecord;
}) {
  const operations = record.operations ?? [];
  return (
    <section className="refactoring-conversation" aria-label="执行对话">
      <h3>执行对话</h3>
      {operations.length ? (
        <ol>
          {operations.map((operation, i) => {
            if (operation.workflow === "execute") {
              const previous = operations[i - 1];
              const decision =
                previous?.decision_kind === "execute" ? previous : undefined;
              return [
                <AgentMessage
                  key={operation.id}
                  record={record}
                  operation={operation}
                  decision={decision}
                />,
                <RunnerMessage
                  key={operation.id + "-runner"}
                  operation={operation}
                />,
              ];
            }
            if (
              operation.decision_kind === "execute" &&
              operations[i + 1]?.workflow === "execute"
            ) {
              return null;
            }
            return (
              <AgentMessage
                key={operation.id}
                record={record}
                operation={operation}
                decision={operation}
              />
            );
          })}
        </ol>
      ) : (
        <>
          <p className="muted">
            {record.active
              ? "等待 SteerAgent 生成执行提示词"
              : "本次重构任务未生成执行提示词"}
          </p>
          {(record.direction_prompt || record.input.requirements) && (
            <DecisionPrompt record={record} />
          )}
        </>
      )}
    </section>
  );
}
