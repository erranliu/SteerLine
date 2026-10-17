const labels: Record<string, string> = {
  ready: "就绪",
  pending: "待执行",
  running: "运行中",
  in_progress: "执行中",
  waiting: "等待中",
  completed: "本轮完成",
  merged: "已合并",
  delivered: "已交付",
  discarded: "已放弃",
  stopped: "已停止",
  stop_pending: "停止待核实",
  failed: "失败",
  blocked: "受阻",
  dependency_blocked: "依赖阻塞",
  waiting_for_merge: "待合并",
  waiting_for_input: "待回答",
  waiting_pr: "待合并",
  answer_saved: "答案已保存",
  reply_accepted: "输入已接受",
  applied: "已应用",
  accepted: "已接受",
  submitted: "已提交",
  rejected: "已拒绝",
  outcome_unknown: "结果待核实",
  open: "待执行",
  closed: "已交付",
  queued: "排队中",
  succeeded: "本轮成功",
  waiting_input: "待回答",
  waiting_external: "等待外部事实",
  reconciling: "核实中",
  abandoned: "已停止参与",
};
export const label = (status: string) => labels[status] ?? status ?? "未知";
export function Status({ value }: { value: string }) {
  return (
    <span className={"status status-" + value}>
      {label(value || "unknown")}
    </span>
  );
}
export const short = (value: string) => value?.slice(0, 8) || "—";
export function SafeLink({
  url,
  children,
}: {
  url?: string;
  children: React.ReactNode;
}) {
  if (!url || !/^https?:\/\//i.test(url)) return <span>{children}</span>;
  return (
    <a href={url} target="_blank" rel="noopener noreferrer">
      {children}
    </a>
  );
}
