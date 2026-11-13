import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { useWorkspace } from "../../app/context";
import type { ProjectReadiness, Receipt } from "../../shared/api/types";
import { Button } from "../../shared/ui/button";
import { environmentLink } from "../environment-checks/use-readiness";

type Profile = { id: string; name: string; model?: string; version: string };
type Environment = {
  environment_key: string;
  name: string;
  agent?: string;
  target: { kind: string; environment?: string };
  status: "ready" | "not_ready" | "unverified" | "unsupported";
  reason?: string;
  current_revision: number;
  profiles: Profile[];
};
type Readiness = {
  branch: string;
  project: ProjectReadiness;
  environments: Environment[];
  default_profile: string;
  steer_agent?: { model: string; reasoning_effort: string };
  reason?: string;
};
type Catalogue = {
  directions: { id: string; name: string; prompt: string; revision: string }[];
  diagnostics?: string[];
};

const empty = {
  environment: "",
  profile: "",
  direction: "simplify",
  requirements: "",
  strategy: "cautious",
  dollars: "",
  minutes: "",
  rounds: "1",
};

function version(
  data?: Readiness,
  environment?: Environment,
  profile?: Profile,
) {
  return data && environment && profile
    ? JSON.stringify([
        environment.environment_key,
        environment.current_revision,
        profile.version,
        data.project.commit,
      ])
    : "";
}

