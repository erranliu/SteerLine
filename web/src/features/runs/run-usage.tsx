import { useEffect, useRef } from "react";
import { useQuery } from "@tanstack/react-query";
import { billingStatus } from "../../shared/api/billing";
import { usageCollection } from "../../shared/api/usage-collection";
import { useWorkspace } from "../../app/context";
import { projectQueries } from "../../shared/api/queries";
import { useQueryVisibility } from "../../shared/query-visibility";
import type { Candidate } from "../../shared/api/types";
import type { RunUsageView } from "../../shared/api/run-usage";
import { Button } from "../../shared/ui/button";
import "./run-usage.css";

const reasons: Record<string, string> = {
  USAGE_DETAILS_UNSUPPORTED: "当前 Runner 用量明细契约不足，Token 仍可查看",
  USAGE_UNSUPPORTED: "当前 Runner 暂不支持用量查询",
  COST_CALCULATION_FAILED: "费用计算失败，Token 用量仍可查看",
  PRICE_POLICY_NOT_CONFIGURED: "未配置价表",
  PRICE_POLICY_INVALID: "价表配置无效或无法读取",
  COST_QUERY_FAILED: "费用读取失败，可刷新重试",
  PRICE_NOT_FOUND: "部分模型缺少适用价格",
  CONFIGURED_MODEL_UNASSIGNED: "部分用量未分配配置模型或阶段，未计价",
  CONFIGURED_USAGE_UNSUPPORTED:
    "当前 Runner 不支持按阶段配置模型估价，Token 仍可查看",
  PRICE_AMBIGUOUS: "同一配置模型匹配到多个计价供应商，请调整价表",
  PRICE_PERIOD_UNKNOWN: "候选创建时间未知，无法选择价格时段",
  USAGE_UNAVAILABLE: "暂无可信用量",
  USAGE_PARTIAL: "用量覆盖不完整",
  USAGE_NOT_FINALIZED: "当前用量尚未收敛",
  COLLECTION_DISABLED: "用量采集已关闭",
  COLLECTION_NOT_OBSERVED: "尚未完成首次用量观察",
  PRODUCTION_BINDING_UNSUPPORTED: "当前执行方式暂不支持用量采集",
  FORMAT_UNSUPPORTED: "当前日志格式暂不支持",
  HOST_OFFLINE: "执行主机离线",
  LOG_NOT_FOUND: "尚未取得用量日志",
  LOG_ACCESS_DENIED: "无法读取用量日志",
  LOG_TAIL_INCOMPLETE: "用量日志尚未完整落盘",
  COVERAGE_INCOMPLETE: "用量覆盖不完整",
  USAGE_CONFLICT: "用量来源存在冲突",
  ACCESS_CLEANUP_PENDING: "远端用量读取授权待回收",
};
const reason = (code: string) => reasons[code] ?? `用量诊断：${code}`;

// Decimal strings remain exact until the final display rounding.
export function formatAmount(value: string, currency: string) {
  const [whole, fraction = ""] = value.split(".");
  const digits = (fraction + "00000").slice(0, 5);
  const positive = /[1-9]/.test(value);
  if (BigInt(whole) === 0n && Number(digits.slice(0, 4)) === 0 && positive)
    return `< ${currency} 0.0001`;
  const rounded =
    BigInt(whole) * 10000n +
    BigInt(digits.slice(0, 4)) +
    (digits[4] >= "5" ? 1n : 0n);
  return `${currency} ${rounded / 10000n}.${(rounded % 10000n).toString().padStart(4, "0")}`;
}
const tokens = (n: number | null | undefined) =>
  n == null ? "—" : n.toLocaleString("zh-CN");

export function usagePollInterval(view: RunUsageView | undefined) {
  if (view?.reason === "USAGE_UNSUPPORTED") return false;
  if (
    view?.usage?.status === "unavailable" &&
    view.usage.issues.some(({ code }) =>
      [
        "PRODUCTION_BINDING_UNSUPPORTED",
        "FORMAT_UNSUPPORTED",
        "COLLECTION_DISABLED",
      ].includes(code),
    )
  )
    return false;
  if (
    view?.usage?.status === "available" &&
    view.usage.finalized &&
    view.usage.access_cleanup_status !== "pending" &&
    view.cost_reason !== "COST_QUERY_FAILED"
  )
    return false;
  return 5000;
}

