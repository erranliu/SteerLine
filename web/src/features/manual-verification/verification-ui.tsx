import type { ReactNode } from "react";
import { ChevronDown, Play, type LucideIcon } from "lucide-react";
import { Button } from "../../shared/ui/button";

export const runStatuses: Record<string, string> = {
  preparing: "正在准备 worktree",
  running: "进程运行中",
  ready: "预览可访问",
  exited: "进程已退出",
  stopped: "已停止",
  failed: "启动或运行失败",
  unknown: "原进程待核实",
};

export const isActiveRun = (status: string) =>
  ["preparing", "running", "ready", "unknown"].includes(status);

export function VerificationStatus({
  tone = "neutral",
  children,
}: {
  tone?: "neutral" | "positive" | "attention" | "negative";
  children: ReactNode;
}) {
  return (
    <span className="mv-status" data-tone={tone}>
      {children}
    </span>
  );
}

export function RunStatus({
  status,
  label,
}: {
  status: string;
  label?: string;
}) {
  const tone =
    status === "failed"
      ? "negative"
      : status === "unknown" || status === "preparing"
        ? "attention"
        : status === "running" || status === "ready"
          ? "positive"
          : "neutral";
  return (
    <VerificationStatus tone={tone}>
      {label || runStatuses[status] || status}
    </VerificationStatus>
  );
}

export function LaunchPanel({
  id,
  title,
  subtitle,
  icon: Icon,
  status,
  expanded,
  toggle,
  children,
}: {
  id: string;
  title: string;
  subtitle: string;
  icon: LucideIcon;
  status?: ReactNode;
  expanded: boolean;
  toggle: () => void;
  children: ReactNode;
}) {
  return (
    <section className="mv-panel mv-launch">
      <h2>
        <button
          type="button"
          className="mv-launch-toggle"
          id={id}
          aria-expanded={expanded}
          aria-controls={id + "-body"}
          aria-label={title}
          aria-describedby={id + "-description"}
          onClick={toggle}
        >
          <span className="mv-launch-icon">
            <Icon size={20} aria-hidden="true" />
          </span>
          <span className="mv-launch-label">
            {title}
            <span className="mv-launch-meta" id={id + "-description"}>
              <small>{subtitle}</small>
              {status}
            </span>
          </span>
          <ChevronDown className="mv-chevron" size={16} aria-hidden="true" />
        </button>
      </h2>
      <div
        id={id + "-body"}
        className="mv-launch-body"
        hidden={!expanded}
        aria-labelledby={id}
      >
        {children}
      </div>
    </section>
  );
}

export type Launch = {
  program: string;
  args: string[];
  cwd?: string;
  url?: string;
};

export function LaunchEntries({
  entries,
  disabled,
  start,
}: {
  entries: { id: string; name: string; launch: Launch }[];
  disabled: boolean;
  start: (id: string) => void;
}) {
  if (!entries.length) return null;
  return (
    <ul className="mv-entry-list">
      {entries.map(({ id, name, launch }, index) => (
        <li key={id}>
          <Button
            type="button"
            size="sm"
            variant={index === 0 ? "default" : "outline"}
            disabled={disabled}
            onClick={() => start(id)}
          >
            <Play size={14} aria-hidden="true" /> {name}
          </Button>
          <details className="mv-disclosure">
            <summary>启动参数</summary>
            <pre>
              <code>{JSON.stringify([launch.program, ...launch.args])}</code>
            </pre>
            <p>
              工作目录 <code>{launch.cwd || "."}</code>
            </p>
          </details>
        </li>
      ))}
    </ul>
  );
}
