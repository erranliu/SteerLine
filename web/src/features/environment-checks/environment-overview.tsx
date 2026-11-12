import { useEffect, useState } from "react";
import { queryOptions, useQueries } from "@tanstack/react-query";
import { useWorkspace } from "../../app/context";
import type { Envelope, Receipt } from "../../shared/api/types";
import { usePageVisible } from "../../shared/query-visibility";
import {
  CopyExecutionInfo,
  requestExecutionInfo,
} from "../activities/execution-info";
import { SubmissionStatus } from "../activities/feedback/feedback";
import { DetectionID, type DetectionRecord } from "./detection-record";
import { Button } from "../../shared/ui/button";
import { environmentStage, type Environment } from "./use-readiness";

const priority = (stage: string) =>
  stage === "ready" ? 2 : stage === "verifying" ? 1 : 0;

export type DetectionSubmission = {
  sending: boolean;
  confirming: boolean;
  accepted: boolean;
  request?: Envelope;
  receipt?: Receipt;
};

type OverviewProps = {
  environments: Environment[];
  verified: boolean;
  defaultProfile: string;
  linkedEnvironment?: string;
  name: (environment: Environment) => string;
  preferred: (key: string) => string | undefined;
  onProfileChange: (key: string, profile: string) => void;
  startState: (environment: Environment) => DetectionSubmission;
  submitting: boolean;
  recordScope: string[];
  command: (
    action: "environment_start" | "environment_stop",
    target: string,
    version: number,
    payload: { [key: string]: unknown },
  ) => Promise<void>;
};

export function EnvironmentOverview({
  environments,
  linkedEnvironment,
  name,
  preferred,
  startState,
  recordScope,
  ...controls
}: OverviewProps) {
  const { verified } = controls;
  const { api, online } = useWorkspace();
  const visible = usePageVisible();
  const [order, setOrder] = useState<string[] | null>(null);
  useEffect(() => {
    if (order === null && verified && environments.length)
      setOrder(
        [...environments]
          .sort((a, b) => priority(a.stage) - priority(b.stage))
          .map((e) => e.environment_key),
      );
  }, [environments, verified, order]);
  // Freeze the first verified ordering. Polling must not move a focused row.
  const rows = [...environments].sort((a, b) => {
    if (!order) return verified ? priority(a.stage) - priority(b.stage) : 0;
    const index = (e: Environment) => {
      const found = order.indexOf(e.environment_key);
      return found < 0 ? order.length : found;
    };
    return index(a) - index(b);
  });
  const activeRecords = new Map(
    rows.flatMap((e) => (e.activity_id ? [[e.activity_id, e] as const] : [])),
  );
  const activityIDs = [...activeRecords.keys()];
  const activities = useQueries({
    queries: activityIDs.map((id) =>
      queryOptions({
        queryKey: [...recordScope, id],
        queryFn: ({ signal }) =>
          api.get<DetectionRecord>(
            "/environment/" + encodeURIComponent(id),
            signal,
          ),
        enabled: online,
        retry: false,
        refetchInterval: (query) =>
          visible &&
          online &&
          (!activeRecords.get(id)?.activity_settled ||
            query.state.data?.settled === false)
            ? 3000
            : false,
      }),
    ),
  });
  const byID = new Map(activityIDs.map((id, index) => [id, activities[index]]));
  const observedRows = rows.map((environment) => {
    const submission = startState(environment);
    const activity = byID.get(environment.activity_id ?? "");
    const stage = submission.confirming
      ? "verifying"
      : verified && !activity?.isError
        ? environment.stage
        : "unverified";
    return { environment, submission, activity, stage };
  });
  const ready = observedRows.filter(({ stage }) => stage === "ready").length;
  const running = observedRows.filter(
    ({ stage }) => stage === "verifying",
  ).length;
  const unverified = observedRows.filter(
    ({ stage }) => stage === "unverified",
  ).length;
  const attention = environments.length - ready - running - unverified;
  return (
    <section aria-label="全部运行环境" className="environment-overview">
      <div className="environment-totals" role="status">
        {verified ? (
          <>
            <span className="environment-total-ready">
              <strong>
                {ready}/{environments.length}
              </strong>{" "}
              已就绪
            </span>
            <span>检测中 {running}</span>
            <span>需要处理 {attention}</span>
            {unverified > 0 && <span>待核实 {unverified}</span>}
          </>
        ) : (
          <span>共 {environments.length} 个环境 · 当前状态待核实</span>
        )}
      </div>
      <table className="environment-table">
        <caption className="sr-only">所有环境的当前就绪情况</caption>
        <thead>
          <tr>
            <th scope="col">运行机 / 执行链路</th>
            <th scope="col">当前状态</th>
            <th scope="col">状态说明</th>
            <th scope="col">检测方案</th>
            <th scope="col">操作</th>
          </tr>
        </thead>
        <tbody>
          {observedRows.map(
            ({ environment: e, submission, activity, stage }) => (
              <EnvironmentRow
                {...controls}
                key={e.environment_key || e.profiles[0]?.id}
                environment={e}
                name={name(e)}
                linked={
                  !!linkedEnvironment && e.environment_key === linkedEnvironment
                }
                preferred={preferred(e.environment_key)}
                submission={submission}
                activity={activity}
                stage={stage}
              />
            ),
          )}
        </tbody>
      </table>
    </section>
  );
}

