import { useEffect, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link, NavLink, useLocation } from "react-router-dom";
import { environmentLink } from "../environment-checks/use-readiness";
import { useWorkspace } from "../../app/context";
import { Button } from "../../shared/ui/button";
import { projectQueries } from "../../shared/api/queries";
import { usePageVisible } from "../../shared/query-visibility";

const dispatchSettingsId = "task-dispatch";

function useDispatchObservation(includeClosed = false) {
  const { api, online, snapshot } = useWorkspace();
  const control = snapshot?.connection.snapshot.page?.control;
  const automatic = control?.dispatch_mode === "auto";
  const visible = usePageVisible();
  const query = useQuery({
    ...projectQueries(api).dispatch(control?.revision ?? 0),
    enabled: online && !!control && (includeClosed || automatic),
    refetchInterval: online && visible && automatic ? 5000 : false,
  });
  const observed =
    online &&
    control &&
    query.isSuccess &&
    query.data?.revision === control.revision &&
    query.data.mode === control.dispatch_mode
      ? query.data
      : undefined;
  const label =
    !online || !control
      ? "待核实"
      : !automatic
        ? "关闭"
        : observed?.state === "ready"
          ? "已开启，可派发"
          : observed?.state === "waiting"
            ? "已开启，等待就绪"
            : "已开启，状态待核实";
  return { observed, label };
}

export function DispatchStatus() {
  const { identity } = useWorkspace();
  const { label: status } = useDispatchObservation();
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
  const { observed, label } = useDispatchObservation(true);
  const page = snapshot?.connection.snapshot.page;
  if (!page) return <p className="muted">任务自动派发状态待核实</p>;
  const { control } = page;
  const automatic = control.dispatch_mode === "auto";
  const caps = page.service.capabilities ?? [];
  const blocker = observed?.blocker;
  return (
    <>
      <div className="actions">
        <Button
          role="switch"
          aria-checked={automatic}
          aria-label="自动派发"
          size="sm"
          disabled={!online || busy || !caps.includes("set_dispatch_mode")}
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
      <div role="status">
        <p className="muted">{label}</p>
        {blocker && (
          <p className="muted">
            {blocker.diagnostic}
            {blocker.category === "project" && (
              <>
                {" "}
                ·{" "}
                <NavLink to={`/projects/${identity.project_id}/status`}>
                  前往项目状态
                </NavLink>
              </>
            )}
            {blocker.category === "environment" && (
              <>
                {" "}
                ·{" "}
                <NavLink
                  to={environmentLink(
                    identity.project_id,
                    observed?.default_profile_id,
                  )}
                >
                  前往环境准备
                </NavLink>
              </>
            )}
            {blocker.category === "profile" && (
              <>
                {" "}
                ·{" "}
                <NavLink to="/settings/execution-configs">
                  前往执行方案配置
                </NavLink>
              </>
            )}
          </p>
        )}
      </div>
      <p className="muted">
        默认执行方案：{observed?.default_profile_name || "尚未核实"}
      </p>
      {observed?.environment_readiness_required === false && (
        <p className="notice amber">环境就绪阻塞已关闭（开发者配置）</p>
      )}
    </>
  );
}
