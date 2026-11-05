import { NavLink } from "react-router-dom";
import {
  environmentLink,
  useEnvironmentReadiness,
} from "../environment-checks/use-readiness";
import { useWorkspace } from "../../app/context";
import { Button } from "../../shared/ui/button";
import { Pause, Play } from "lucide-react";
export function Dispatch() {
  const { online, snapshot, send, busy, identity } = useWorkspace();
  const preparation = useEnvironmentReadiness();
  const selected = preparation.data?.profiles?.find(
    (p) => p.id === preparation.data?.default_profile_id,
  );
  const unverified =
    !preparation.verified || !selected || selected.stage === "unverified";
  const page = snapshot?.connection.snapshot.page;
  if (!page) return null;
  const { control } = page;
  const caps = page.service.capabilities ?? [];
  return (
    <div className="dispatch-bar">
      <div>
        <strong>当前项目派发</strong>
        <span className="muted">
          {control.dispatch_paused
            ? "已暂停"
            : control.dispatch_mode === "auto"
              ? "自动派发"
              : "手动派发"}
        </span>
      </div>
      <div className="actions">
        <Button
          role="switch"
          aria-checked={control.dispatch_mode === "auto"}
          aria-label="自动派发"
          size="sm"
          disabled={!online || busy || !caps.includes("set_dispatch_mode")}
          onClick={() =>
            void send(
              "set_dispatch_mode",
              "dispatch",
              control.revision,
              { mode: control.dispatch_mode === "auto" ? "manual" : "auto" },
              { expected_control_revision: control.revision },
            )
          }
        >
          自动派发 {control.dispatch_mode === "auto" ? "开启" : "关闭"}
        </Button>
        <Button
          size="sm"
          disabled={
            !online ||
            busy ||
            !caps.includes(
              control.dispatch_paused ? "resume_dispatch" : "pause_dispatch",
            )
          }
          onClick={() =>
            void send(
              control.dispatch_paused ? "resume_dispatch" : "pause_dispatch",
              "dispatch",
              control.revision,
              {},
              { expected_control_revision: control.revision },
            )
          }
        >
          {control.dispatch_paused ? <Play size={12} /> : <Pause size={12} />}{" "}
          {control.dispatch_paused ? "恢复派发" : "暂停派发"}
        </Button>
      </div>
      <p className="muted">默认执行方案：{selected?.name || "尚未核实"}</p>
      {!preparation.required && (
        <p className="notice amber">环境就绪阻塞已关闭（开发者配置）</p>
      )}
      {preparation.required && (unverified || selected.status !== "ready") && (
        <p className="muted">
          {unverified
            ? "默认执行环境待核实"
            : selected.stage === "verifying"
              ? "默认执行环境检测中"
              : "默认执行环境阻断"}
          {preparation.verified && selected?.reason && `：${selected.reason}`} ·{" "}
          <NavLink to={environmentLink(identity.project_id, selected?.id)}>
            前往准备
          </NavLink>
        </p>
      )}
      {control.blockers?.length > 0 && (
        <p className="muted">
          {control.blockers.map((b) => b.Reason).join("；")}
        </p>
      )}
    </div>
  );
}
