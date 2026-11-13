import { useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  executionConfigAPI,
  saveExecutionConfig,
  type ConfigSnapshot,
  type Catalog,
} from "../../shared/api/execution-configs";
import { Button } from "../../shared/ui/button";
import type { GenerationSettingsProps } from "./generation-settings";

export function SteerAgentSettings({
  home,
  csrf,
  loadedDigest,
  applicationConnected,
  onDirtyChange,
}: GenerationSettingsProps & { onDirtyChange?: (dirty: boolean) => void }) {
  const api = useMemo(() => executionConfigAPI(csrf), [csrf]);
  const client = useQueryClient();
  const query = useQuery({
    queryKey: [home, "execution-config"],
    queryFn: ({ signal }) => api.read(signal),
    retry: false,
  });
  const [draft, setDraft] = useState<{
    base: ConfigSnapshot;
    model: string;
    reasoning_effort: string;
  }>();
  const [catalog, setCatalog] = useState<Catalog>();
  const [working, setWorking] = useState(false);
  const [message, setMessage] = useState("");
  const base = draft?.base ?? query.data;
  const value = draft ??
    base?.config.steer_agent ?? {
      model: "gpt-6.1-sol",
      reasoning_effort: "high",
    };
  useEffect(() => {
    onDirtyChange?.(!!draft || working);
    return () => onDirtyChange?.(false);
  }, [draft, working, onDirtyChange]);
  async function discover() {
    setWorking(true);
    setMessage("");
    try {
      setCatalog(
        await api.discover({
          kind: "models",
          adapter: "codex-app-server",
          host: "local",
          agent: "codex",
          refresh: true,
          profile_id: base?.config.execution_profiles.find(
            (p) =>
              p.execution.adapter === "codex-app-server" &&
              p.execution.target.kind === "local",
          )?.id,
        }),
      );
    } catch (e) {
      setMessage(String(e));
    } finally {
      setWorking(false);
    }
  }
  async function save() {
    if (!base || !draft) return;
    setWorking(true);
    setMessage("");
    try {
      const updated = await saveExecutionConfig(
        api,
        {
          ...base.config,
          steer_agent: {
            model: draft.model,
            reasoning_effort: draft.reasoning_effort,
          },
        },
        base.saved_digest,
      );
      client.setQueryData([home, "execution-config"], updated);
      await client.invalidateQueries({
        queryKey: [home, "projects"],
      });
      setDraft(undefined);
      setMessage("SteerAgent 设置已保存，重启 SteerLine 后生效。");
    } catch (e) {
      setMessage(String(e));
    } finally {
      setWorking(false);
    }
  }
  const model = catalog?.models?.find((m) => m.id === value.model);
  const valid =
    catalog?.state === "ready" &&
    !!model?.efforts.some((e) => e.value === value.reasoning_effort);
  return (
    <section className="settings-panel" aria-label="SteerAgent 设置">
      <h2>SteerAgent</h2>
      <p className="muted">
        所有项目的 SteerAgent 模式共享本地 Codex 模型和思考强度。重启后生效。
      </p>
      {query.error && <p role="alert">{String(query.error)}</p>}
      <p>
        当前设置：{value.model} / {value.reasoning_effort}
      </p>
      <Button
        disabled={working || !base?.writable}
        onClick={() => void discover()}
      >
        读取本地 Codex 模型
      </Button>
      {catalog?.diagnostic && <p role="alert">{catalog.diagnostic}</p>}
      <label>
        模型
        <select
          aria-label="SteerAgent 模型"
          value={value.model}
          disabled={working || !base?.writable || catalog?.state !== "ready"}
          onChange={(e) =>
            base &&
            setDraft({
              base,
              model: e.target.value,
              reasoning_effort:
                catalog?.models?.find((m) => m.id === e.target.value)
                  ?.default_effort ?? "high",
            })
          }
        >
          <option value={value.model}>{value.model}</option>
          {catalog?.models
            ?.filter((m) => m.id !== value.model)
            .map((m) => (
              <option key={m.id} value={m.id}>
                {m.label || m.id}
              </option>
            ))}
        </select>
      </label>
      <label>
        思考强度
        <select
          aria-label="SteerAgent 思考强度"
          value={value.reasoning_effort}
          disabled={working || !base?.writable || !model}
          onChange={(e) =>
            base &&
            setDraft({
              base,
              model: value.model,
              reasoning_effort: e.target.value,
            })
          }
        >
          <option value={value.reasoning_effort}>
            {value.reasoning_effort}
          </option>
          {model?.efforts
            .filter((e) => e.value !== value.reasoning_effort)
            .map((e) => (
              <option key={e.value} value={e.value}>
                {e.label}
              </option>
            ))}
        </select>
      </label>
      {base && !base.writable && <p role="alert">{base.reason}</p>}
      <div className="actions">
        <Button
          disabled={!draft || !valid || working || !base?.writable}
          onClick={() => void save()}
        >
          保存SteerAgent 设置
        </Button>
        <Button
          variant="outline"
          disabled={!draft || working}
          onClick={() => setDraft(undefined)}
        >
          取消SteerAgent 修改
        </Button>
      </div>
      {applicationConnected &&
        loadedDigest &&
        query.data?.saved_digest !== loadedDigest && (
          <p role="status">配置已保存，待重启生效</p>
        )}
      <p role="status">{message}</p>
    </section>
  );
}
