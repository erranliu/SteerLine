import { useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  executionConfigAPI,
  saveExecutionConfig,
  stable,
  type ConfigSnapshot,
} from "../../shared/api/execution-configs";
import {
  defaultUsagePolicy,
  usageCollection,
  type UsagePolicy,
} from "../../shared/api/usage-collection";
import { Button } from "../../shared/ui/button";
import type { GenerationSettingsProps } from "./generation-settings";

export function UsageSettings({
  home,
  csrf,
  applicationConnected,
  onDirtyChange,
}: GenerationSettingsProps & { onDirtyChange?: (dirty: boolean) => void }) {
  const api = useMemo(() => executionConfigAPI(csrf), [csrf]);
  const client = useQueryClient();
  const [busy, setBusy] = useState(false);
  const config = useQuery({
    queryKey: [home, "execution-config"],
    queryFn: ({ signal }) => api.read(signal),
    retry: false,
  });
  const effective = useQuery({
    queryKey: [home, "usage-collection"],
    queryFn: ({ signal }) => usageCollection(signal),
    retry: false,
    enabled: applicationConnected,
  });
  const [draft, setDraft] = useState<{
    base: ConfigSnapshot;
    policy: UsagePolicy;
  }>();
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const base = draft?.base ?? config.data;
  const saved =
    (base?.config.usage_policy as unknown as UsagePolicy | undefined) ??
    defaultUsagePolicy();
  const policy = draft?.policy ?? saved;
  useEffect(() => {
    onDirtyChange?.(!!draft || busy);
    return () => onDirtyChange?.(false);
  }, [draft, busy, onDirtyChange]);
  const change = (p: UsagePolicy) => {
    if (base) setDraft({ base, policy: p });
  };
  const ready =
    applicationConnected &&
    !effective.error &&
    effective.data?.status === "ready";
  const pending =
    ready && stable(saved) !== stable(effective.data!.usage_policy);
  async function save() {
    if (!draft || busy) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const result = await saveExecutionConfig(
        api,
        { ...draft.base.config, usage_policy: draft.policy },
        draft.base.saved_digest,
      );
      await client.cancelQueries({ queryKey: [home, "execution-config"] });
      client.setQueryData([home, "execution-config"], result);
      setDraft(undefined);
      void client.invalidateQueries({
        queryKey: [home, "projects"],
      });
      setNotice("采集设置已保存，是否生效以上方运行中策略为准。");
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <section
      id="usage-collection"
      className="settings-panel usage-settings"
      aria-label="用量采集设置"
    >
      <h2>用量采集</h2>
      <p>
        Token
        采集默认启用，自动读取当前运行已核实的本机和远端会话日志。修改采集开关后须正式重启。
      </p>
      <div className="billing-actions">
        {base && (
          <label className="usage-switch">
            启用 Token 采集
            <input
              type="checkbox"
              role="switch"
              checked={policy.enabled}
              disabled={busy || !base.writable}
              onChange={(e) => change({ ...policy, enabled: e.target.checked })}
            />
          </label>
        )}
        <p
          role="status"
          className={ready && !pending ? "usage-effective" : undefined}
        >
          {ready
            ? pending
              ? "已保存，待正式重启"
              : "运行中策略已生效"
            : "运行中策略未知或服务离线"}
        </p>
      </div>
      {(error || config.error || effective.error) && (
        <p role="alert">{error || String(config.error || effective.error)}</p>
      )}
      {notice && <p role="status">{notice}</p>}
      <div className="billing-actions">
        {base && (
          <Button
            disabled={!draft || busy || !base.writable}
            onClick={() => void save()}
          >
            保存采集设置
          </Button>
        )}
        <Button
          disabled={busy || effective.isFetching}
          onClick={() => {
            void effective.refetch();
            void config.refetch().then((r) => {
              if (!r.error) setDraft(undefined);
            });
          }}
        >
          重新读取采集状态
        </Button>
      </div>
    </section>
  );
}
