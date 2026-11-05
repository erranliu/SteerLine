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
        queryKey: [home, "execution-config-status"],
      });
      setNotice(
        "采集设置已保存，是否生效以上方运行中策略为准。",
      );
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <section
      id="usage-collection"
      className="settings-panel"
      aria-label="用量采集设置"
    >
      <h2>用量采集</h2>
      <p>采集授权与价格设置分别维护。远端读取默认关闭，修改后须正式重启。</p>
      <p role="status">
        {ready
          ? pending
            ? "已保存，待正式重启"
            : "运行中策略已生效"
          : "运行中策略未知或服务离线"}
      </p>
      {effective.data?.diagnostic && (
        <p role="status">{effective.data.diagnostic}</p>
      )}
      {(error || config.error || effective.error) && (
        <p role="alert">{error || String(config.error || effective.error)}</p>
      )}
      {notice && <p role="status">{notice}</p>}
      {base && (
        <>
          <label>
            <input
              type="checkbox"
              checked={policy.enabled}
              disabled={busy || !base.writable}
              onChange={(e) => change({ ...policy, enabled: e.target.checked })}
            />
            启用 Token 采集
          </label>
          <label>
            <input
              type="checkbox"
              checked={policy.remote_log_access.mode === "temporary"}
              disabled={busy || !base.writable}
              onChange={(e) =>
                change({
                  ...policy,
                  remote_log_access: {
                    mode: e.target.checked ? "temporary" : "disabled",
                    grants: e.target.checked
                      ? policy.remote_log_access.grants
                      : [],
                  },
                })
              }
            />
            显式允许临时远端日志读取
          </label>
          <p className="muted">
            仅选择 Runner 已核实的主机与 session
            叶目录。临时目录登记不等于只读权限或自动过期；回收状态由 Runner
            核实。
          </p>
          {policy.remote_log_access.grants.map((g) => (
            <div key={g.environment + g.host_id + g.session_roots.join(";")}>
              <p className="path">
                {g.environment} · {g.host_id}
              </p>
              {g.session_roots.map((root) => (
                <p key={root} className="path">
                  {root}
                </p>
              ))}
              <Button
                disabled={busy || !base.writable}
                onClick={() =>
                  change({
                    ...policy,
                    remote_log_access: {
                      ...policy.remote_log_access,
                      grants: policy.remote_log_access.grants.filter(
                        (x) => x !== g,
                      ),
                    },
                  })
                }
              >
                移除此授权
              </Button>
            </div>
          ))}
          {(effective.data?.requirements ?? []).map((r) => (
            <div key={r.run_id + r.source_ref}>
              <p className="path">
                {r.environment} · {r.host_id}
                <br />
                {r.session_root}
              </p>
              <Button
                disabled={
                  !ready ||
                  busy ||
                  !base.writable ||
                  policy.remote_log_access.mode !== "temporary" ||
                  policy.remote_log_access.grants.some(
                    (g) =>
                      g.environment === r.environment &&
                      g.host_id === r.host_id &&
                      g.session_roots.includes(r.session_root),
                  )
                }
                onClick={() =>
                  change({
                    ...policy,
                    remote_log_access: {
                      mode: "temporary",
                      grants: [
                        ...policy.remote_log_access.grants,
                        {
                          environment: r.environment,
                          host_id: r.host_id,
                          session_roots: [r.session_root],
                        },
                      ],
                    },
                  })
                }
              >
                授权此已核实日志叶目录
              </Button>
            </div>
          ))}
          {!effective.data?.requirements?.length && (
            <p>尚无可核实的远端授权需求；不能自行填写宽泛目录。</p>
          )}
          <Button
            disabled={!draft || busy || !base.writable}
            onClick={() => void save()}
          >
            保存采集设置
          </Button>
        </>
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
        重新读取策略与授权需求
      </Button>
    </section>
  );
}
