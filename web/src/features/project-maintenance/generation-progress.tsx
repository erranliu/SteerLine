import { useEffect, useRef, useState } from "react";
import { LoaderCircle } from "lucide-react";

export type GenerationProgressValue = {
  id: string;
  total: number;
  succeeded: number;
  queued: number;
  running: number;
  failed: number;
  blocked: number;
  completed_at?: string;
};

export function GenerationProgress({
  value,
  online,
  scope = "project",
}: {
  value?: GenerationProgressValue;
  online: boolean;
  scope?: string;
}) {
  const [, tick] = useState(0);
  const observedActive = useRef("");
  const [displayWindow, setDisplayWindow] = useState<{ id: string; deadline: number }>();
  const complete = !!value?.total && value.succeeded === value.total;
  useEffect(() => {
    if (!value?.total) return;
    if (!complete) {
      observedActive.current = value.id;
      return;
    }
    if (!value.completed_at) return;
    const backendTime = Date.parse(value.completed_at);
    if (!Number.isFinite(backendTime)) return;
    // The polling delay must not consume the five seconds the user sees.
    // Persist only this display preference, never generation facts.
    const key = "steerline:generate-notice:" + scope;
    const id = value.id + ":" + value.completed_at;
    let deadline = backendTime + 5000;
    try {
      const saved = JSON.parse(sessionStorage.getItem(key) || "null") as {
        id?: string;
        deadline?: number;
      } | null;
      if (saved?.id === id && typeof saved.deadline === "number")
        deadline = saved.deadline;
      else if (observedActive.current === value.id || deadline > Date.now()) {
        deadline = Date.now() + 5000;
        sessionStorage.setItem(key, JSON.stringify({ id, deadline }));
      }
    } catch {
      if (observedActive.current === value.id) deadline = Date.now() + 5000;
    }
    setDisplayWindow({ id, deadline });
  }, [value?.id, value?.completed_at, complete, scope]);
  const deadline =
    displayWindow?.id === value?.id + ":" + value?.completed_at
      ? displayWindow.deadline
      : undefined;
  useEffect(() => {
    if (deadline === undefined || !Number.isFinite(deadline)) return;
    const timer = setTimeout(
      () => tick((v) => v + 1),
      Math.max(0, deadline - Date.now()),
    );
    return () => clearTimeout(timer);
  }, [deadline, value?.id]);
  if (!value?.total) return null;
  if (complete && deadline !== undefined && Date.now() >= deadline) return null;
  const active = online && value.running > 0;
  const text = !online
    ? "状态待核实"
    : complete
      ? "生成完毕"
      : value.running
        ? "生成中"
        : value.queued
          ? "排队中"
          : "已生成";
  return (
    <span role="status" aria-live="polite" className="maintenance-progress">
      {active && (
        <LoaderCircle
          aria-label="正在生成"
          className="maintenance-spinner"
          size={16}
        />
      )}
      <span>
        {value.succeeded}/{value.total} {text}
        {value.failed > 0 && `，${value.failed} 项失败`}
        {value.blocked > 0 && `，${value.blocked} 项待处理`}
      </span>
    </span>
  );
}