export function NewMaintenance({
  open,
  close,
  full,
  receipt,
  onSubmit,
  onAccepted,
}: {
  open: boolean;
  close: () => void;
  full: boolean;
  receipt?: Receipt;
  onSubmit: (payload: Record<string, unknown>) => Promise<Receipt | undefined>;
  onAccepted: (id: string) => void;
}) {
  const { api, identity, online } = useWorkspace();
  const dialog = useRef<HTMLDialogElement>(null);
  const closeButton = useRef<HTMLButtonElement>(null);
  const seeded = useRef(false);
  const sent = useRef(false);
  const [draft, setDraft] = useState(empty);
  const [confirmed, setConfirmed] = useState("");
  const [pending, setPending] = useState(false);
  const [localReceipt, setLocalReceipt] = useState<Receipt>();
  const [error, setError] = useState("");
  const scope = [
    api.home,
    identity.project_id,
    identity.instance_id,
    "refactoring",
  ];
  const readiness = useQuery({
    queryKey: [...scope, "readiness"],
    queryFn: ({ signal }) =>
      api.get<Readiness>("/refactoring/readiness", signal),
    enabled: open && online,
    retry: false,
    refetchInterval: 10000,
  });
  const catalogue = useQuery({
    queryKey: [...scope, "directions"],
    queryFn: ({ signal }) =>
      api.get<Catalogue>("/refactoring/directions", signal),
    enabled: open && online,
  });
  const ready = readiness.data;
  const options =
    ready?.environments.filter((item) => item.status === "ready") ?? [];
  const environment = options.find(
    (item) => item.environment_key === draft.environment,
  );
  const profile = environment?.profiles.find(
    (item) => item.id === draft.profile,
  );
  const selectedVersion = version(ready, environment, profile);
  const direction = catalogue.data?.directions.find(
    (item) => item.id === draft.direction,
  );
  const available = online && readiness.isSuccess;

  useEffect(() => {
    const panel = dialog.current;
    if (!panel) return;
    if (open && !panel.open) {
      if (typeof panel.showModal === "function") panel.showModal();
      else panel.setAttribute("open", "");
      closeButton.current?.focus();
    } else if (!open && panel.open) {
      if (typeof panel.close === "function") panel.close();
      else panel.removeAttribute("open");
    }
  }, [open]);

  useEffect(() => {
    if (!open || !ready || !available || seeded.current) return;
    seeded.current = true;
    const candidate = ready.environments.find(
      (item) =>
        item.status === "ready" &&
        item.profiles.some((p) => p.id === ready.default_profile),
    );
    const selected = candidate?.profiles.find(
      (p) => p.id === ready.default_profile,
    );
    if (candidate && selected) {
      setDraft((old) => ({
        ...old,
        environment: candidate.environment_key,
        profile: selected.id,
      }));
      setConfirmed(version(ready, candidate, selected));
    }
  }, [open, ready, available]);

  const outcome =
    receipt?.status === "applied" || receipt?.status === "rejected"
      ? receipt
      : localReceipt;
  useEffect(() => {
    if (!pending || !outcome) return;
    const applied = outcome.refactoring as { id?: string } | undefined;
    if (outcome.status === "applied" && applied?.id) {
      sent.current = false;
      seeded.current = false;
      setPending(false);
      setLocalReceipt(undefined);
      setDraft(empty);
      setConfirmed("");
      setError("");
      onAccepted(applied.id);
    } else if (outcome.status === "rejected") {
      sent.current = false;
      setPending(false);
      setLocalReceipt(undefined);
      setError(outcome.diagnostic || "启动被拒绝");
      void readiness.refetch();
      void catalogue.refetch();
    }
  }, [pending, outcome, onAccepted, readiness, catalogue]);

  const chooseEnvironment = (key: string) => {
    const item = options.find((option) => option.environment_key === key);
    const selected =
      item?.profiles.length === 1
        ? item.profiles[0]
        : item?.profiles.find((p) => p.id === ready?.default_profile);
    setDraft((old) => ({
      ...old,
      environment: key,
      profile: selected?.id ?? "",
    }));
    setConfirmed(version(ready, item, selected));
    setError("");
  };
  const change = (field: keyof typeof empty, value: string) =>
    setDraft((old) => ({ ...old, [field]: value }));
  const needsConfirmation =
    selectedVersion !== "" && confirmed !== selectedVersion;
  const canStart =
    available &&
    ready?.project.status === "ready" &&
    !!profile &&
    !needsConfirmation &&
    !!direction &&
    catalogue.isSuccess &&
    !pending &&
    !!(draft.dollars || draft.minutes || draft.rounds);
  const other = ready?.environments.find(
    (item) => item.environment_key === draft.environment,
  );
  const groups = new Map<string, Environment[]>();
  for (const item of options) {
    const machine =
      item.target.kind === "local"
        ? "本机"
        : item.target.environment || item.target.kind;
    const members = groups.get(machine) ?? [];
    members.push(item);
    groups.set(machine, members);
  }

  return (
    <dialog
      ref={dialog}
      className="refactoring-sheet"
      aria-labelledby="new-maintenance-title"
      onCancel={(e) => {
        e.preventDefault();
        close();
      }}
    >
      <section className="refactoring-panel">
        <header className="refactoring-heading">
          <h2 id="new-maintenance-title">新建维护行动</h2>
          <Button
            ref={closeButton}
            variant="ghost"
            onClick={close}
            type="button"
          >
            关闭
          </Button>
        </header>
        <form
          onSubmit={async (e) => {
            e.preventDefault();
            if (
              !canStart ||
              !direction ||
              !profile ||
              !environment ||
              !ready ||
              sent.current
            )
              return;
            sent.current = true;
            setError("");
            setPending(true);
            try {
              setLocalReceipt(
                await onSubmit({
                  direction: direction.id,
                  direction_revision: direction.revision,
                  requirements: draft.requirements,
                  strategy: draft.strategy,
                  profile: profile.id,
                  profile_version: profile.version,
                  environment_key: environment.environment_key,
                  environment_revision: environment.current_revision,
                  base_commit: ready.project.commit,
                  budget: {
                    dollars: draft.dollars,
                    minutes: draft.minutes,
                    rounds: draft.rounds ? Number(draft.rounds) : 0,
                  },
                }),
              );
            } catch (err) {
              sent.current = false;
              setPending(false);
              setError(String(err));
            }
          }}
        >
          <fieldset disabled={pending}>
            {ready?.steer_agent?.model && (
              <p>
                SteerAgent 只读决策 {ready.steer_agent.model} /{" "}
                {ready.steer_agent.reasoning_effort}
              </p>
            )}
            <label>
              运行环境
              <select
                required
                value={draft.environment}
                onChange={(e) => chooseEnvironment(e.target.value)}
              >
                <option value="">选择已就绪的运行环境</option>
                {draft.environment && !environment && (
                  <option value={draft.environment} disabled>
                    {other?.name || draft.environment.slice(0, 12)}
                    （当前不可用）
                  </option>
                )}
                {[...groups].map(([machine, items]) => (
                  <optgroup label={machine} key={machine}>
                    {items.map((item) => (
                      <option
                        value={item.environment_key}
                        key={item.environment_key}
                      >
                        {item.name}
                        {item.agent ? " · " + item.agent : ""} ·{" "}
                        {item.environment_key.slice(0, 8)}
                      </option>
                    ))}
                  </optgroup>
                ))}
              </select>
            </label>
            {environment && (
              <label>
                执行方案
                <select
                  required
                  value={draft.profile}
                  onChange={(e) => {
                    change("profile", e.target.value);
                    setConfirmed(
                      version(
                        ready,
                        environment,
                        environment.profiles.find(
                          (p) => p.id === e.target.value,
                        ),
                      ),
                    );
                    setError("");
                  }}
                >
                  <option value="">选择执行方案</option>
                  {draft.profile && !profile && (
                    <option value={draft.profile} disabled>
                      {draft.profile}（当前不可用）
                    </option>
                  )}
                  {environment.profiles.map((p) => (
                    <option value={p.id} key={p.id}>
                      {p.name || p.id}
                      {p.model ? " · " + p.model : ""}
                    </option>
                  ))}
                </select>
              </label>
            )}
            {!available && (
              <p role="status" className="notice amber">
                {online && readiness.isPending
                  ? "正在查询运行环境"
                  : "运行环境状态查询失败或离线"}
                {ready && "，显示上次已知结果，当前待核实"}
              </p>
            )}
            {available &&
              (!options.length ||
                (draft.environment && !environment) ||
                (draft.profile && !profile)) && (
                <div className="notice amber">
                  <p>
                    {ready?.reason ||
                      (draft.environment
                        ? "所选运行环境或执行方案当前不可用"
                        : "没有可用的已就绪运行环境")}
                  </p>
                  {other?.reason && <p>{other.reason}</p>}
                  {!options.length &&
                    ready?.environments.map(
                      (item) =>
                        item.reason && (
                          <p key={item.environment_key}>
                            {item.name} · {item.reason}
                          </p>
                        ),
                    )}
                  <Link
                    to={environmentLink(
                      identity.project_id,
                      draft.profile || ready?.default_profile || "",
                    )}
                  >
                    打开项目运行环境
                  </Link>
                </div>
              )}
            <label>
              重构方向
              <select
                required
                value={draft.direction}
                onChange={(e) => change("direction", e.target.value)}
              >
                {catalogue.data?.directions.map((item) => (
                  <option value={item.id} key={item.id}>
                    {item.name}
                  </option>
                ))}
              </select>
            </label>
            {catalogue.isError && <p role="alert">维护方向查询失败</p>}
            {catalogue.data?.diagnostics?.map((message) => (
              <p className="notice amber" key={message}>
                {message}
              </p>
            ))}
            {direction && (
              <div className="refactoring-preview">
                <h3>方向提示词</h3>
                <pre aria-label="方向提示词" tabIndex={0}>
                  {direction.prompt}
                </pre>
              </div>
            )}
            <label>
              补充要求
              <textarea
                maxLength={32768}
                value={draft.requirements}
                onChange={(e) => change("requirements", e.target.value)}
              />
            </label>
            <label>
              决策策略
              <select
                value={draft.strategy}
                onChange={(e) => change("strategy", e.target.value)}
              >
                <option value="cautious">谨慎决策</option>
                <option value="autonomous">完全自主</option>
                <option value="aggressive">激进决策</option>
              </select>
            </label>
            <div className="refactoring-fields">
              <label>
                美元预算
                <input
                  type="number"
                  min="0.000001"
                  step="any"
                  value={draft.dollars}
                  onChange={(e) => change("dollars", e.target.value)}
                />
              </label>
              <label>
                时间预算（分钟）
                <input
                  type="number"
                  min="0.000001"
                  step="any"
                  value={draft.minutes}
                  onChange={(e) => change("minutes", e.target.value)}
                />
              </label>
              <label>
                重构轮次
                <input
                  type="number"
                  min="1"
                  max="10000"
                  step="1"
                  value={draft.rounds}
                  onChange={(e) => change("rounds", e.target.value)}
                />
              </label>
            </div>
            <p className="muted">
              至少设置一项预算。任一项耗尽后，行动进入收尾。
            </p>
            {ready && (
              <div className="refactoring-confirm">
                {profile && (
                  <p>
                    {environment?.name} · {profile.name || profile.id}
                  </p>
                )}
                <p>
                  项目基线
                  {ready.project.status === "ready"
                    ? "已就绪"
                    : "未就绪"} · {ready.branch}{" "}
                  {ready.project.commit && <code>{ready.project.commit}</code>}
                </p>
                {ready.project.blockers.map((blocker) => (
                  <p key={blocker.code}>{blocker.diagnostic}</p>
                ))}
                {ready.project.status !== "ready" && (
                  <Link
                    to={{
                      pathname: `/projects/${identity.project_id}/dag`,
                      search: "?maintenance=preparation",
                    }}
                  >
                    打开项目维护
                  </Link>
                )}
                {needsConfirmation && (
                  <div className="notice amber">
                    <p>所选方案、运行环境资格或项目基线已更新。</p>
                    <Button
                      type="button"
                      disabled={!available}
                      onClick={() => setConfirmed(selectedVersion)}
                    >
                      确认当前选择
                    </Button>
                  </div>
                )}
              </div>
            )}
            {full && <p role="status">推进名额已满，提交后进入队列。</p>}
            {error && (
              <p role="alert" className="notice amber">
                {error}
              </p>
            )}
            <Button disabled={!canStart} variant="default" type="submit">
              开始维护
            </Button>
          </fieldset>
          {pending && (
            <p role="status">{outcome?.diagnostic || "正在核实原启动请求"}</p>
          )}
        </form>
      </section>
    </dialog>
  );
}
