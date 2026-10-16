import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Copy } from "lucide-react";
import { useWorkspace } from "../../app/context";
import { projectQueries } from "../../shared/api/queries";
import type { Candidate } from "../../shared/api/types";
import { Button } from "../../shared/ui/button";

export function CopyExecutionInfo({ run }: { run: Candidate }) {
  const { api, online } = useWorkspace();
  const presentation = useQuery({
    ...projectQueries(api).presentation(),
    enabled: online,
  });
  const location = presentation.data?.presentation.execution_location;
  const [copying, setCopying] = useState(false);
  const [feedback, setFeedback] = useState({ text: "", failed: false });

  async function copy(kind: "SteerLine" | "SteerRunner") {
    if (!location) return;
    const text =
      kind === "SteerLine"
        ? [
            "SteerLine",
            `Home: ${location.application_home}`,
            `Repository: ${location.repository}`,
            `Task ID: ${run.domain_task_id || run.task_id}`,
            `Candidate Run ID: ${run.run_id}`,
          ]
        : [
            "SteerRunner",
            `Executable: ${location.runner?.executable}`,
            `State directory: ${run.runner_state_dir || "尚未记录"}`,
            `Runner Run ID: ${run.execution_id || "尚未生成"}`,
            `Attempt ID: ${run.attempt_id || "尚未生成"}`,
          ];
    setCopying(true);
    setFeedback({ text: "", failed: false });
    try {
      if (!navigator.clipboard?.writeText) {
        throw new Error("当前浏览器不支持剪贴板写入");
      }
      await navigator.clipboard.writeText(text.join("\n"));
      setFeedback({ text: `已复制 ${kind} 执行信息`, failed: false });
    } catch (error) {
      setFeedback({
        text: `复制失败：${error instanceof Error ? error.message : String(error)}`,
        failed: true,
      });
    } finally {
      setCopying(false);
    }
  }

  return (
    <div className="copy-execution-info">
      <div className="actions">
        <Button
          size="sm"
          disabled={!location || copying}
          title={!location ? "执行定位信息尚未就绪" : undefined}
          onClick={() => void copy("SteerLine")}
        >
          <Copy size={13} />
          复制 SteerLine 执行信息
        </Button>
        {location?.runner && (
          <Button
            size="sm"
            disabled={copying}
            onClick={() => void copy("SteerRunner")}
          >
            <Copy size={13} />
            复制 SteerRunner 执行信息
          </Button>
        )}
      </div>
      {feedback.text && (
        <p
          className={feedback.failed ? "notice amber" : "muted"}
          role={feedback.failed ? "alert" : "status"}
        >
          {feedback.text}
        </p>
      )}
      {presentation.error && (
        <p className="notice amber" role="alert">
          执行定位信息读取失败：{String(presentation.error)}
        </p>
      )}
    </div>
  );
}
