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

export type GenerationSettingsProps = {
  home: string;
  csrf: string;
  loadedDigest?: string;
  applicationConnected: boolean;
};

export function GenerationSettings({
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
  const [draft, setDraft] = useState<{ base: ConfigSnapshot; limit: string }>();
  const base = draft?.base ?? query.data;
  const limit =
    draft?.limit ?? String(base?.config.max_generate_concurrency ?? 5);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const dirty = !!draft;
  const valid = Number.isSafeInteger(Number(limit)) && Number(limit) > 0;
  useEffect(() => {
    onDirtyChange?.(dirty || saving);
    return () => onDirtyChange?.(false);
  }, [onDirtyChange, dirty, saving]);
  const pendingRestart =
    applicationConnected &&
    !!loadedDigest &&
    !!query.data &&
    loadedDigest !== query.data.saved_digest;

  async function save() {
    if (!base || !dirty || !valid || !base.writable || saving) return;
    setSaving(true);
    setError("");
    setNotice("");
    try {
      const result = await saveExecutionConfig(
        api,
        { ...base.config, max_generate_concurrency: Number(limit) },
        base.saved_digest,
      );
      await client.cancelQueries({
        queryKey: [home, "execution-config"],
        exact: true,
      });
      client.setQueryData([home, "execution-config"], result);
      setDraft(undefined);
      void client.invalidateQueries({ queryKey: [home, "projects"] });
      setNotice("生成并发已保存。重启 SteerLine 后生效。");
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
  async function reload() {
    const result = await query.refetch();
    if (result.data && !result.error) {
      setDraft(undefined);
      setError("");
      setNotice("");
    }
  }
  return (
    <section className="settings-panel" aria-label="生成并发设置">
      {!onDirtyChange && <NavigationGuard dirty={dirty || saving} />}
      <h2>生成并发</h2>
      <p className="muted">材料与测试安排生成使用以下并发上限。</p>
      <p className="settings-scope">所有项目共享</p>
      {query.isPending && <p role="status">正在读取生成并发配置…</p>}
      {(error || query.error) && (
        <div className="notice amber" role="alert">
          <p>{error || String(query.error)}</p>
          <Button
            size="sm"
            disabled={saving || query.isFetching}
            onClick={() => void reload()}
          >
            重新读取配置（放弃当前输入）
          </Button>
        </div>
      )}
      {base && (
        <>
          {!base.writable && (
            <p className="notice amber">{base.reason || "当前配置只读"}</p>
          )}
          <div className="configuration-field">
            <label htmlFor="generate-concurrency">生成最大并发数</label>
            <input
              id="generate-concurrency"
              type="number"
              min={1}
              step={1}
              value={limit}
              disabled={!base.writable || saving}
              onChange={(e) => {
                setDraft(
                  e.target.value ===
                    String(base.config.max_generate_concurrency ?? 5)
                    ? undefined
                    : { base, limit: e.target.value },
                );
                setNotice("");
              }}
              aria-describedby="generate-concurrency-help"
            />
            <p id="generate-concurrency-help" className="muted">
              所有项目的自动生成、一键生成和单项重试共用队列，默认最多同时生成 5
              项。保存后重启 SteerLine 生效。
            </p>
          </div>
          {dirty && !valid && <p className="notice amber">请输入正整数。</p>}
          <div className="actions">
            <Button
              disabled={!base.writable || saving || !dirty || !valid}
              onClick={() => void save()}
            >
              {saving ? "正在保存…" : "保存生成并发"}
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
              取消修改
            </Button>
          </div>
          {pendingRestart && (
            <p className="notice amber" role="status">
              配置已保存，待重启生效
            </p>
          )}
          {(!applicationConnected || !loadedDigest) && (
            <p className="muted">服务配置生效状态未知</p>
          )}
        </>
      )}
      <p role="status">{notice}</p>
    </section>
  );
}
