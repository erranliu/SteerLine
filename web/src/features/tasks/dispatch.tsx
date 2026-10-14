import { useWorkspace } from "../../app/context";
import { Button } from "../../shared/ui/button";
import { Pause, Play } from "lucide-react";
export function Dispatch() {
  const { online, snapshot, send, busy } = useWorkspace();
  const page = snapshot?.connection.snapshot.page;
  if (!page) return null;
  const { control } = page;
  const caps = page.service.capabilities ?? [];
  return (
    <div className="dispatch-bar">
      <div>
        <span className="connection-dot" />
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
      {control.blockers?.length > 0 && (
        <p className="muted">
          {control.blockers.map((b) => b.Reason).join("；")}
        </p>
      )}
    </div>
  );
}
