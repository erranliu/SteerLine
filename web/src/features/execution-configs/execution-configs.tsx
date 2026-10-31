import { useEffect, useMemo, useRef, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Plus,
  Copy,
  Trash2,
  Monitor,
  Server,
  RefreshCw,
  ChevronRight,
  Code,
  Clock3,
  Globe,
} from "lucide-react";
import { ApiError } from "../../shared/api/client";
import {
  executionConfigAPI,
  saveExecutionConfig,
  stable,
  type Profile,
  type Settings,
  type ConfigSnapshot,
  type ExecutionConfig,
  type Catalog,
} from "../../shared/api/execution-configs";
import { NavigationGuard } from "../../shared/ui/navigation-guard";
import "./execution-configs.css";

const clone = <T,>(value: T): T => structuredClone(value);
const values = (p: Profile) =>
  p.execution.adapter === "orca-terminal"
    ? (p.execution.settings.agent_launch ?? {})
    : p.execution.settings;
const remote = (p: Profile) => p.execution.target.kind === "orca_environment";
const agentOf = (p: Profile) => p.execution.settings.agent || "codex";
const editable = (p: Profile) =>
  (p.execution.adapter === "codex-app-server" &&
    p.execution.target.kind === "local") ||
  (p.execution.adapter === "orca-terminal" &&
    (remote(p)
      ? agentOf(p) === "codex"
      : p.execution.target.kind === "local" &&
        ["codex", "pi", "dsh"].includes(agentOf(p))));
const effortNames: Record<string, string> = {
  none: "None · 无",
  minimal: "Minimal · 最小",
  low: "Low · 低",
  medium: "Medium · 中",
  high: "High · 高",
  xhigh: "Extra high · 更高",
  max: "Max · 最大",
  ultra: "Ultra · 极高",
};

