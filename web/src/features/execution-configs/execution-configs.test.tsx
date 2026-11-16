import { afterEach, expect, it, vi } from "vitest";
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { ExecutionConfigs } from "./execution-configs";
import { SettingsPage } from "../settings/settings";
import type { ConfigSnapshot } from "../../shared/api/execution-configs";
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
function fixture(
  options: {
    conflict?: boolean;
    configuration?: boolean;
    readOnly?: boolean;
    delay?: boolean;
    unknown?: boolean;
    defaults?: boolean;
    lost?: boolean;
    helper?: boolean;
    helperAgent?: "codex" | "pi";
    piProviders?: boolean;
    remoteAgents?: boolean;
    remotePair?: boolean;
    contentWorkflow?: boolean;
    onlyRemote?: boolean;
    externalModel?: string;
    localPair?: boolean;
    reloadProgram?: boolean;
    delayLocalDirectory?: { resolve?: () => void };
    defaultWithoutEffort?: boolean;
  } = {},
) {
  let saved: ConfigSnapshot = {
    environments: [],
    saved_digest: "old",
    writable: !options.readOnly,
    efforts: ["medium", "high", "ultra"],
    profile_views: {},
    combinations: [
      ...["codex-app-server", "orca-terminal"].map((adapter) => ({
        host: "local",
        adapter,
        agent: "codex",
        supported: true,
        efforts: ["medium", "high", "ultra"],
        workflows: [
          {
            id: "development",
            version: 2,
            supported_adapters: [adapter],
            audit_model_required: false,
          },
        ],
      })),
      ...["pi", "dsh"].map((agent) => ({
        host: "local",
        adapter: "orca-terminal",
        agent,
        supported: true,
        efforts:
          agent === "pi"
            ? ["minimal", "low", "medium", "high", "xhigh"]
            : ["low", "medium", "high", "max"],
        workflows: [
          {
            id: "development",
            version: 2,
            supported_adapters: ["orca-terminal"],
            audit_model_required: false,
          },
        ],
      })),
      {
        host: "orca_environment",
        adapter: "orca-terminal",
        agent: "codex",
        supported: true,
        efforts: ["medium", "high", "ultra"],
        workflows: [
          {
            id: "development",
            version: 2,
            supported_adapters: ["orca-terminal"],
            audit_model_required: false,
          },
        ],
      },
    ],
    workflows: [
      {
        id: "development",
        version: 2,
        supported_adapters: ["codex-app-server", "orca-terminal"],
        audit_model_required: false,
      },
    ],
    config: {
      version: 3,
      runner: { executable: "runner", version: "v", sha256: "sha" },
      default_execution_profile: "local",
      execution_profiles: [
        {
          id: "local",
          name: "本机日常",
          execution: {
            adapter: "codex-app-server",
            target: { kind: "local" },
            settings: {
              access_mode: "full_access",
              executable: "codex.exe",
              model: options.unknown ? "unconfirmed" : "large",
              reasoning_effort: "ultra",
              github_auth: { private_key_env: "EXAMPLE" },
            },
          },
          workflow: {
            id: "development",
            version: 2,
            prompt_overrides: { "implement.execute": "preserve" },
          },
        },
      ],
    },
  };
  if (options.contentWorkflow) {
    const p = saved.config.execution_profiles[0];
    p.workflow = { id: "generate", version: 1 };
    p.execution.settings.access_mode = "read_only";
  }
  if (options.helper)
    saved.config.execution_profiles.push({
      id: "orca-template",
      name: "本机 Orca 部署模板",
      execution: {
        adapter: "orca-terminal",
        target: { kind: "local" },
        settings: {
          access_mode: "full_access",
          agent: options.helperAgent ?? "codex",
          transport: { executable: "orca.exe" },
          agent_launch: {
            ...(options.defaultWithoutEffort
              ? { reasoning_effort: "high" }
              : {
                  executable: options.helperAgent === "pi" ? "pi.cmd" : "codex",
                }),
          },
        },
      },
      workflow: { id: "development", version: 2 },
    });
  if (options.defaults) {
    delete saved.config.execution_profiles[0].execution.settings.model;
    delete saved.config.execution_profiles[0].execution.settings
      .reasoning_effort;
  }
  if (
    options.localPair ||
    options.reloadProgram ||
    options.defaultWithoutEffort
  )
    delete saved.config.execution_profiles[0].execution.settings.github_auth;
  if (options.localPair) {
    const original = saved.config.execution_profiles[0];
    original.execution.settings.executable = "C:/chosen/first-codex.exe";
    const second = structuredClone(original);
    second.id = "local-second";
    second.name = "第二本机方案";
    second.execution.settings.executable = "C:/chosen/second-codex.exe";
    saved.config.execution_profiles.push(second);
  }
  if (options.remotePair) {
    for (const target of ["one", "two"])
      saved.config.execution_profiles.push({
        id: `remote-${target}`,
        name: `远端${target}方案`,
        execution: {
          adapter: "orca-terminal",
          target: { kind: "orca_environment", environment: target },
          settings: { agent: "codex", agent_launch: { model: "selected" } },
        },
        workflow: { id: "development", version: 2 },
      });
  }
  if (options.onlyRemote) {
    saved.config.execution_profiles = saved.config.execution_profiles.filter(
      (p) => p.execution.target.kind === "orca_environment",
    );
    saved.config.default_execution_profile =
      saved.config.execution_profiles[0].id;
  }
  const snapshot = () => {
    const environments: ConfigSnapshot["environments"] = [];
    for (const profile of saved.config.execution_profiles) {
      let group = environments.find(
        (g) =>
          g.adapter === profile.execution.adapter &&
          g.target.kind === profile.execution.target.kind &&
          g.target.environment === profile.execution.target.environment,
      );
      if (!group) {
        group = {
          adapter: profile.execution.adapter,
          target: profile.execution.target,
          profile_ids: [],
        };
        environments.push(group);
      }
      group.profile_ids.push(profile.id);
    }
    const profile_views: ConfigSnapshot["profile_views"] = {};
    for (const p of saved.config.execution_profiles) {
      const fields = (
        p.execution.adapter === "orca-terminal"
          ? p.execution.settings.agent_launch
          : p.execution.settings
      ) as { model?: string; reasoning_effort?: string } | undefined;
      profile_views[p.id] = {
        editable: !options.unknown,
        view: {
          adapter: p.execution.adapter,
          target: p.execution.target,
          agent: String(p.execution.settings.agent ?? "codex"),
          access_mode: String(
            p.execution.settings.access_mode ?? "full_access",
          ),
          model: fields?.model,
          reasoning_effort: fields?.reasoning_effort,
        },
      };
    }
    return { ...saved, environments, profile_views };
  };
  const writes: unknown[] = [];
  let reads = 0;
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: string, init?: RequestInit) => {
      const response = (body: unknown, status = 200) =>
        new Response(JSON.stringify(body), { status });
      if (url.endsWith("/discover")) {
        const q = JSON.parse(init!.body as string);
        if (q.kind === "environments")
          return response({
            state: "ready",
            environments: [{ id: "remote", name: "远端机器" }],
          });
        if (q.host === "orca_environment") {
          if (options.remoteAgents)
            return response({
              state: "ready",
              origin: "probe",
              agents: [
                { id: "codex", directory: "registered" },
                { id: "pi", directory: "registered" },
              ],
              models: [],
              context_unverified: true,
            });
          if (options.delay) await new Promise((r) => setTimeout(r, 120));
          return response({
            state: "unavailable",
            diagnostic: "远端不可达",
            models: [],
            origin: "unknown",
            context_unverified: true,
          });
        }
        if (q.adapter === "orca-terminal") {
          if (options.delay) await new Promise((r) => setTimeout(r, 120));
          if (q.agent === "pi") {
            if (options.piProviders)
              return response({
                state: "ready",
                origin: "probe",
                agents: [
                  { id: "pi", directory: "registered" },
                  { id: "custom-agent", directory: "registered" },
                ],
                models: [
                  {
                    id: "deepseek/deepseek-flash",
                    provider: "deepseek",
                    label: "DeepSeek direct",
                    efforts: [{ value: "high" }],
                    is_default: true,
                    default_effort: "high",
                  },
                  {
                    id: "openrouter/deepseek/deepseek-chat",
                    provider: "openrouter",
                    label: "OpenRouter DeepSeek",
                    efforts: [{ value: "high" }],
                  },
                ],
                context_unverified: true,
              });
            if (options.delay) await new Promise((r) => setTimeout(r, 120));
            return response({
              state: "unsupported",
              diagnostic: "Pi 未注册目录；可手动填写",
              models: [],
              agents: [{ id: "codex", directory: "registered" }],
              context_unverified: true,
            });
          }
          return response({
            state: "ready",
            origin: "probe",
            models: [
              {
                id: q.agent + "-local",
                label: q.agent + "-local",
                efforts: options.defaultWithoutEffort
                  ? []
                  : [{ value: "high" }],
                is_default: true,
                default_effort: options.defaultWithoutEffort
                  ? undefined
                  : "high",
              },
            ],
            context_unverified: true,
          });
        }
        if (options.reloadProgram) {
          const reloaded = saved.saved_digest === "reloaded";
          if (!reloaded && options.delayLocalDirectory)
            await new Promise<void>((resolve) => {
              options.delayLocalDirectory!.resolve = resolve;
            });
          return response({
            state: "ready",
            models: [
              {
                id: reloaded ? "new-program-model" : "old-program-model",
                label: reloaded ? "New program model" : "Old program model",
                efforts: [],
              },
            ],
          });
        }
        if (options.localPair)
          return response({
            state: "ready",
            models: [
              {
                id:
                  q.profile_id === "local-second"
                    ? "second-model"
                    : "first-model",
                label:
                  q.profile_id === "local-second"
                    ? "Second program model"
                    : "First program model",
                efforts: [],
              },
            ],
          });
        return response({
          state: "ready",
          origin: "probe",
          models: [
            {
              id: "large",
              label: "Large",
              efforts: [{ value: "high" }, { value: "ultra" }],
            },
            {
              id: "small",
              label: "Small",
              efforts: [{ value: "medium" }, { value: "high" }],
            },
          ],
        });
      }
      if (url.endsWith("/compose")) {
        const q = JSON.parse(init!.body as string);
        const base = saved.config.execution_profiles.find(
          (p) => p.id === q.base_profile_id,
        );
        const selection = q.selection;
        const matching =
          base &&
          base.execution.adapter === selection.adapter &&
          JSON.stringify(base.execution.target) ===
            JSON.stringify(selection.target);
        const settings: Record<string, unknown> = matching
          ? structuredClone(base.execution.settings)
          : {};
        settings.access_mode = selection.access_mode;
        const fields =
          selection.adapter === "orca-terminal"
            ? ((settings.agent_launch ??= {}) as Record<string, unknown>)
            : settings;
        if (selection.adapter === "orca-terminal")
          settings.agent = selection.agent;
        if (selection.model) fields.model = selection.model;
        if (selection.reasoning_effort)
          fields.reasoning_effort = selection.reasoning_effort;
        for (const key of q.unset) delete fields[key];
        return response({
          execution: {
            adapter: selection.adapter,
            target: selection.target,
            settings,
          },
          view: selection,
        });
      }
      if (init?.method === "PUT") {
        writes.push(JSON.parse(init.body as string).config);
        if (options.conflict) {
          if (options.externalModel)
            saved.config.execution_profiles[0].execution.settings.model =
              options.externalModel;
          return response(
            { code: "configuration_changed", reason: "changed" },
            412,
          );
        }
        saved = {
          ...saved,
          config: JSON.parse(init.body as string).config,
          saved_digest: "new",
        };
        if (options.lost) throw new TypeError("lost response");
        return response(snapshot());
      }
      if (
        options.reloadProgram &&
        url.endsWith("/execution-config") &&
        !init?.method &&
        ++reads > 1
      ) {
        saved.config.execution_profiles[0].execution.settings.executable =
          "C:/chosen/reloaded-codex.exe";
        saved.saved_digest = "reloaded";
      }
      return response(snapshot());
    }),
  );
  const query = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={query}>
      <MemoryRouter>
        {options.configuration ? (
          <SettingsPage
            home="test-home"
            csrf="csrf"
            loadedDigest="old"
            applicationConnected
          />
        ) : (
          <ExecutionConfigs
            home="test-home"
            csrf="csrf"
            loadedDigest="old"
            applicationConnected
          />
        )}
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return writes;
}
it("edits and saves without dropping advanced values, waiting for activation", async () => {
  const writes = fixture();
  const name = await screen.findByLabelText("配置名称");
  expect(screen.getByRole("button", { name: "保存配置" })).toBeDisabled();
  fireEvent.change(name, { target: { value: "已修改" } });
  fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
  await screen.findByText("执行方案已保存。");
  expect(writes).toHaveLength(1);
  expect(writes[0]).toMatchObject({
    execution_profiles: [
      {
        name: "已修改",
        execution: {
          settings: { github_auth: { private_key_env: "EXAMPLE" } },
        },
        workflow: { prompt_overrides: { "implement.execute": "preserve" } },
      },
    ],
  });
});
it("clears incompatible effort and protects unsaved changes", async () => {
  fixture();
  await screen.findByLabelText("配置名称");
  await screen.findByRole("option", { name: "Small" });
  fireEvent.change(screen.getByLabelText("模型"), {
    target: { value: "small" },
  });
  expect(screen.getByLabelText("Thinking effort")).toHaveValue("");
  expect(screen.getByRole("button", { name: "保存配置" })).toBeDisabled();
  fireEvent.change(screen.getByLabelText("Thinking effort"), {
    target: { value: "high" },
  });
  fireEvent.click(screen.getByRole("button", { name: "新建方案" }));
  await screen.findByText("当前修改尚未保存");
  fireEvent.click(screen.getByRole("button", { name: "继续编辑" }));
  expect(screen.getByLabelText("模型")).toHaveValue("small");
});
it("preserves the draft on a conditional save conflict", async () => {
  const writes = fixture({ conflict: true });
  const name = await screen.findByLabelText("配置名称");
  fireEvent.change(name, { target: { value: "保留草稿" } });
  fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
  await screen.findByText(/配置已被其他页面修改/);
  expect(screen.getByLabelText("配置名称")).toHaveValue("保留草稿");
  expect(writes).toHaveLength(1);
});
it("keeps unknown saved values, and late remote results cannot replace local selection", async () => {
  fixture({ delay: true, unknown: true });
  await screen.findByLabelText("配置名称");
  await screen.findByRole("option", { name: "远端机器" });
  expect(screen.getByLabelText("模型")).toHaveValue("unconfirmed");
  fireEvent.change(screen.getByLabelText("运行机"), {
    target: { value: "remote" },
  });
  fireEvent.change(screen.getByLabelText("运行机"), {
    target: { value: "local" },
  });
  await waitFor(() =>
    expect(screen.getByLabelText("运行机")).toHaveValue("local"),
  );
  await new Promise((r) => setTimeout(r, 180));
  expect(screen.queryByText("远端不可达")).not.toBeInTheDocument();
});

