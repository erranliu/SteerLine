import { useEffect, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { useWorkspace } from "../../app/context";
import type { ProjectReadiness } from "../../shared/api/types";
import { Button } from "../../shared/ui/button";
import { environmentLink } from "../environment-checks/use-readiness";
import {
  defaultDirection,
  DirectionCatalogueStatus,
  useDirectionCatalogue,
  useDirectionChange,
  validDirectionPrompt,
} from "./directions";

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
const empty = {
  environment: "",
  profile: "",
  direction: defaultDirection,
  strategy: "cautious",
  dollars: "",
  minutes: "",
};

export type StartInput = {
  direction: string;
  direction_revision: string;
  direction_prompt: string;
  strategy: string;
  profile: string;
  profile_version: string;
  environment_key: string;
  environment_revision: number;
  base_commit: string;
  budget: { dollars?: string; minutes?: string };
};

function validBudget(value: string) {
  return (
    !value ||
    (value.length <= 32 &&
      /^(?:0|[1-9]\d*)(?:\.\d{1,18})?$/.test(value) &&
      Number(value) > 0)
  );
}

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
  initial,
  submissionPending,
  onSubmit,
}: {
  open: boolean;
  close: () => void;
  full: boolean;
  initial?: StartInput;
  submissionPending: boolean;
  onSubmit: (
    payload: StartInput,
    directionName: string,
    environmentName: string,
  ) => void;
}) {
  const { api, identity, online } = useWorkspace();
  const dialog = useRef<HTMLDialogElement>(null);
  const closeButton = useRef<HTMLButtonElement>(null);
  const seeded = useRef(!!initial);
  const [draft, setDraft] = useState(() =>
    initial
      ? {
          direction: initial.direction,
          strategy: initial.strategy,
          environment: initial.environment_key,
          profile: initial.profile,
          dollars: initial.budget.dollars ?? "",
          minutes: initial.budget.minutes ?? "",
        }
      : empty,
  );
  const [directionEdit, setDirectionEdit] = useState(() =>
    initial
      ? {
          revision: initial.direction_revision,
          prompt: initial.direction_prompt,
        }
      : undefined,
  );
  const [copyName, setCopyName] = useState<string>();
  const [directionSaved, setDirectionSaved] = useState(false);
  const [confirmed, setConfirmed] = useState(() =>
    initial
      ? JSON.stringify([
          initial.environment_key,
          initial.environment_revision,
          initial.profile_version,
          initial.base_commit,
        ])
      : "",
  );
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
  const catalogue = useDirectionCatalogue(open);
  const directionChange = useDirectionChange();
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
  const directionPrompt = directionEdit?.prompt ?? direction?.prompt ?? "";
  const directionStale =
    !!directionEdit && directionEdit.revision !== direction?.revision;
  const promptValid = validDirectionPrompt(directionPrompt);
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
  };
  const change = (field: keyof typeof empty, value: string) =>
    setDraft((old) => ({ ...old, [field]: value }));
  const needsConfirmation =
    selectedVersion !== "" && confirmed !== selectedVersion;
  const budgetValid = validBudget(draft.dollars) && validBudget(draft.minutes);
  const canStart =
    available &&
    ready?.project.status === "ready" &&
    !!profile &&
    !needsConfirmation &&
    !!direction &&
    catalogue.isSuccess &&
    !directionStale &&
    !directionChange.isPending &&
    copyName === undefined &&
    !submissionPending &&
    budgetValid &&
    promptValid;
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
        if (!directionChange.isPending) close();
      }}
    >
      <section className="refactoring-panel">
        <header className="refactoring-heading">
          <h2 id="new-maintenance-title">新建重构任务</h2>
          <Button
            ref={closeButton}
            variant="ghost"
            disabled={directionChange.isPending}
            onClick={close}
            type="button"
          >
            关闭
          </Button>
        </header>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (!canStart || !direction || !profile || !environment || !ready)
              return;
            onSubmit(
              {
                direction: direction.id,
                direction_revision: direction.revision,
                direction_prompt: directionPrompt,
                strategy: draft.strategy,
                profile: profile.id,
                profile_version: profile.version,
                environment_key: environment.environment_key,
                environment_revision: environment.current_revision,
                base_commit: ready.project.commit,
                budget: {
                  dollars: draft.dollars,
                  minutes: draft.minutes,
                },
              },
              direction.name,
              environment.name,
            );
          }}
        >
          <fieldset>
            <div className="refactoring-prompts">
              <section className="refactoring-preview">
                <h3>维护方向</h3>
                <label>
                  重构方向
                  <select
                    required
                    value={draft.direction}
                    disabled={directionChange.isPending}
                    onChange={(e) => {
                      change("direction", e.target.value);
                      setDirectionEdit(undefined);
                      setCopyName(undefined);
                      setDirectionSaved(false);
                      directionChange.reset();
                    }}
                  >
                    {catalogue.data && !direction && (
                      <option value={draft.direction} disabled>
                        原方向已不可用
                      </option>
                    )}
                    {catalogue.data?.directions.map((item) => (
                      <option value={item.id} key={item.id}>
                        {item.name}
                      </option>
                    ))}
                  </select>
                </label>
                {open && <DirectionCatalogueStatus catalogue={catalogue} />}
                {(direction || directionEdit) && (
                  <>
                    <label>
                      方向正文
                      <textarea
                        rows={8}
                        disabled={directionChange.isPending}
                        value={directionPrompt}
                        onChange={(e) => {
                          const revision =
                            directionEdit?.revision ?? direction?.revision;
                          if (revision)
                            setDirectionEdit({
                              revision,
                              prompt: e.target.value,
                            });
                          setDirectionSaved(false);
                        }}
                      />
                    </label>
                    {!promptValid && (
                      <p role="status">方向正文必填，最多 32 KiB。</p>
                    )}
                    {directionStale && (
                      <p role="status">
                        方向已更新或删除。可另存当前正文，或重新选择方向。
                      </p>
                    )}
                    {direction && directionStale && (
                      <Button
                        type="button"
                        variant="ghost"
                        onClick={() => setDirectionEdit(undefined)}
                      >
                        加载最新正文
                      </Button>
                    )}
                    <div className="refactoring-direction-actions">
                      {direction && direction.id !== defaultDirection && (
                        <Button
                          type="button"
                          disabled={
                            !promptValid ||
                            directionStale ||
                            directionPrompt === direction.prompt ||
                            directionChange.isPending
                          }
                          onClick={() =>
                            directionChange.mutate(
                              {
                                action: "update",
                                id: direction.id,
                                revision: direction.revision,
                                name: direction.name,
                                prompt: directionPrompt,
                              },
                              {
                                onSuccess: (row) => {
                                  setDirectionEdit({
                                    revision: row.revision,
                                    prompt: row.prompt,
                                  });
                                  setDirectionSaved(true);
                                },
                              },
                            )
                          }
                        >
                          保存到方向
                        </Button>
                      )}
                      <Button
                        type="button"
                        variant="ghost"
                        disabled={!promptValid || directionChange.isPending}
                        onClick={() => setCopyName("")}
                      >
                        另存为新方向
                      </Button>
                    </div>
                    {copyName !== undefined && (
                      <div className="refactoring-direction-actions">
                        <label>
                          新方向名称
                          <input
                            disabled={directionChange.isPending}
                            value={copyName}
                            onChange={(e) => setCopyName(e.target.value)}
                          />
                        </label>
                        <Button
                          type="button"
                          disabled={
                            !copyName.trim() ||
                            !promptValid ||
                            directionChange.isPending
                          }
                          onClick={() =>
                            directionChange.mutate(
                              {
                                action: "create",
                                name: copyName,
                                prompt: directionPrompt,
                              },
                              {
                                onSuccess: (row) => {
                                  change("direction", row.id);
                                  setDirectionEdit({
                                    revision: row.revision,
                                    prompt: row.prompt,
                                  });
                                  setCopyName(undefined);
                                  setDirectionSaved(true);
                                },
                              },
                            )
                          }
                        >
                          保存新方向
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          disabled={directionChange.isPending}
                          onClick={() => {
                            setCopyName(undefined);
                            directionChange.reset();
                          }}
                        >
                          取消
                        </Button>
                      </div>
                    )}
                    {directionChange.error && (
                      <p role="alert">{directionChange.error.message}</p>
                    )}
                    {directionSaved && !directionChange.error && (
                      <p role="status">已保存方向</p>
                    )}
                    <p className="muted">
                      直接启动时，当前正文仅用于本次重构任务。
                    </p>
                  </>
                )}
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
              </section>
              <section className="refactoring-preview">
                <h3>执行指令</h3>
                <p className="muted">
                  启动后自动实施和验证，完成后创建 PR
                  并评审。没有有效改动时保存调查结论。
                </p>
              </section>
            </div>
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
            </div>
            <p className="muted">
              预算可选。不填写时完成一项改进后交付。任一预算耗尽后，完成必要收尾并推进 PR。
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
            {!budgetValid && (
              <p role="status">
                请填写至少一项有效的正数预算，轮次最多 10000。
              </p>
            )}
            {submissionPending && (
              <p role="status">上一条提交待确认，确认后可以继续提交。</p>
            )}
            <Button disabled={!canStart} variant="default" type="submit">
              启动重构任务
            </Button>
          </fieldset>
        </form>
      </section>
    </dialog>
  );
}
