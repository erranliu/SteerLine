import { useEffect, useRef } from "react";
import { Link, NavLink, useLocation } from "react-router-dom";
import {
  environmentLink,
  type EnvironmentOptions,
  useEnvironmentReadiness,
} from "../environment-checks/use-readiness";
import { useWorkspace } from "../../app/context";
import { Button } from "../../shared/ui/button";
import { useProjectReadiness } from "../project-maintenance/use-readiness";

const dispatchSettingsId = "task-dispatch";

function defaultProfile(options?: EnvironmentOptions) {
  return options?.profiles?.find(
    (profile) => profile.id === options.default_profile_id,
  );
}

export function DispatchStatus() {
  const { identity, online, snapshot } = useWorkspace();
  const preparation = useEnvironmentReadiness();
  const mode = snapshot?.connection.snapshot.page?.control.dispatch_mode;
  let status = "待核实";
  if (online && mode === "manual") status = "关闭";
  if (online && mode === "auto") {
    status = defaultProfile(preparation.data)?.name || "默认执行方案待核实";
  }
  const label = `任务自动派发：${status}`;
  return (
    <Link
      className="dispatch-status"
      title={label}
      to={`/projects/${identity.project_id}/settings#${dispatchSettingsId}`}
    >
      {label}
    </Link>
  );
}

export function Dispatch() {
  const panel = useRef<HTMLElement>(null);
  const location = useLocation();
  useEffect(() => {
    if (location.hash !== `#${dispatchSettingsId}`) return;
    panel.current?.focus({ preventScroll: true });
    panel.current?.scrollIntoView?.({ block: "start" });
  }, [location.hash, location.key]);

  return (
    <section
      className="settings-panel"
      id={dispatchSettingsId}
      aria-labelledby={`${dispatchSettingsId}-title`}
      ref={panel}
      tabIndex={-1}
    >
      <h2 id={`${dispatchSettingsId}-title`}>任务自动派发</h2>
      <DispatchControls />
    </section>
  );
}

function DispatchControls() {
  const { online, snapshot, send, busy, identity } = useWorkspace();
  const preparation = useEnvironmentReadiness();
  const project = useProjectReadiness();
  const selected = defaultProfile(preparation.data);
  const unverified =
    !preparation.verified || !selected || selected.stage === "unverified";
  const page = snapshot?.connection.snapshot.page;
  if (!page) return <p className="muted">任务自动派发状态待核实</p>;
  const { control } = page;
  const automatic = control.dispatch_mode === "auto";
  const caps = page.service.capabilities ?? [];
  return (
    <>
      <div className="actions">
        <Button
          role="switch"
          aria-checked={automatic}
          aria-label="自动派发"
          size="sm"
          disabled={
            !online ||
            busy ||
            !caps.includes("set_dispatch_mode") ||
            (!automatic && !project.ready)
          }
          title={!automatic ? project.reason : undefined}
          onClick={() =>
            void send(
              "set_dispatch_mode",
              "dispatch",
              control.revision,
              { mode: automatic ? "manual" : "auto" },
              { expected_control_revision: control.revision },
            )
          }
        >
          自动派发 {automatic ? "开启" : "关闭"}
        </Button>
      </div>
      <p className="muted">默认执行方案：{selected?.name || "尚未核实"}</p>
      {!project.ready && (
        <p className="muted">
          {project.text}
          {project.diagnostic && `：${project.diagnostic}`} ·{" "}
          <NavLink
            to={`/projects/${identity.project_id}/status`}
          >
            前往项目状态
          </NavLink>
        </p>
      )}
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
    </>
  );
}