it("renames a profile without expanding native model defaults", async () => {
  const writes = fixture({ defaults: true });
  const name = await screen.findByLabelText("配置名称");
  await screen.findByRole("option", { name: "Small" });
  expect(screen.getByLabelText("模型")).toHaveValue("");
  fireEvent.change(name, { target: { value: "原生默认配置" } });
  fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
  await screen.findByText("执行方案已保存。");
  expect(writes).toHaveLength(1);
  const settings = (writes[0] as ConfigSnapshot["config"]).execution_profiles[0]
    .execution.settings;
  expect(settings).not.toHaveProperty("model");
  expect(settings).not.toHaveProperty("reasoning_effort");
});

it("reconciles a lost save response before claiming the result", async () => {
  const writes = fixture({ lost: true });
  const name = await screen.findByLabelText("配置名称");
  fireEvent.change(name, { target: { value: "回执丢失" } });
  fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
  await screen.findByText("执行方案已保存。");
  expect(writes).toHaveLength(1);
  expect(screen.getByLabelText("配置名称")).toHaveValue("回执丢失");
});

it("shows fixed development access without obsolete permission choices", async () => {
  const writes = fixture();
  await screen.findByLabelText("配置名称");
  fireEvent.click(
    screen.getByText("更多设置", { selector: "summary", exact: false }),
  );
  expect(screen.getByText("开发执行权限固定为完全访问。")).toBeVisible();
  expect(
    screen.queryByRole("combobox", { name: "权限模式" }),
  ).not.toBeInTheDocument();
  expect(
    screen.queryByRole("option", { name: /Runner 默认|沙箱/ }),
  ).not.toBeInTheDocument();
  fireEvent.change(screen.getByLabelText("配置名称"), {
    target: { value: "固定开发权限" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(
    (writes[0] as ConfigSnapshot["config"]).execution_profiles[0].execution
      .settings.access_mode,
  ).toBe("full_access");
});

it("preserves read-only content settings until explicitly switching to development", async () => {
  const writes = fixture({ contentWorkflow: true });
  await screen.findByLabelText("配置名称");
  fireEvent.change(screen.getByLabelText("配置名称"), {
    target: { value: "内容方案" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(
    (writes[0] as ConfigSnapshot["config"]).execution_profiles[0].execution
      .settings.access_mode,
  ).toBe("read_only");
  await screen.findByText("执行方案已保存。");
  fireEvent.click(
    screen.getByText("更多设置", { selector: "summary", exact: false }),
  );
  fireEvent.change(screen.getByLabelText("工作流"), {
    target: { value: "development/2" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
  await waitFor(() => expect(writes).toHaveLength(2));
  expect(
    (writes[1] as ConfigSnapshot["config"]).execution_profiles[0],
  ).toMatchObject({
    workflow: { id: "development", version: 2 },
    execution: { settings: { access_mode: "full_access" } },
  });
});

it("copies and deletes a saved non-default profile without deleting the default", async () => {
  const writes = fixture();
  await screen.findByLabelText("配置名称");
  expect(screen.getByRole("button", { name: "删除" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "复制" }));
  fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
  await screen.findByText("执行方案已保存。");
  await waitFor(() =>
    expect(screen.getByRole("button", { name: "删除" })).toBeEnabled(),
  );
  fireEvent.click(screen.getByRole("button", { name: "删除" }));
  fireEvent.click(screen.getByRole("button", { name: "确认删除" }));
  await waitFor(() => expect(writes).toHaveLength(2));
  expect(
    (writes[1] as ConfigSnapshot["config"]).execution_profiles,
  ).toHaveLength(1);
  expect(
    (writes[1] as ConfigSnapshot["config"]).default_execution_profile,
  ).toBe("local");
});

it("creates a local Orca Codex profile from a Pi-only local deployment with explicit development access", async () => {
  const writes = fixture({ helper: true, helperAgent: "pi" });
  await screen.findByDisplayValue("本机日常");
  fireEvent.click(screen.getByRole("button", { name: "新建方案" }));
  fireEvent.change(screen.getByLabelText("执行链路"), {
    target: { value: "orca-terminal" },
  });
  await screen.findByRole("option", { name: "codex-local" });
  fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  const created = (writes[0] as ConfigSnapshot["config"]).execution_profiles.at(
    -1,
  )!;
  expect(created.execution).toMatchObject({
    adapter: "orca-terminal",
    target: { kind: "local" },
    settings: {
      access_mode: "full_access",
      agent: "codex",
      agent_launch: { model: "codex-local" },
    },
  });
});

it("composes local and remote logical selections without retired helper fields", async () => {
  const writes = fixture({ helper: true, delay: true });
  await screen.findByDisplayValue("本机日常");
  fireEvent.change(screen.getByLabelText("执行链路"), {
    target: { value: "orca-terminal" },
  });
  await screen.findByRole("option", { name: "codex-local" });
  fireEvent.change(screen.getByLabelText("Agent"), { target: { value: "pi" } });
  fireEvent.change(screen.getByLabelText("Agent"), {
    target: { value: "pi" },
  });
  await screen.findByText("Pi 未注册目录；可手动填写");
  fireEvent.click(screen.getByRole("button", { name: "手动填写" }));
  fireEvent.change(screen.getByLabelText("模型标识"), {
    target: { value: "provider/manual" },
  });
  fireEvent.change(screen.getByLabelText("Thinking effort"), {
    target: { value: "high" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  const local = (writes[0] as ConfigSnapshot["config"]).execution_profiles[0];
  expect(local.workflow.prompt_overrides).toEqual({
    "implement.execute": "preserve",
  });
  expect(local.execution).toMatchObject({
    adapter: "orca-terminal",
    target: { kind: "local" },
    settings: {
      agent: "pi",
      agent_launch: {
        model: "provider/manual",
        reasoning_effort: "high",
      },
    },
  });
  fireEvent.change(screen.getByLabelText("运行机"), {
    target: { value: "remote" },
  });
  await screen.findByText("远端不可达");
  fireEvent.click(screen.getByRole("button", { name: "手动填写" }));
  fireEvent.change(screen.getByLabelText("模型标识"), {
    target: { value: "remote-model" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
  await waitFor(() => expect(writes).toHaveLength(2));
  const remote = (writes[1] as ConfigSnapshot["config"]).execution_profiles[0];
  expect(remote.execution.settings.agent).toBe("codex");
  expect(remote.execution.settings.agent_launch).not.toHaveProperty(
    "runtime_executable",
  );
  expect(remote.execution.settings.agent_launch).not.toHaveProperty(
    "runtime_sha256",
  );
});
it("allows logical local selection without retired helper gating and labels missing Pi catalogs", async () => {
  fixture();
  await screen.findByDisplayValue("本机日常");
  fireEvent.change(screen.getByLabelText("执行链路"), {
    target: { value: "orca-terminal" },
  });
  fireEvent.change(screen.getByLabelText("Agent"), { target: { value: "pi" } });
  await screen.findByText("Pi 未注册目录；可手动填写");
  expect(screen.queryByText("helper 未部署")).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "手动填写" }));
  fireEvent.change(screen.getByLabelText("模型标识"), {
    target: { value: "provider/manual" },
  });
  expect(screen.getByLabelText("Thinking effort").textContent).not.toContain(
    "ultra",
  );
  expect(screen.getByRole("button", { name: "保存配置" })).not.toBeDisabled();
  const calls = vi
    .mocked(fetch)
    .mock.calls.filter(([, init]) =>
      String(init?.body).includes('"kind":"models"'),
    );
  expect(
    calls.some(
      ([, init]) =>
        JSON.parse(String(init?.body)).adapter === "orca-terminal" &&
        JSON.parse(String(init?.body)).agent === "pi",
    ),
  ).toBe(true);
});

it("selects an Orca-provided Pi provider without storing credentials or widening remote execution", async () => {
  const writes = fixture({
    helper: true,
    piProviders: true,
    remoteAgents: true,
  });
  await screen.findByDisplayValue("本机日常");
  fireEvent.change(screen.getByLabelText("执行链路"), {
    target: { value: "orca-terminal" },
  });
  fireEvent.change(screen.getByLabelText("Agent"), { target: { value: "pi" } });
  await screen.findByRole("option", { name: "openrouter" });
  fireEvent.change(screen.getByLabelText("Provider"), {
    target: { value: "openrouter" },
  });
  expect(screen.getByLabelText("模型")).toHaveValue("");
  expect(screen.queryByRole("option", { name: "DeepSeek direct" })).toBeNull();
  fireEvent.change(screen.getByLabelText("模型"), {
    target: { value: "openrouter/deepseek/deepseek-chat" },
  });
  fireEvent.change(screen.getByLabelText("Thinking effort"), {
    target: { value: "high" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  const profile = (writes[0] as ConfigSnapshot["config"]).execution_profiles[0];
  expect(
    (profile.execution.settings.agent_launch as Record<string, unknown>)?.model,
  ).toBe("openrouter/deepseek/deepseek-chat");
  expect(profile.execution.settings).not.toHaveProperty("provider");
  expect(profile.execution.settings).not.toHaveProperty("api_key");
  expect(screen.getByText(/Provider 与 API key 请在目标 Orca/)).toBeVisible();
  fireEvent.change(screen.getByLabelText("运行机"), {
    target: { value: "remote" },
  });
  await screen.findByRole("option", {
    name: /Pi · 目录已发现，当前执行链路未支持/,
  });
  expect(
    screen.getByRole("option", { name: /Pi · 目录已发现，当前执行链路未支持/ }),
  ).toBeDisabled();
  expect(screen.queryByLabelText("Provider")).toBeNull();
  expect(screen.getByLabelText("Agent")).toHaveValue("codex");
});

it("GQ saves shared generation concurrency in configuration and requires restart", async () => {
  const writes = fixture({ configuration: true });
  const input = await screen.findByLabelText("生成最大并发数");
  expect(input).toHaveValue(5);
  fireEvent.change(input, { target: { value: "0" } });
  expect(screen.getByRole("button", { name: "保存生成并发" })).toBeDisabled();
  fireEvent.change(input, { target: { value: "3" } });
  fireEvent.click(screen.getByRole("button", { name: "保存生成并发" }));
  await screen.findByText("配置已保存，待重启生效");
  expect(writes).toHaveLength(1);
  expect(writes[0]).toMatchObject({
    max_generate_concurrency: 3,
    default_execution_profile: "local",
  });
  expect(screen.getByRole("button", { name: "保存生成并发" })).toBeDisabled();
});

it("configuration preserves input on a conditional conflict and never drops execution profiles", async () => {
  const writes = fixture({ configuration: true, conflict: true });
  fireEvent.change(await screen.findByLabelText("生成最大并发数"), {
    target: { value: "7" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存生成并发" }));
  await screen.findByText(/配置已被其他页面修改，当前输入已保留/);
  expect(screen.getByLabelText("生成最大并发数")).toHaveValue(7);
  expect(writes).toHaveLength(1);
  expect(writes[0]).toMatchObject({
    execution_profiles: [
      {
        execution: {
          settings: { github_auth: { private_key_env: "EXAMPLE" } },
        },
      },
    ],
  });
});

it("configuration reconciles a lost save response and disables writes in a frozen package", async () => {
  const writes = fixture({ configuration: true, lost: true });
  fireEvent.change(await screen.findByLabelText("生成最大并发数"), {
    target: { value: "6" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存生成并发" }));
  await screen.findByText("生成并发已保存。重启 SteerLine 后生效。");
  expect(writes).toHaveLength(1);
  cleanup();
  fixture({ configuration: true, readOnly: true });
  expect(await screen.findByLabelText("生成最大并发数")).toBeDisabled();
  expect(screen.getByRole("button", { name: "保存生成并发" })).toBeDisabled();
});

it("binds discovery to the complete saved remote target", async () => {
  fixture({ remotePair: true });
  fireEvent.click(await screen.findByRole("button", { name: /远端one方案/ }));
  await waitFor(() =>
    expect(
      vi.mocked(fetch).mock.calls.some(([, init]) => {
        const q = init?.body && JSON.parse(String(init.body));
        return q?.environment === "one" && q.profile_id === "remote-one";
      }),
    ).toBe(true),
  );
  fireEvent.click(screen.getByRole("button", { name: /远端two方案/ }));
  await waitFor(() =>
    expect(
      vi.mocked(fetch).mock.calls.some(([, init]) => {
        const q = init?.body && JSON.parse(String(init.body));
        return q?.environment === "two" && q.profile_id === "remote-two";
      }),
    ).toBe(true),
  );
});

it("keeps a manually selected model when an older directory response arrives", async () => {
  fixture({ delay: true });
  await screen.findByDisplayValue("本机日常");
  fireEvent.change(screen.getByLabelText("执行链路"), {
    target: { value: "orca-terminal" },
  });
  fireEvent.click(screen.getByRole("button", { name: "手动填写" }));
  fireEvent.change(screen.getByLabelText("模型标识"), {
    target: { value: "user-selected" },
  });
  await new Promise((r) => setTimeout(r, 220));
  expect(screen.getByLabelText("模型标识")).toHaveValue("user-selected");
});
it("reloads both the saved execution and its new logical view after a conflict", async () => {
  fixture({ conflict: true, externalModel: "external-new-model" });
  fireEvent.change(await screen.findByLabelText("配置名称"), {
    target: { value: "my rename" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
  await screen.findByText(/配置已被其他页面修改。当前草稿已保留/);
  fireEvent.click(screen.getByRole("button", { name: "重新读取配置" }));
  await waitFor(() =>
    expect(screen.getByLabelText("模型")).toHaveValue("external-new-model"),
  );
  expect(screen.getByRole("button", { name: "保存配置" })).toBeDisabled();
});
it("creates a logical default without an existing local source", async () => {
  fixture({ remotePair: true, onlyRemote: true });
  await screen.findByDisplayValue("远端one方案");
  fireEvent.click(screen.getByRole("button", { name: "新建方案" }));
  await screen.findByDisplayValue("新的执行方案");
  expect(screen.getByLabelText("执行链路")).toHaveValue("codex-app-server");
  await waitFor(() => {
    const calls = vi
      .mocked(fetch)
      .mock.calls.filter(
        ([url, init]) =>
          String(url).endsWith("/discover") &&
          JSON.parse(String(init?.body)).kind === "models",
      );
    expect(JSON.parse(String(calls.at(-1)?.[1]?.body))).toMatchObject({
      adapter: "codex-app-server",
      host: "local",
    });
    expect(JSON.parse(String(calls.at(-1)?.[1]?.body))).not.toHaveProperty(
      "profile_id",
    );
  });
});

it("discovers the selected saved program and keeps that source when copying", async () => {
  fixture({ localPair: true });
  await screen.findByRole("option", { name: "First program model" });
  fireEvent.click(screen.getByRole("button", { name: /第二本机方案/ }));
  await screen.findByRole("option", { name: "Second program model" });
  expect(
    screen.queryByRole("option", { name: "First program model" }),
  ).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "复制" }));
  await screen.findByDisplayValue("第二本机方案 副本");
  await screen.findByRole("option", { name: "Second program model" });
  const calls = vi
    .mocked(fetch)
    .mock.calls.filter(
      ([url, init]) =>
        String(url).endsWith("/discover") &&
        JSON.parse(String(init?.body)).kind === "models",
    );
  expect(calls).toHaveLength(3);
  expect(
    calls.slice(1).map(([, init]) => JSON.parse(String(init?.body)).profile_id),
  ).toEqual(["local-second", "local-second"]);
});

it("requeries a reloaded saved program and rejects its older directory response", async () => {
  const delayed: { resolve?: () => void } = {};
  fixture({
    reloadProgram: true,
    delayLocalDirectory: delayed,
    conflict: true,
  });
  await screen.findByDisplayValue("本机日常");
  await waitFor(() => expect(delayed.resolve).toBeDefined());
  const oldCall = vi
    .mocked(fetch)
    .mock.calls.find(
      ([url, init]) =>
        String(url).endsWith("/discover") &&
        JSON.parse(String(init?.body)).kind === "models",
    )!;
  fireEvent.change(screen.getByLabelText("配置名称"), {
    target: { value: "本地草稿" },
  });
  fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
  await screen.findByText(/配置已被其他页面修改。当前草稿已保留/);
  fireEvent.click(screen.getByRole("button", { name: "重新读取配置" }));
  await screen.findByRole("option", { name: "New program model" });
  expect(oldCall[1]?.signal?.aborted).toBe(true);
  await act(async () => delayed.resolve!());
  expect(
    screen.queryByRole("option", { name: "Old program model" }),
  ).toBeNull();
  expect(
    screen.getByRole("option", { name: "New program model" }),
  ).toBeInTheDocument();
});

it("keeps explicit effort removal when a saved base supplies a default model without effort", async () => {
  const writes = fixture({ helper: true, defaultWithoutEffort: true });
  await screen.findByDisplayValue("本机日常");
  fireEvent.change(screen.getByLabelText("执行链路"), {
    target: { value: "orca-terminal" },
  });
  await screen.findByRole("option", { name: "codex-local" });
  fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
  await waitFor(() => expect(writes).toHaveLength(1));
  const compose = vi
    .mocked(fetch)
    .mock.calls.find(([url]) => String(url).endsWith("/compose"))!;
  expect(JSON.parse(String(compose[1]?.body)).unset).toContain(
    "reasoning_effort",
  );
  const profile = (writes[0] as ConfigSnapshot["config"]).execution_profiles[0];
  expect(profile.execution.settings.agent_launch).not.toHaveProperty(
    "reasoning_effort",
  );
});
