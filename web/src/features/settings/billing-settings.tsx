import { useEffect, useMemo, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  billingAPI,
  type BillingSnapshot,
  type PricePolicy,
  type PriceRule,
} from "../../shared/api/billing";
import { Button } from "../../shared/ui/button";
import type { GenerationSettingsProps } from "./generation-settings";

const emptyPolicy = (): PricePolicy => ({
  source: "",
  currency: "USD",
  basis: "manual_flat_token_estimate",
  rules: [],
});
const rateFields = [
  ["input_uncached_per_million", "非缓存输入"],
  ["cache_read_per_million", "缓存读取"],
  ["cache_write_per_million", "缓存写入"],
  ["output_per_million", "输出（含推理）"],
] as const;
export function BillingSettings({
  home,
  csrf,
  onDirtyChange,
}: GenerationSettingsProps & { onDirtyChange?: (dirty: boolean) => void }) {
  const api = useMemo(() => billingAPI(csrf), [csrf]);
  const client = useQueryClient();
  const [busy, setBusy] = useState(false);
  const query = useQuery({
    queryKey: [home, "billing-config"],
    queryFn: ({ signal }) => api.read(signal),
    retry: false,
  });
  const [draft, setDraft] = useState<{
    base: BillingSnapshot;
    policy: PricePolicy;
  }>();
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const base = draft?.base ?? query.data;
  const policy =
    draft?.policy ??
    base?.policies.find((p) => p.version === base.active_policy_version) ??
    emptyPolicy();
  useEffect(() => {
    onDirtyChange?.(!!draft || busy);
    return () => onDirtyChange?.(false);
  }, [draft, busy, onDirtyChange]);
  function change(p: PricePolicy) {
    if (base) setDraft({ base, policy: p });
  }
  function rule(index: number, value: PriceRule) {
    change({
      ...policy,
      rules: policy.rules.map((r, i) => (i === index ? value : r)),
    });
  }
  async function write(version?: string) {
    if (!base || busy) return;
    setBusy(true);
    setError("");
    setNotice("");
    try {
      const result = version
        ? await api.activate(version, base)
        : await api.save(policy, base);
      await client.cancelQueries({ queryKey: [home, "billing-config"] });
      client.setQueryData([home, "billing-config"], result);
      setDraft(undefined);
      window.dispatchEvent(new Event("billing-settings-changed"));
      await client.invalidateQueries({
        predicate: (q) =>
          q.queryKey.includes("run-usage") ||
          q.queryKey.includes("billing-status"),
      });
      setNotice("价表已保存并立即用于重算，无需重启。");
    } catch (e) {
      setError(String(e));
    } finally {
      setBusy(false);
    }
  }
  return (
    <section
      id="billing"
      className="settings-panel billing-settings"
      aria-label="计费设置"
    >
      <h2>计费</h2>
      <p>所有项目共享。按配置价表估算，非实际账单；历史用量按当前价表重算。</p>
      <p className="muted">
        固定费率不包含上下文阶梯、服务档位、缓存多档、折扣及附加费。单价单位：每一百万
        Token。
      </p>
      {(error || query.error) && (
        <p role="alert">{error || String(query.error)}</p>
      )}
      {notice && <p role="status">{notice}</p>}
      {query.isPending ? (
        <p role="status">正在读取计费设置…</p>
      ) : (
        base && (
          <>
            <label>
              价表版本
              <select
                aria-label="价表版本"
                value={base.active_policy_version}
                disabled={busy || !!draft}
                onChange={(e) => void write(e.target.value)}
              >
                {!base.configured && <option value="">未配置</option>}
                {base.policies.map((p) => (
                  <option key={p.version} value={p.version}>
                    {p.version} · {p.currency}
                  </option>
                ))}
              </select>
            </label>
            <label>
              币种
              <input
                aria-label="币种"
                disabled={busy}
                value={policy.currency}
                maxLength={3}
                onChange={(e) =>
                  change({ ...policy, currency: e.target.value.toUpperCase() })
                }
              />
            </label>
            <label>
              价格来源
              <input
                aria-label="价格来源"
                disabled={busy}
                value={policy.source}
                onChange={(e) => change({ ...policy, source: e.target.value })}
              />
            </label>
            {policy.rules.map((r, i) => (
              <fieldset key={r.id} disabled={busy}>
                <legend>模型费率 {i + 1}</legend>
                <label>
                  Provider
                  <input
                    aria-label={`Provider ${i + 1}`}
                    value={r.provider}
                    onChange={(e) =>
                      rule(i, { ...r, provider: e.target.value })
                    }
                  />
                </label>
                <label>
                  实际模型
                  <input
                    aria-label={`实际模型 ${i + 1}`}
                    value={r.model}
                    onChange={(e) => rule(i, { ...r, model: e.target.value })}
                  />
                </label>
                <label>
                  生效时间（UTC）
                  <input
                    aria-label={`生效时间 ${i + 1}`}
                    placeholder="2026-11-01T00:00:00Z"
                    value={r.effective_from}
                    onChange={(e) =>
                      rule(i, { ...r, effective_from: e.target.value })
                    }
                  />
                </label>
                <label>
                  结束时间（UTC，可空）
                  <input
                    aria-label={`结束时间 ${i + 1}`}
                    value={r.effective_until ?? ""}
                    onChange={(e) =>
                      rule(i, { ...r, effective_until: e.target.value || null })
                    }
                  />
                </label>
                {rateFields.map(([key, label]) => (
                  <label key={key}>
                    {label}
                    <input
                      aria-label={`${label} ${i + 1}`}
                      inputMode="decimal"
                      value={r.rates[key]}
                      onChange={(e) =>
                        rule(i, {
                          ...r,
                          rates: { ...r.rates, [key]: e.target.value },
                        })
                      }
                    />
                  </label>
                ))}
                <Button
                  disabled={busy}
                  onClick={() =>
                    change({
                      ...policy,
                      rules: policy.rules.filter((_, j) => i !== j),
                    })
                  }
                >
                  移除此费率
                </Button>
              </fieldset>
            ))}
            <Button
              disabled={busy}
              onClick={() =>
                change({
                  ...policy,
                  rules: [
                    ...policy.rules,
                    {
                      id: crypto.randomUUID(),
                      provider: "",
                      model: "",
                      effective_from: "",
                      effective_until: null,
                      rates: {
                        input_uncached_per_million: "",
                        cache_read_per_million: "",
                        cache_write_per_million: "",
                        output_per_million: "",
                      },
                    },
                  ],
                })
              }
            >
              添加模型费率
            </Button>
            <Button disabled={busy || !draft} onClick={() => void write()}>
              保存为新价表版本
            </Button>
          </>
        )
      )}
      <Button
        disabled={busy || query.isFetching}
        onClick={() => {
          void query.refetch().then((r) => {
            if (!r.error) {
              setDraft(undefined);
              setError("");
            }
          });
        }}
      >
        重新读取（放弃未保存修改）
      </Button>
    </section>
  );
}