export function RunUsage({ run }: { run: Candidate }) {
  const { api, online } = useWorkspace();
  const statusEnabled = useQueryVisibility(online, [
    api.home,
    "usage-settings-status",
  ]);
  const prices = useQuery({
    queryKey: [api.home, "billing-status"],
    queryFn: ({ signal }) => billingStatus(signal),
    enabled: statusEnabled,
    refetchInterval: statusEnabled ? 5000 : false,
    retry: false,
  });
  const collection = useQuery({
    queryKey: [api.home, "usage-collection-status"],
    queryFn: ({ signal }) => usageCollection(signal, true),
    enabled: statusEnabled,
    refetchInterval: statusEnabled ? 5000 : false,
    retry: false,
  });
  const generation = prices.data?.generation;
  const options = projectQueries(api).runUsage(
    run,
    generation,
    collection.data?.usage_policy_revision,
  );
  const enabled = useQueryVisibility(
    online && !!run.execution_id,
    options.queryKey,
  );
  const query = useQuery({
    ...options,
    enabled,
    refetchInterval: (q) =>
      q.state.error ? 5000 : usagePollInterval(q.state.data),
    refetchIntervalInBackground: false,
  });
  useEffect(() => {
    const changed = () => {
      void prices.refetch();
    };
    window.addEventListener("billing-settings-changed", changed);
    return () =>
      window.removeEventListener("billing-settings-changed", changed);
  }, [prices.refetch]);
  const identity = JSON.stringify(options.queryKey.slice(0, -2));
  const retained = useRef<{ identity: string; view: RunUsageView } | undefined>(
    undefined,
  );
  useEffect(() => {
    if (query.data) retained.current = { identity, view: query.data };
  }, [identity, query.data]);
  const previous =
    retained.current?.identity === identity ? retained.current.view : undefined;
  const view = query.data ?? previous;
  const showingPrevious = !query.data && !!previous;
  const usage = view?.usage;
  const cost = view?.cost;
  const amount =
    cost?.status === "estimated"
      ? cost.total_estimated_amount
      : cost?.covered_estimated_amount;
  const formatted =
    amount != null && cost ? formatAmount(amount, cost.currency) : "";
  const unsupported =
    view?.reason === "USAGE_UNSUPPORTED" ||
    usage?.issues.some((i) => i.code === "PRODUCTION_BINDING_UNSUPPORTED") ||
    usage?.issues.some((i) => i.code === "FORMAT_UNSUPPORTED");
  const label =
    amount != null && cost
      ? `${cost.status === "estimated" ? "费用估算" : "已计价部分"} ${formatted.startsWith("<") ? "" : "≈ "}${formatted}`
      : usage?.status === "pending"
        ? "等待用量数据"
        : unsupported
          ? "用量暂不支持"
          : !run.execution_id
            ? "等待执行创建"
            : !view
              ? query.isPending && enabled
                ? "正在读取用量…"
                : "用量暂不可用"
              : "费用未估算";
  const issues = [
    ...new Set([
      ...(usage?.issues.map((i) => i.code) ?? []),
      ...(cost?.issues ?? []),
      ...(view?.reason ? [view.reason] : []),
      ...(view?.cost_reason ? [view.cost_reason] : []),
    ]),
  ];
  return (
    <section className="run-usage" aria-label="用量与费用">
      <details>
        <summary>
          <strong>{label}</strong>
          <span className="muted">
            Token {tokens(usage?.tokens?.total)}
            {usage?.status === "partial" ? " · 已核实部分" : ""}
          </span>
          {view?.cost_reason === "PRICE_POLICY_NOT_CONFIGURED" && (
            <a href="/settings#billing">未配置价表 · 打开计费设置</a>
          )}
          <span className="usage-expand">用量与费用</span>
        </summary>
        <dl>
          <div>
            <dt>输入 Token（含缓存）</dt>
            <dd>{tokens(usage?.tokens?.input_total)}</dd>
          </div>
          <div>
            <dt>缓存读取 / 写入</dt>
            <dd>
              {tokens(usage?.tokens?.cache_read)} /{" "}
              {tokens(usage?.tokens?.cache_write)}
            </dd>
          </div>
          <div>
            <dt>输出 Token（含推理）</dt>
            <dd>{tokens(usage?.tokens?.output)}</dd>
          </div>
          <div>
            <dt>完整计价记录 / 全部记录</dt>
            <dd>
              {cost
                ? `${cost.fully_priced_records} / ${cost.record_count}`
                : "—"}
            </dd>
          </div>
          <div>
            <dt>统计范围</dt>
            <dd>
              该候选累计主执行会话 · 不含独立 parser、子 agent 及工具内部模型
            </dd>
          </div>
          <div>
            <dt>部分计价 / 未计价记录</dt>
            <dd>
              {cost
                ? `${cost.partially_priced_records} / ${cost.unpriced_records}`
                : "—"}
            </dd>
          </div>
          <div>
            <dt>价格匹配时间（候选创建）</dt>
            <dd>
              {cost?.price_at && !cost.price_at.startsWith("0001-")
                ? new Date(cost.price_at).toLocaleString("zh-CN", {
                    hour12: false,
                  })
                : "—"}
            </dd>
          </div>
          <div>
            <dt>价表</dt>
            <dd title={cost?.pricing_policy_digest}>
              {cost?.pricing_policy
                ? `${cost.pricing_policy} · ${cost.pricing_policy_version}`
                : "—"}
            </dd>
          </div>
          <div>
            <dt>价格来源</dt>
            <dd>{cost?.pricing_source ?? "—"}</dd>
          </div>
          <div>
            <dt>数据观察时间</dt>
            <dd>
              {usage?.observed_at ? (
                <time dateTime={usage.observed_at}>
                  {new Date(usage.observed_at).toLocaleString("zh-CN", {
                    hour12: false,
                  })}
                </time>
              ) : (
                "—"
              )}
            </dd>
          </div>
        </dl>
        {cost?.matches.map((match) => (
          <p
            className="muted"
            key={JSON.stringify([
              match.rule_id,
              match.step_id,
              match.configured_reasoning_effort,
            ])}
            title={match.rule_id}
          >
            {match.step_id} · {match.provider} / {match.model}
            {match.configured_reasoning_effort
              ? ` · ${match.configured_reasoning_effort}`
              : ""}{" "}
            · {match.records} 条记录 ·{" "}
            {formatAmount(match.amount, cost.currency)}
          </p>
        ))}
        {issues.map((code) => (
          <p className="muted" key={code}>
            {reason(code)}
          </p>
        ))}
        {cost?.assumptions?.map((text) => (
          <p className="muted" key={text}>
            {text}
          </p>
        ))}
        <p className="output-hint">
          按约定配置模型估算，非实际账单；历史用量按当前价表重算，以候选创建时间匹配价格。未分配模型的用量保留为未计价，不含订阅额度结算及外部
          API 费用。
        </p>
        <Button
          size="sm"
          variant="ghost"
          disabled={!enabled || query.isFetching}
          onClick={() => void query.refetch()}
        >
          刷新用量
        </Button>
      </details>
      {query.error && (
        <p className="muted" role="status">
          用量刷新失败，保留最后观察；可展开重试。
        </p>
      )}
      {showingPrevious && (
        <p className="muted" role="status">
          暂显示原价表版本的最后估算，当前设置的重算尚未成功。
        </p>
      )}
      {(prices.error || collection.error) && view && (
        <p className="muted" role="status">
          设置状态刷新失败，已显示的价表版本保留；当前价格或采集策略尚未核实。
        </p>
      )}
      {!online && <p className="muted">服务离线，已暂停刷新。</p>}
      {view?.cost_reason === "COST_QUERY_FAILED" && (
        <p className="muted" role="status">
          费用读取失败，Token 用量仍可查看。
        </p>
      )}
    </section>
  );
}
