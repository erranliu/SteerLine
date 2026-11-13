import { useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError } from "../../shared/api/client";
import {
  executionConfigAPI,
  saveExecutionConfig,
  type ConfigSnapshot,
} from "../../shared/api/execution-configs";
import { Button } from "../../shared/ui/button";
import { NavigationGuard } from "../../shared/ui/navigation-guard";
import type { GenerationSettingsProps } from "./generation-settings";

export function DeveloperSettings({
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
  const [draft, setDraft] = useState<{ base: ConfigSnapshot; skip: boolean }>();
  const base = draft?.base ?? query.data;
  const skip =
    draft?.skip ?? base?.config.developer?.skip_environment_readiness ?? false;
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const dirty = !!draft;
  const pendingRestart =
    applicationConnected &&
    !!loadedDigest &&
    !!query.data &&
    loadedDigest !== query.data.saved_digest;
  useEffect(() => {
    onDirtyChange?.(dirty || saving);
    return () => onDirtyChange?.(false);
  }, [onDirtyChange, dirty, saving]);

  async function reload() {
    const result = await query.refetch();
    if (result.data && !result.error) {
      setDraft(undefined);
      setError("");
      setNotice("");
    }
  }
  async function save() {
    if (
      !base ||
      !base.developer_available ||
      !base.writable ||
      !dirty ||
      saving
    )
      return;
    setSaving(true);
    setError("");
    setNotice("");
    try {
      const result = await saveExecutionConfig(
        api,
        {
          ...base.config,
          developer: {
            ...base.config.developer,
            skip_environment_readiness: skip,
          },
        },
        base.saved_digest,
      );
      await client.cancelQueries({
        queryKey: [home, "execution-config"],
        exact: true,
      });
      client.setQueryData([home, "execution-config"], result);
      setDraft(undefined);
      void client.invalidateQueries({ queryKey: [home, "projects"] });
      setNotice("开发者配置已保存。重启 SteerLine 后生效。");
    } catch (e) {
      setError(
        e instanceof ApiError && e.status === 412
          ? "配置已被其他页面修改，当前输入已保留。请重新读取后再修改并保存。"
          : String(e),
      );
    } finally {
      setSaving(false);
    }
  }

  if (!query.data?.developer_available) return null;
  return (
    <section className="settings-panel" aria-label="开发者配置">
      {!onDirtyChange && <NavigationGuard dirty={dirty || saving} />}
      <h2>开发者配置</h2>
      <p className="settings-scope">所有项目共享</p>
      <p className="muted">
        保存后重启生效；配置变更后，新的执行受理将暂停至重启完成。
      </p>
      {(error || query.error) && (
        <div className="notice amber" role="alert">
          <p>{error || String(query.error)}</p>
          <Button
            size="sm"
            disabled={saving || query.isFetching}
            onClick={() => void reload()}
          >
            重新读取开发者配置（放弃当前输入）
          </Button>
        </div>
      )}
      {base && (
        <>
          {!base.writable && (
            <p className="notice amber">{base.reason || "当前配置只读"}</p>
          )}
          <div className="configuration-field">
            <label htmlFor="environment-readiness-gate">
              环境未就绪时阻止执行
            </label>
            <input
              id="environment-readiness-gate"
              type="checkbox"
              role="switch"
              checked={!skip}
              aria-checked={!skip}
              aria-describedby="environment-readiness-gate-help"
              disabled={!base.writable || saving}
              onChange={(e) => {
                const next = !e.target.checked;
                setDraft(
                  next ===
                    (base.config.developer?.skip_environment_readiness ?? false)
                    ? undefined
                    : { base, skip: next },
                );
                setNotice("");
              }}
            />
            <p id="environment-readiness-gate-help" className="muted">
              默认开启。关闭后，未检测、检测未通过或结果失效仍允许启动任务；环境检测结果继续如实显示。
            </p>
          </div>
          <div className="actions">
            <Button
              disabled={!base.writable || saving || !dirty}
              onClick={() => void save()}
            >
              {saving ? "正在保存…" : "保存开发者配置"}
            </Button>
            <Button
              variant="outline"
              disabled={saving || !dirty}
              onClick={() => {
                setDraft(undefined);
                setError("");
                setNotice("");
              }}
            >
              取消开发者配置修改
            </Button>
          </div>
          <p className="muted">
            当前生效状态：
            {!applicationConnected || !loadedDigest
              ? "未知"
              : pendingRestart
                ? "配置已保存，待重启核实"
                : query.data.config.developer?.skip_environment_readiness
                  ? "环境就绪阻塞已关闭"
                  : "环境就绪阻塞已开启"}
          </p>
          {dirty && <p className="muted">开发者配置有未保存的修改</p>}
          {pendingRestart && (
            <p className="notice amber" role="status">
              全局配置已保存，待重启生效
            </p>
          )}
        </>
      )}
      <p role="status">{notice}</p>
    </section>
  );
}