export function ExecutionConfigs({
  home,
  csrf,
  loadedDigest,
  applicationConnected,
}: {
  home: string;
  csrf: string;
  loadedDigest?: string;
  applicationConnected: boolean;
}) {
  const api = useMemo(() => executionConfigAPI(csrf), [csrf]);
  const client = useQueryClient();
  const query = useQuery({
    queryKey: [home, "execution-config"],
    queryFn: ({ signal }) => api.read(signal),
    retry: false,
  });
  const [base, setBase] = useState<ConfigSnapshot>();
  const [draft, setDraft] = useState<Profile>();
  const [defaultChoice, setDefaultChoice] = useState(false);
  const [manual, setManual] = useState(false);
  const [providerChoice, setProviderChoice] = useState("");
  const [catalog, setCatalog] = useState<Catalog>();
  const [loading, setLoading] = useState(false);
  const [refresh, setRefresh] = useState(0);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [pending, setPending] = useState<(() => void) | null>(null);
  const [deleting, setDeleting] = useState(false);
  const latest = useRef(0);
  const environments = useQuery({
    queryKey: [home, "execution-environments"],
    queryFn: ({ signal }) =>
      api.discover({ kind: "environments", host: "orca_environment" }, signal),
    retry: false,
  });
  useEffect(() => {
    if (!base && query.data) {
      setBase(query.data);
      const first = query.data.config.execution_profiles[0];
      setDraft(clone(first));
      setDefaultChoice(
        first.id === query.data.config.default_execution_profile,
      );
    }
  }, [query.data, base]);
  const target = draft?.execution.target;
  const draftID = draft?.id;
  const adapter = draft?.execution.adapter;
  const environment = target?.environment;
  const host = target?.kind;
  const agent = draft ? agentOf(draft) : "codex";
  useEffect(() => {
    if (!draft || !editable(draft)) return;
    const controller = new AbortController();
    const generation = ++latest.current;
    setLoading(true);
    setCatalog(undefined);
    setProviderChoice("");
    const probeProfile = base?.config.execution_profiles.find(
      (p) =>
        p.execution.adapter === draft.execution.adapter &&
        p.execution.target.kind === draft.execution.target.kind &&
        agentOf(p) === agentOf(draft),
    );
    let attempts = 0;
    const run = async () => {
      let result: Catalog;
      do {
        result = await api.discover(
          {
            kind: "models",
            host: draft.execution.target.kind,
            environment: draft.execution.target.environment,
            agent: agentOf(draft),
            adapter: draft.execution.adapter,
            profile_id: probeProfile?.id,
            refresh: refresh > 0,
          },
          controller.signal,
        );
        if (
          result.state !== "in_progress" ||
          ++attempts >= 3 ||
          controller.signal.aborted ||
          document.visibilityState === "hidden"
        )
          break;
      } while (true);
      if (generation !== latest.current || controller.signal.aborted) return;
      setCatalog(result);
      if (result.state === "ready" && result.models?.length) {
        setDraft((current) => {
          if (!current || current.id !== draft.id || values(current).model)
            return current;
          if (
            base?.config.execution_profiles.some(
              (p) => p.id === current.id && stable(p) === stable(current),
            )
          )
            return current;
          const m =
            result.models!.find((x) => x.is_default) ?? result.models![0];
          const choice = base?.combinations.find(
            (c) =>
              c.host === current.execution.target.kind &&
              c.adapter === current.execution.adapter &&
              c.agent === agentOf(current),
          );
          return changeModel(
            current,
            m.id,
            choice?.efforts.includes(m.default_effort ?? "")
              ? m.default_effort
              : undefined,
          );
        });
      }
    };
    void run()
      .catch((e: unknown) => {
        if (!controller.signal.aborted && generation === latest.current)
          setCatalog({
            state: "unavailable",
            diagnostic: String(e),
            origin: "unknown",
            freshness: "fresh",
            context_key: "",
            context_unverified: true,
          });
      })
      .finally(() => {
        if (!controller.signal.aborted && generation === latest.current)
          setLoading(false);
      });
    return () => {
      controller.abort();
      latest.current++;
    };
    // Model/effort edits do not launch a new native query.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [api, draftID, host, adapter, agent, environment, refresh]);
  const saved = base?.config.execution_profiles.find((p) => p.id === draft?.id);
  const dirty =
    !!draft &&
    (!saved ||
      stable(saved) !== stable(draft) ||
      defaultChoice !== (base?.config.default_execution_profile === draft.id));
  const models = catalog?.models ?? [];
  const model = draft ? String(values(draft).model ?? "") : "";
  const providerEnabled = adapter === "orca-terminal" && agent === "pi";
  const providers = [
    ...new Set(models.flatMap((m) => (m.provider ? [m.provider] : []))),
  ];
  const selectedProvider = providerEnabled
    ? (models.find((m) => m.id === model)?.provider ?? providerChoice)
    : "";
  const visibleModels =
    providerEnabled && selectedProvider
      ? models.filter((m) => m.provider === selectedProvider)
      : models;
  const mappedAgents =
    base?.combinations
      .filter((c) => c.host === host && c.adapter === adapter && c.supported)
      .map((c) => c.agent) ?? [];
  const agentChoices = [
    ...new Set([
      ...mappedAgents,
      ...(catalog?.agents?.map((a) => a.id) ?? []),
      agent,
    ]),
  ];
  const effort = draft ? String(values(draft).reasoning_effort ?? "") : "";
  const combination = base?.combinations.find(
    (c) => c.host === host && c.adapter === adapter && c.agent === agent,
  );
  const schemaEfforts = combination?.efforts ?? [];
  const workflowChoices = combination?.workflows ?? [];
  const localOrca = host === "local" && adapter === "orca-terminal";
  const executionUnchanged =
    !!saved &&
    !!draft &&
    stable(saved.execution) === stable(draft.execution) &&
    stable(saved.workflow) === stable(draft.workflow);
  const materialReady =
    !localOrca || !!base?.local_helper.available || executionUnchanged;
  const allowed =
    models
      .find((m) => m.id === model)
      ?.efforts.filter((e) => schemaEfforts.includes(e.value)) ??
    schemaEfforts.map((value) => ({
      value,
      label: effortNames[value] ?? value,
    }));
  const modelHasEffort =
    !models.some((m) => m.id === model) ||
    (models.find((m) => m.id === model)?.efforts.length ?? 0) > 0;
  const preservedModelChoice =
    !!saved &&
    model === String(values(saved).model ?? "") &&
    effort === String(values(saved).reasoning_effort ?? "");
  const valid =
    !!draft?.name.trim() &&
    materialReady &&
    !!combination?.supported &&
    (preservedModelChoice ||
      (!!model &&
        (!modelHasEffort || !!effort || manual) &&
        (!effort || schemaEfforts.includes(effort))));
  const applied = applicationConnected && !!loadedDigest && !!base;
  const pendingRestart = applied && loadedDigest !== base.saved_digest;
  const go = (action: () => void) => {
    if (dirty) setPending(() => action);
    else action();
  };
  function select(p: Profile) {
    setDraft(clone(p));
    setDefaultChoice(p.id === base?.config.default_execution_profile);
    setManual(false);
    setError("");
    setNotice("");
    setPending(null);
    setDeleting(false);
  }
  function create(copy = false) {
    if (!base) return;
    const id = "profile-" + crypto.randomUUID().slice(0, 8);
    const localExecutable =
      base.config.execution_profiles.find(
        (p) => p.execution.adapter === "codex-app-server",
      )?.execution.settings.executable ?? "codex.exe";
    const p: Profile =
      copy && draft
        ? { ...clone(draft), id, name: draft.name + " 副本" }
        : {
            id,
            name: "新的执行方案",
            execution: {
              adapter: "codex-app-server",
              target: { kind: "local" },
              settings: { executable: localExecutable },
            },
            workflow: {
              id: "development",
              version:
                base.workflows.find((w) => w.id === "development")?.version ??
                2,
            },
          };
    select(p);
    setDefaultChoice(false);
  }
  function patchSettings(patch: Partial<Settings>) {
    if (draft)
      setDraft({
        ...draft,
        execution: {
          ...draft.execution,
          settings: { ...draft.execution.settings, ...patch },
        },
      });
  }
  function setMain(modelValue: string, effortValue?: string) {
    if (draft) setDraft(changeModel(draft, modelValue, effortValue));
  }
  function changeRoute(
    kind: string,
    chain: string,
    selectedAgent = "codex",
    env?: string,
  ) {
    if (!draft || !base) return;
    const templates = base.config.execution_profiles;
    const template = templates.find(
      (p) =>
        p.execution.adapter === chain &&
        p.execution.target.kind === kind &&
        agentOf(p) === selectedAgent,
    );
    let settings: Settings;
    if (chain === "codex-app-server") {
      settings = template
        ? clone(template.execution.settings)
        : { executable: "codex.exe" };
      delete settings.model;
      delete settings.reasoning_effort;
    } else {
      const localTemplate = templates.find(
        (p) =>
          p.execution.adapter === chain && p.execution.target.kind === "local",
      );
      const routeTemplate =
        template ??
        templates.find(
          (p) =>
            p.execution.adapter === chain && p.execution.target.kind === kind,
        );
      settings = template
        ? clone(template.execution.settings)
        : {
            transport: clone(
              routeTemplate?.execution.settings.transport ?? {
                executable: "orca.exe",
              },
            ),
            agent_launch: {
              executable: { codex: "codex", pi: "pi.cmd", dsh: "dsh.cmd" }[
                selectedAgent
              ],
            },
          };
      settings.agent = selectedAgent;
      settings.agent_launch ??= {};
      delete settings.agent_launch.model;
      delete settings.agent_launch.reasoning_effort;
      delete settings.agent_launch.runtime_executable;
      delete settings.agent_launch.runtime_sha256;
      if (kind === "local" && localTemplate?.execution.settings.agent_launch) {
        const source = localTemplate.execution.settings.agent_launch;
        if (source.runtime_executable)
          settings.agent_launch.runtime_executable = source.runtime_executable;
        if (source.runtime_sha256)
          settings.agent_launch.runtime_sha256 = source.runtime_sha256;
      }
    }
    const choice = base.combinations.find(
      (c) =>
        c.host === kind && c.adapter === chain && c.agent === selectedAgent,
    );
    const workflow =
      choice?.workflows.find(
        (w) =>
          w.id === draft.workflow.id && w.version === draft.workflow.version,
      ) ?? choice?.workflows.find((w) => w.id === "development");
    const nextWorkflow: Profile["workflow"] = {
      ...clone(draft.workflow),
      id: workflow?.id ?? "development",
      version: workflow?.version ?? 2,
    };
    if (!workflow?.audit_model_required) delete nextWorkflow.audit_model;
    if (
      workflow?.prompt_steps &&
      nextWorkflow.prompt_overrides &&
      typeof nextWorkflow.prompt_overrides === "object"
    ) {
      nextWorkflow.prompt_overrides = Object.fromEntries(
        Object.entries(nextWorkflow.prompt_overrides).filter(([key]) =>
          workflow.prompt_steps!.includes(key),
        ),
      );
    }
    setDraft({
      ...draft,
      execution: {
        adapter: chain,
        target: kind === "local" ? { kind } : { kind, environment: env },
        settings,
      },
      workflow: nextWorkflow,
    });
    setManual(false);
    setError("");
  }
  function changeHost(value: string) {
    if (value === "local") changeRoute("local", adapter ?? "codex-app-server");
    else changeRoute("orca_environment", "orca-terminal", "codex", value);
  }
  async function saveConfig(config: ExecutionConfig, selectedID: string) {
    if (!base) return;
    setSaving(true);
    setError("");
    setNotice("");
    try {
      const result = await saveExecutionConfig(api, config, base.saved_digest);
      setBase(result);
      client.setQueryData([home, "execution-config"], result);
      const p =
        result.config.execution_profiles.find((v) => v.id === selectedID) ??
        result.config.execution_profiles[0];
      setDraft(clone(p));
      setDefaultChoice(p.id === result.config.default_execution_profile);
      setDeleting(false);
      setPending(null);
      setNotice("配置已保存。重启 SteerLine 后生效。");
      void client.invalidateQueries({ queryKey: [home, "projects"] });
      void client.invalidateQueries({
        queryKey: [home, "execution-config-status"],
      });
    } catch (e) {
      setError(
        e instanceof ApiError && e.status === 412
          ? "配置已被其他页面修改。当前草稿已保留，请重新读取后合并。"
          : String(e),
      );
    } finally {
      setSaving(false);
    }
  }
  function save() {
    if (!base || !draft || !valid) return;
    const config = clone(base.config);
    const i = config.execution_profiles.findIndex((p) => p.id === draft.id);
    if (i < 0) config.execution_profiles.push(draft);
    else config.execution_profiles[i] = draft;
    if (defaultChoice) config.default_execution_profile = draft.id;
    void saveConfig(config, draft.id);
  }
  function remove() {
    if (!base || !draft || draft.id === base.config.default_execution_profile)
      return;
    const config = clone(base.config);
    config.execution_profiles = config.execution_profiles.filter(
      (p) => p.id !== draft.id,
    );
    void saveConfig(config, config.default_execution_profile);
  }
  function reload() {
    void api
      .read()
      .then((result) => {
        setBase(result);
        select(result.config.execution_profiles[0]);
        client.setQueryData([home, "execution-config"], result);
      })
      .catch((e) => setError(String(e)));
  }
  const hostName = (p: Profile) =>
    !remote(p)
      ? "本机"
      : (environments.data?.environments?.find(
          (e) =>
            e.id === p.execution.target.environment ||
            e.name === p.execution.target.environment,
        )?.name ??
        p.execution.target.environment ??
        "远端机器");
  const queryTitle = loading
    ? "正在获取能力目录…"
    : manual
      ? "手动填写 · 目录未确认"
      : catalog?.diagnostic
        ? catalog.freshness === "expired"
          ? "使用上次目录 · 本次查询失败"
          : "能力目录当前不可用"
        : catalog?.state === "ready"
          ? "目录已获取"
          : catalog?.state === "in_progress"
            ? "目录仍在获取中"
            : "目录尚未确认";
  const currentValue = (value: string, options: string[]) =>
    value && !options.includes(value) ? (
      <option value={value}>{value} · 当前目录未确认</option>
    ) : null;
  const effortOptions = (current: string, entries = allowed) => (
    <>
      <option value="">
        {manual ? "原生默认（不指定）" : "请选择思考强度"}
      </option>
      {currentValue(
        current,
        entries.map((e) => e.value),
      )}
      {entries.map((e) => (
        <option key={e.value} value={e.value}>
          {effortNames[e.value] ?? e.value}
        </option>
      ))}
    </>
  );
  const modelOptions = (value: string) => (
    <>
      {!value && <option value="">请选择模型</option>}
      {currentValue(
        value,
        visibleModels.map((m) => m.id),
      )}
      {visibleModels.map((m) => (
        <option key={m.id} value={m.id}>
          {m.label || m.id}
        </option>
      ))}
    </>
  );
  return (
    <div className="execution-config-app">
      <NavigationGuard dirty={dirty || saving} />
      <div className="sl-page-head">
        <div>
          <div className="sl-eyebrow">EXECUTION PROFILES</div>
          <h1>执行方案配置</h1>
          <p className="sl-caption">根据运行环境选择能力，保存为可复用配置。</p>
        </div>
        <button
          type="button"
          className="sl-button primary"
          disabled={!base || saving || !base.writable}
          onClick={() => go(() => create())}
        >
          <Plus />
          新建方案
        </button>
      </div>
      {pendingRestart && (
        <div className="sl-alert" role="status">
          <Clock3 />
          <div>
            <strong>配置已保存，待重启生效</strong>
            <p>
              请通过现有入口重启
              SteerLine。已有执行保留原参数，刷新页面不会应用新配置。
            </p>
          </div>
        </div>
      )}
      {base && !base.writable && (
        <div className="sl-alert" role="status">
          {base.reason || "当前配置只读"}
        </div>
      )}
      {query.isPending && <p role="status">正在读取执行方案…</p>}
      {query.error && (
        <div className="sl-alert" role="alert">
          {String(query.error)}
          <button
            type="button"
            className="sl-button"
            onClick={() => void query.refetch()}
          >
            重新读取
          </button>
        </div>
      )}
      {base && draft && (
        <div className="sl-workspace">
          <aside className="sl-profiles" aria-label="已保存执行方案">
            <div className="sl-list-head">
              <span>已保存方案</span>
              <span>{base.config.execution_profiles.length}</span>
            </div>
            {[
              ...new Set(
                base.config.execution_profiles.map((p) =>
                  JSON.stringify([
                    p.execution.target.kind,
                    p.execution.target.environment || "",
                  ]),
                ),
              ),
            ].map((target) => {
              const profiles = base.config.execution_profiles.filter(
                (p) =>
                  JSON.stringify([
                    p.execution.target.kind,
                    p.execution.target.environment || "",
                  ]) === target,
              );
              return (
                <section key={target} aria-label={hostName(profiles[0])}>
                  <h3 className="sl-host-heading">{hostName(profiles[0])}</h3>
                  {profiles.map((p) => (
                    <button
                      type="button"
                      key={p.id}
                      className={
                        "sl-profile " + (draft.id === p.id ? "selected" : "")
                      }
                      aria-pressed={draft.id === p.id}
                      disabled={saving}
                      onClick={() => go(() => select(p))}
                    >
                      <span className="sl-profile-name">
                        {p.name}
                        {p.id === base.config.default_execution_profile && (
                          <span className="sl-tag">默认</span>
                        )}
                      </span>
                      <span className="sl-profile-meta">
                        {remote(p) ? <Server /> : <Monitor />}
                        {hostName(p)} ·{" "}
                        {p.execution.adapter === "orca-terminal"
                          ? "Orca / " + agentOf(p)
                          : "Codex"}
                      </span>
                      <span className="sl-profile-meta">
                        {String(values(p).model ?? "原生默认")} /{" "}
                        {String(values(p).reasoning_effort ?? "默认")}
                      </span>
                      {p.workflow.id === "development-audited" && (
                        <span className="sl-profile-meta">审计开发流程</span>
                      )}
                    </button>
                  ))}
                </section>
              );
            })}
          </aside>
          <section className="sl-editor" aria-label="执行方案编辑器">
            <div className="sl-editor-head">
              <div className="sl-inline">
                <h2>{saved ? "编辑执行方案" : "新建执行方案"}</h2>
                {dirty && <span className="sl-tag">未保存</span>}
              </div>
              <div className="sl-inline">
                <button
                  type="button"
                  className="sl-button quiet"
                  disabled={saving || !editable(draft)}
                  onClick={() => go(() => create(true))}
                >
                  <Copy />
                  复制
                </button>
                <button
                  type="button"
                  className="sl-button quiet danger"
                  disabled={
                    saving ||
                    !saved ||
                    base.config.execution_profiles.length < 2 ||
                    draft.id === base.config.default_execution_profile ||
                    !base.writable
                  }
                  onClick={() => setDeleting(true)}
                >
                  <Trash2 />
                  删除
                </button>
              </div>
            </div>
            <div className="sl-form">
              {pending && (
                <div className="sl-alert">
                  <div>
                    <strong>当前修改尚未保存</strong>
                    <p>切换前可以先保存，或放弃当前修改。</p>
                    <div className="sl-inline">
                      <button
                        type="button"
                        className="sl-button"
                        onClick={() => {
                          const action = pending;
                          setPending(null);
                          action();
                        }}
                      >
                        放弃并继续
                      </button>
                      <button
                        type="button"
                        className="sl-button quiet"
                        onClick={() => setPending(null)}
                      >
                        继续编辑
                      </button>
                    </div>
                  </div>
                </div>
              )}
              {error && (
                <div className="sl-alert" role="alert">
                  <div>
                    {error}
                    <p>重新读取将放弃当前草稿。</p>
                    <button
                      type="button"
                      className="sl-button"
                      onClick={reload}
                    >
                      重新读取配置
                    </button>
                  </div>
                </div>
              )}
              {deleting && (
                <div className="sl-alert">
                  <div>
                    <strong>删除“{draft.name}”？</strong>
                    <p>确认后保存配置目录，历史执行保留原参数。</p>
                    <button
                      type="button"
                      className="sl-button"
                      disabled={saving}
                      onClick={remove}
                    >
                      确认删除
                    </button>
                    <button
                      type="button"
                      className="sl-button quiet"
                      onClick={() => setDeleting(false)}
                    >
                      取消
                    </button>
                  </div>
                </div>
              )}
              {!editable(draft) && (
                <div className="sl-alert">
                  当前 Web
                  版本不提供此链路的参数编辑；原配置完整保留，可以删除。
                </div>
              )}
              <fieldset disabled={saving || !editable(draft)}>
                <div className="sl-field">
                  <label htmlFor="execution-name">配置名称</label>
                  <input
                    id="execution-name"
                    value={draft.name}
                    maxLength={64}
                    onChange={(e) =>
                      setDraft({ ...draft, name: e.target.value })
                    }
                  />
                </div>
                <div className="sl-row">
                  <div className="sl-field">
                    <label htmlFor="execution-host">运行机</label>
                    <select
                      id="execution-host"
                      value={remote(draft) ? environment : "local"}
                      onChange={(e) => changeHost(e.target.value)}
                    >
                      <option value="local">本机</option>
                      {remote(draft) &&
                        !environments.data?.environments?.some(
                          (e) => e.id === environment,
                        ) && (
                          <option value={environment}>
                            {hostName(draft)} · 已保存
                          </option>
                        )}
                      {environments.data?.environments?.map((e) => (
                        <option key={e.id} value={e.id}>
                          {e.name}
                        </option>
                      ))}
                    </select>
                  </div>
                  <div className="sl-field">
                    <label htmlFor="execution-chain">执行链路</label>
                    <select
                      id="execution-chain"
                      value={adapter}
                      onChange={(e) => changeRoute("local", e.target.value)}
                      disabled={remote(draft)}
                    >
                      {!remote(draft) && (
                        <option value="codex-app-server">Codex 直连</option>
                      )}
                      <option value="orca-terminal">Orca + Agent</option>
                    </select>
                  </div>
                </div>
                {environments.error && (
                  <p className="sl-help">
                    远端机器目录暂不可用：{String(environments.error)}
                  </p>
                )}
                {adapter === "orca-terminal" && (
                  <div className="sl-field">
                    <label htmlFor="execution-agent">Agent</label>
                    <select
                      id="execution-agent"
                      value={agent}
                      onChange={(e) =>
                        changeRoute(
                          host!,
                          "orca-terminal",
                          e.target.value,
                          environment,
                        )
                      }
                    >
                      {agentChoices.map((a) => (
                        <option
                          key={a}
                          value={a}
                          disabled={!mappedAgents.includes(a)}
                        >
                          {(
                            {
                              codex: "Codex",
                              pi: "Pi",
                              dsh: "DSH",
                            } as Record<string, string>
                          )[a] ?? a}
                          {!mappedAgents.includes(a)
                            ? " · 目录已发现，当前执行链路未支持"
                            : catalog?.agents &&
                                !catalog.agents.some((entry) => entry.id === a)
                              ? " · 未发现目录，可手动填写"
                              : ""}
                        </option>
                      ))}
                    </select>
                  </div>
                )}
                {adapter === "orca-terminal" && (
                  <p className="sl-help">
                    Provider 与 API key 请在目标 Orca / Agent 中配置；SteerLine
                    仅查询和选择已有目录，不保存凭据。
                  </p>
                )}
                {providerEnabled && (
                  <div className="sl-field">
                    <label htmlFor="execution-provider">Provider</label>
                    <select
                      id="execution-provider"
                      value={selectedProvider}
                      disabled={manual || loading}
                      onChange={(e) => {
                        setProviderChoice(e.target.value);
                        setMain("", undefined);
                      }}
                    >
                      <option value="">全部已发现 Provider</option>
                      {providers.map((p) => (
                        <option key={p} value={p}>
                          {p}
                        </option>
                      ))}
                    </select>
                    <p className="sl-help">
                      模型保留完整 provider/model 标识；目录可见不代表 API key
                      或实际调用已验证。
                    </p>
                  </div>
                )}
                {localOrca && (
                  <p className="sl-help">
                    {base.local_helper.available
                      ? "本机 helper 部署材料已核对；路径与摘要由部署配置提供。"
                      : base.local_helper.reason}
                  </p>
                )}
                <div className="sl-query" aria-live="polite">
                  <div>
                    <strong>
                      <span
                        className={
                          "sl-dot " +
                          (catalog?.state !== "ready" || manual ? "warn" : "")
                        }
                      />
                      {queryTitle}
                    </strong>
                    <small>
                      {catalog?.diagnostic ||
                        [
                          catalog?.freshness === "cached"
                            ? "缓存"
                            : catalog?.freshness === "expired"
                              ? "已过期"
                              : catalog?.origin === "probe"
                                ? "原生探测目录"
                                : catalog?.origin || "",
                          catalog?.fetched_at
                            ? new Date(catalog.fetched_at).toLocaleTimeString()
                            : "",
                        ]
                          .filter(Boolean)
                          .join(" · ")}
                    </small>
                  </div>
                  <button
                    type="button"
                    className="sl-button quiet"
                    disabled={loading}
                    onClick={() => setRefresh((r) => r + 1)}
                  >
                    <RefreshCw />
                    刷新能力目录
                  </button>
                </div>
                {catalog?.context_unverified && (
                  <p className="sl-help sl-scope">
                    {catalog.scope ||
                      "目录范围未核实，不代表实际执行环境已验证。"}
                  </p>
                )}
                <div className="sl-row">
                  <div className="sl-field">
                    <label htmlFor="execution-model">
                      {manual ? "模型标识" : "模型"}
                    </label>
                    {manual ? (
                      <input
                        id="execution-model"
                        value={model}
                        spellCheck={false}
                        onChange={(e) => setMain(e.target.value, effort)}
                      />
                    ) : (
                      <select
                        id="execution-model"
                        value={model}
                        onChange={(e) => {
                          const m = models.find((v) => v.id === e.target.value);
                          const keep = m?.efforts.some(
                            (v) => v.value === effort,
                          );
                          setMain(
                            e.target.value,
                            keep ? effort : m?.efforts.length ? "" : undefined,
                          );
                        }}
                      >
                        {modelOptions(model)}
                      </select>
                    )}
                  </div>
                  <div className="sl-field">
                    <label htmlFor="execution-effort">Thinking effort</label>
                    {!modelHasEffort && !manual ? (
                      <div className="sl-static">不适用</div>
                    ) : (
                      <select
                        id="execution-effort"
                        value={effort}
                        onChange={(e) => setMain(model, e.target.value)}
                      >
                        {effortOptions(effort)}
                      </select>
                    )}
                  </div>
                </div>
                <div className="sl-model-note">
                  <p className="sl-help">
                    {manual
                      ? "原始值将原样保存，当前目录未确认。"
                      : "仅显示当前模型支持的思考强度。"}
                  </p>
                  <button
                    type="button"
                    className="sl-button quiet"
                    onClick={() => setManual(!manual)}
                  >
                    {manual ? "返回目录选择" : "手动填写"}
                  </button>
                </div>
                <label className="sl-check">
                  <input
                    type="checkbox"
                    checked={defaultChoice}
                    disabled={
                      base.config.default_execution_profile === draft.id
                    }
                    onChange={(e) => setDefaultChoice(e.target.checked)}
                  />
                  用作新任务的默认配置
                </label>
                <details className="sl-advanced">
                  <summary>
                    <ChevronRight />
                    更多设置<span className="sl-help">权限与工作流</span>
                  </summary>
                  <div className="sl-row">
                    <div className="sl-field">
                      <label htmlFor="execution-access">权限模式</label>
                      <select
                        id="execution-access"
                        value={draft.execution.settings.access_mode ?? ""}
                        onChange={(e) =>
                          patchSettings({ access_mode: e.target.value })
                        }
                      >
                        <option value="">Runner 默认</option>
                        <option value="sandbox">沙箱</option>
                        <option value="full_access">完全访问</option>
                      </select>
                    </div>
                    <div className="sl-field">
                      <label htmlFor="execution-workflow">工作流</label>
                      <select
                        id="execution-workflow"
                        value={draft.workflow.id + "/" + draft.workflow.version}
                        onChange={(e) => {
                          const w = base.workflows.find(
                            (v) => v.id + "/" + v.version === e.target.value,
                          );
                          if (!w) return;
                          const workflow = {
                            ...draft.workflow,
                            id: w.id,
                            version: w.version,
                          };
                          if (w.audit_model_required && !workflow.audit_model)
                            workflow.audit_model = {
                              model,
                              ...(effort ? { reasoning_effort: effort } : {}),
                            };
                          if (!w.audit_model_required)
                            delete workflow.audit_model;
                          setDraft({ ...draft, workflow });
                        }}
                      >
                        <option
                          value={
                            draft.workflow.id + "/" + draft.workflow.version
                          }
                        >
                          {draft.workflow.id === "development-audited"
                            ? "审计开发"
                            : draft.workflow.id === "development"
                              ? "普通开发"
                              : draft.workflow.id}{" "}
                          / {draft.workflow.version}
                        </option>
                        {workflowChoices
                          .filter(
                            (w) =>
                              ["development", "development-audited"].includes(
                                w.id,
                              ) &&
                              w.supported_adapters.includes(
                                draft.execution.adapter,
                              ) &&
                              (w.id !== draft.workflow.id ||
                                w.version !== draft.workflow.version),
                          )
                          .map((w) => (
                            <option
                              key={w.id + w.version}
                              value={w.id + "/" + w.version}
                            >
                              {w.id === "development" ? "普通开发" : "审计开发"}{" "}
                              / {w.version}
                            </option>
                          ))}
                      </select>
                    </div>
                  </div>
                  {draft.workflow.audit_model && (
                    <div className="sl-row">
                      <div className="sl-field">
                        <label htmlFor="execution-audit-model">审计模型</label>
                        <select
                          id="execution-audit-model"
                          value={draft.workflow.audit_model.model}
                          onChange={(e) => {
                            const m = models.find(
                              (v) => v.id === e.target.value,
                            );
                            setDraft({
                              ...draft,
                              workflow: {
                                ...draft.workflow,
                                audit_model: {
                                  model: e.target.value,
                                  ...(m?.default_effort
                                    ? { reasoning_effort: m.default_effort }
                                    : {}),
                                },
                              },
                            });
                          }}
                        >
                          {modelOptions(draft.workflow.audit_model.model)}
                        </select>
                      </div>
                      <div className="sl-field">
                        <label htmlFor="execution-audit-effort">
                          审计 Thinking effort
                        </label>
                        <select
                          id="execution-audit-effort"
                          value={
                            draft.workflow.audit_model.reasoning_effort ?? ""
                          }
                          onChange={(e) =>
                            setDraft({
                              ...draft,
                              workflow: {
                                ...draft.workflow,
                                audit_model: {
                                  ...draft.workflow.audit_model!,
                                  reasoning_effort: e.target.value,
                                },
                              },
                            })
                          }
                        >
                          {effortOptions(
                            draft.workflow.audit_model.reasoning_effort ?? "",
                            models.find(
                              (m) => m.id === draft.workflow.audit_model?.model,
                            )?.efforts ??
                              schemaEfforts.map((value) => ({
                                value,
                                label: value,
                              })),
                          )}
                        </select>
                      </div>
                    </div>
                  )}
                  <p className="sl-help">
                    程序与其他高级参数沿用原配置，保存时完整保留。
                  </p>
                </details>
              </fieldset>
              <details className="sl-code">
                <summary>
                  <Code />
                  查看生成配置
                </summary>
                <pre>{JSON.stringify(draft, null, 2)}</pre>
              </details>
            </div>
            <footer className="sl-footer">
              <span className={"sl-footer-note " + (dirty ? "dirty" : "")}>
                {dirty ? "有未保存的修改" : "保存后需重启 SteerLine 生效"}
              </span>
              <div className="sl-inline">
                <button
                  type="button"
                  className="sl-button"
                  disabled={saving || !dirty}
                  onClick={() =>
                    saved
                      ? select(saved)
                      : select(base.config.execution_profiles[0])
                  }
                >
                  取消修改
                </button>
                <button
                  type="button"
                  className="sl-button primary"
                  disabled={
                    !dirty ||
                    saving ||
                    !valid ||
                    !base.writable ||
                    !editable(draft)
                  }
                  onClick={save}
                >
                  {saving ? "正在保存…" : "保存配置"}
                </button>
              </div>
            </footer>
          </section>
        </div>
      )}
      <div className="sl-service-line">
        <span className="sl-inline">
          <Globe />
          全局共享 · 所有项目可用
        </span>
        <span
          className={
            "sl-status " + (!applied || pendingRestart ? "pending" : "")
          }
        >
          <span
            className={"sl-dot " + (!applied || pendingRestart ? "warn" : "")}
          />
          {!applied
            ? "服务配置生效状态未知"
            : pendingRestart
              ? "服务仍使用已加载配置"
              : "当前服务已加载保存配置"}
        </span>
      </div>
      <div className="sl-toast" role="status">
        {notice}
      </div>
    </div>
  );
}

function changeModel(p: Profile, model: string, effort?: string): Profile {
  const next = clone(p);
  const settings = next.execution.settings;
  const fields =
    next.execution.adapter === "orca-terminal"
      ? (settings.agent_launch ??
        (settings.agent_launch = { executable: "codex" }))
      : settings;
  fields.model = model;
  if (effort !== undefined && effort !== "") fields.reasoning_effort = effort;
  else delete fields.reasoning_effort;
  return next;
}