type RowProps = Pick<
  OverviewProps,
  "verified" | "defaultProfile" | "onProfileChange" | "submitting" | "command"
> & {
  environment: Environment;
  name: string;
  linked: boolean;
  preferred?: string;
  submission: DetectionSubmission;
  activity?: { data?: DetectionRecord; isError: boolean };
  stage: string;
};

function EnvironmentRow({
  environment: e,
  name,
  linked,
  preferred,
  submission,
  verified,
  defaultProfile,
  onProfileChange,
  submitting,
  activity,
  stage,
  command,
}: RowProps) {
  const { identity, online } = useWorkspace();
  const { activity_id: activityID, environment_key: key, profiles } = e;
  const record = activity?.data;
  const current = record?.id === activityID ? record : undefined;
  const unsettled =
    e.stage === "verifying" ||
    (!!activityID && (!e.activity_settled || current?.settled === false));
  const { confirming, sending, accepted, request, receipt } = submission;
  const label = confirming
    ? "正在确认启动"
    : environmentStage[stage] || environmentStage.unverified;
  const selected =
    profiles.length === 1
      ? profiles[0].id
      : preferred === "" || profiles.some((p) => p.id === preferred)
        ? preferred
        : (profiles.find((p) => p.id === defaultProfile)?.id ?? "");
  return (
    <tr
      id={`environment-${key || profiles[0]?.id}`}
      tabIndex={-1}
      className={linked ? "environment-row-selected" : undefined}
    >
      <th scope="row">{name}</th>
      <td>
        <span className="environment-status" data-stage={stage}>
          {label}
        </span>
        {!confirming && current?.created_at && (
          <p className="muted">
            最近检测：{new Date(current.created_at).toLocaleString("zh-CN")}
          </p>
        )}
        {!confirming && activityID && (
          <DetectionID id={activityID} execution={current?.execution_ref} />
        )}
      </td>
      <td>
        <p className="environment-overview-reason">
          {confirming
            ? "正在核实原请求，请勿重复提交。"
            : !verified
              ? `上次状态：${environmentStage[e.stage] || "待核实"}`
              : activity?.isError
                ? "检测状态查询失败，请刷新重试。"
                : e.stage === "ready"
                  ? "当前准备要求已通过"
                  : e.reason || "暂无状态说明"}
        </p>
        {!confirming && unsettled && <p className="muted">本次检测尚未结束</p>}
        {!confirming && stage === "ready" && (
          <p className="muted">重新检测会使当前就绪状态失效。</p>
        )}
        <SubmissionStatus
          label={
            sending
              ? "正在提交检测…"
              : confirming
                ? accepted
                  ? "已受理，正在读取工作信息"
                  : "提交结果待核实 · 请查看原请求"
                : activityID
                  ? "检测进度可在活动中心查看"
                  : ""
          }
          busy={sending}
          showActivity={!!request || !!activityID}
          announce={confirming}
        />
        {confirming && request && (
          <CopyExecutionInfo
            info={{
              ...requestExecutionInfo(request, receipt),
              project: identity.repository_identity,
            }}
          />
        )}
      </td>
      <td>
        {profiles.length > 1 ? (
          <select
            aria-label="检测所用方案"
            value={selected}
            disabled={confirming}
            onChange={(event) => onProfileChange(key, event.target.value)}
          >
            <option value="">选择本次检测方案</option>
            {profiles.map((p) => (
              <option key={p.id} value={p.id}>
                {[p.name, p.model, p.reasoning_effort]
                  .filter(Boolean)
                  .join(" · ")}
              </option>
            ))}
          </select>
        ) : (
          profiles[0]?.name || "无"
        )}
      </td>
      <td>
        {unsettled && !confirming ? (
          <Button
            size="sm"
            disabled={
              !online ||
              submitting ||
              activity?.isError ||
              !current ||
              current.settled
            }
            onClick={() =>
              current &&
              void command("environment_stop", current.id, current.revision, {})
            }
          >
            停止检测
          </Button>
        ) : (
          <Button
            size="sm"
            disabled={
              !verified ||
              stage === "unverified" ||
              submitting ||
              confirming ||
              !key ||
              !selected
            }
            onClick={() =>
              void command("environment_start", identity.project_id, 1, {
                environment_key: key,
                profile_id: selected,
                round_timeout_seconds: 3600,
                expected_current_revision: e.current_revision,
              })
            }
          >
            {sending
              ? "正在提交检测…"
              : e.stage === "ready" || activityID
                ? "重新检测"
                : "开始检测"}
          </Button>
        )}
      </td>
    </tr>
  );
}
