import { useState } from "react";
import { Copy } from "lucide-react";
import { Button } from "../../shared/ui/button";
import type { Envelope, Receipt } from "../../shared/api/types";

export type ExecutionReference = {
  request_id?: string;
  run_id?: string;
  attempt_id?: string;
  session_id?: string;
  action?: string;
};
export type ExecutionInfo = {
  title: string;
  project?: string;
  requestID?: string;
  record?: { id: string; label: string };
  taskID?: string;
  generationID?: string;
  runner?: ExecutionReference;
  runnerUnverified?: boolean;
};
export const operationNames: Record<string, string> = {
  roadmap_parse: "Roadmap 解析",
  roadmap_generate_documents: "Spec / Plan 生成",
  roadmap_task_generate: "确认任务卡并生成 Spec / Plan",
  roadmap_retry_documents: "单任务文档重生成",
  roadmap_publish: "Roadmap 文档发布",
  roadmap_admit: "Roadmap 入图",
  roadmap_resume: "继续 Roadmap 操作",
  roadmap_confirm: "确认任务与依赖",
  roadmap_stop: "停止内容生成",
  roadmap_observe: "查询原操作",
  roadmap_prepare_static: "初始化静态材料",
  roadmap_edit_documents: "修改生成文档",
  environment_start: "环境检测",
  environment_stop: "停止环境检测",
  environment_invalidate: "标记环境失效",
  activity_abandon: "放弃活动",
  maintenance_check: "检查主分支",
  maintenance_update: "更新本地",
  start_task: "启动任务",
  start_candidate: "启动候选",
};
export function formatExecutionInfo(info: ExecutionInfo) {
  const lines = [
    [
      info.project
        ?.replace(/[\\/]\.git(?:[\\/].*)?$/i, "")
        .split(/[\\/]/)
        .filter(Boolean)
        .at(-1),
      info.title,
    ]
      .filter(Boolean)
      .join(" · "),
  ];
  if (info.taskID) lines.push(`Task ID：${info.taskID}（正式任务）`);
  if (info.requestID && info.requestID === info.record?.id) {
    lines.push(
      `请求 / ${info.record.label} ID：${info.requestID}（本次提交及结果）`,
    );
  } else {
    if (info.requestID) lines.push(`请求 ID：${info.requestID}（本次提交）`);
    if (info.record)
      lines.push(`${info.record.label} ID：${info.record.id}（关联业务记录）`);
  }
  if (info.generationID)
    lines.push(`Generation ID：${info.generationID}（任务生成）`);
  if (info.runner) {
    lines.push(
      info.runner.run_id
        ? `Runner Run ID：${info.runner.run_id}（底层运行）`
        : info.runnerUnverified
          ? "Runner 运行待核实"
          : "Runner 运行尚未创建",
    );
    if (info.runner.attempt_id)
      lines.push(`Attempt ID：${info.runner.attempt_id}（具体执行轮次）`);
  }
  if (info.runner?.session_id)
    lines.push(`Session ID：${info.runner.session_id}（原生会话）`);
  return lines.join("\n");
}

// Uses only the selected request/receipt, never a newer domain record.
export function requestExecutionInfo(
  e: Envelope,
  receipt?: Receipt,
): ExecutionInfo {
  const record = (receipt?.roadmap || receipt?.environment) as
    | { id?: string; run_id?: string; execution_ref?: ExecutionReference }
    | undefined;
  const runner = [
    "roadmap_parse",
    "environment_start",
    "start_task",
    "start_candidate",
  ].includes(e.action);
  return {
    title: operationNames[e.action] || e.action,
    requestID: e.request_id,
    taskID: ["start_task", "start_candidate"].includes(e.action)
      ? e.target_id
      : undefined,
    record: ["start_task", "start_candidate"].includes(e.action)
      ? { id: e.request_id, label: "候选" }
      : record?.id
        ? {
            id: record.id,
            label: receipt?.roadmap ? "Roadmap 记录" : "检测记录",
          }
        : undefined,
    runner: runner
      ? record?.execution_ref || { run_id: record?.run_id }
      : undefined,
    runnerUnverified: runner && !record && receipt?.status !== "rejected",
  };
}

export function CopyExecutionInfo({ info }: { info: ExecutionInfo }) {
  return (
    <CopyInfoButton text={formatExecutionInfo(info)} label="复制执行信息" />
  );
}

export function CopyInfoButton({
  text,
  label,
}: {
  text: string | (() => Promise<string>);
  label: string;
}) {
  const [feedback, setFeedback] = useState<
    { kind: "copied" | "load-failed" } | { kind: "manual"; text: string }
  >();
  const [copying, setCopying] = useState(false);
  return (
    <span className="copy-execution-info">
      <Button
        size="sm"
        variant="ghost"
        disabled={copying}
        onClick={async () => {
          setCopying(true);
          setFeedback(undefined);
          let content: string | undefined;
          try {
            content = typeof text === "string" ? text : await text();
            if (!navigator.clipboard?.writeText)
              throw new Error("剪贴板不可用");
            await navigator.clipboard.writeText(content);
            setFeedback({ kind: "copied" });
          } catch {
            setFeedback(
              content === undefined
                ? { kind: "load-failed" }
                : { kind: "manual", text: content },
            );
          } finally {
            setCopying(false);
          }
        }}
      >
        <Copy size={13} />
        {label}
      </Button>
      {feedback?.kind === "manual" && (
        <span role="alert">
          复制失败，请手动复制：<pre>{feedback.text}</pre>
        </span>
      )}
      {feedback?.kind === "load-failed" && (
        <span role="alert">读取失败，请重试</span>
      )}
      {feedback?.kind === "copied" && <span role="status">已{label}</span>}
    </span>
  );
}
