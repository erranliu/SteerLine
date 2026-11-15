import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import { Button } from "../../shared/ui/button";

type Settings = {
  directory: string;
  effective: string;
  default_directory: string;
};

export function WorktreeSettings() {
  const { api, identity, online, submit } = useWorkspace();
  const [draft, setDraft] = useState<string>();
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState("");
  const settings = useQuery({
    queryKey: [
      "project-worktrees",
      api.home,
      identity.project_id,
      identity.instance_id,
    ],
    queryFn: ({ signal }) => api.get<Settings>("/settings/worktrees", signal),
    enabled: online,
  });
  const current = settings.data;
  const value = draft ?? current?.directory ?? "";
  const disabled = !online || pending || !current;

  async function save(directory: string) {
    setPending(true);
    setMessage("");
    try {
      const result = await submit(
        api.envelope(
          "maintenance_set_worktree_directory",
          identity.project_id,
          1,
          { directory },
        ),
      );
      if (result?.status !== "applied") {
        setMessage(result?.diagnostic || "保存结果待核实");
        return;
      }
      await settings.refetch();
      setDraft(undefined);
      setMessage("已保存，新建 worktree 将使用此设置。");
    } catch (error) {
      setMessage(String(error));
    } finally {
      setPending(false);
    }
  }

  return (
    <section
      className="settings-panel"
      aria-labelledby="worktree-settings-title"
    >
      <h2 id="worktree-settings-title">本机 Worktree 根目录</h2>
      <p>
        应用于 SteerLine 在本机为此项目创建的 worktree。Runner 使用自己的设置。
      </p>
      {current && (
        <p>
          当前生效目录 <code>{current.effective}</code>
        </p>
      )}
      {settings.error && <p role="alert">{String(settings.error)}</p>}
      <form
        onSubmit={(event) => {
          event.preventDefault();
          void save(value.trim());
        }}
      >
        <label htmlFor="worktree-directory">目录</label>{" "}
        <input
          id="worktree-directory"
          value={value}
          disabled={disabled}
          placeholder={current?.default_directory}
          onChange={(event) => setDraft(event.target.value)}
        />
        <div className="actions">
          <Button
            type="submit"
            disabled={
              disabled || !value.trim() || value.trim() === current?.directory
            }
          >
            保存目录
          </Button>
          <Button
            type="button"
            disabled={disabled || !current?.directory}
            onClick={() => void save("")}
          >
            恢复默认目录
          </Button>
        </div>
      </form>
      {message && <p role="status">{message}</p>}
    </section>
  );
}
