import { afterEach, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { ExecutionConfigs } from "./execution-configs";
import type { ConfigSnapshot } from "../../shared/api/execution-configs";
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
function fixture(
  options: {
    conflict?: boolean;
    delay?: boolean;
    unknown?: boolean;
    defaults?: boolean;
    lost?: boolean;
    helper?: boolean;
  } = {},
) {
  let saved: ConfigSnapshot = {
    saved_digest: "old",
    writable: true,
    efforts: ["medium", "high", "ultra"],
    local_helper: { available: !!options.helper, reason: "helper 未部署" },
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
  if (options.helper)
    saved.config.execution_profiles.push({
      id: "orca-template",
      name: "本机 Orca 部署模板",
      execution: {
        adapter: "orca-terminal",
        target: { kind: "local" },
        settings: {
          agent: "codex",
          transport: { executable: "orca.exe" },
          agent_launch: {
            executable: "codex",
            runtime_executable: "C:/deployment/helper.exe",
            runtime_sha256: "a".repeat(64),
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
  const writes: unknown[] = [];
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
          if (q.agent === "pi") {
            if (options.delay) await new Promise((r) => setTimeout(r, 120));
            return response({
              state: "unsupported",
              diagnostic: "Pi 未注册目录；可手动填写",
              models: [],
              agents: [{ id: "pi", supported: true, directory: "unverified" }],
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
                efforts: [{ value: "high" }],
                is_default: true,
                default_effort: "high",
              },
            ],
            context_unverified: true,
          });
        }
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
      if (init?.method === "PUT") {
        writes.push(JSON.parse(init.body as string));
        if (options.conflict)
          return response(
            { code: "configuration_changed", reason: "changed" },
            412,
          );
        saved = {
          ...saved,
          config: JSON.parse(init.body as string),
          saved_digest: "new",
        };
        if (options.lost) throw new TypeError("lost response");
        return response(saved);
      }
      return response(saved);
    }),
  );
  const query = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  render(
    <QueryClientProvider client={query}>
      <MemoryRouter>
        <ExecutionConfigs
          home="test-home"
          csrf="csrf"
          loadedDigest="old"
          applicationConnected
        />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  return writes;
}
it("edits and saves without dropping advanced values, showing pending restart", async () => {
  const writes = fixture();
  const name = await screen.findByLabelText("配置名称");
  expect(screen.getByRole("button", { name: "保存配置" })).toBeDisabled();
  fireEvent.change(name, { target: { value: "已修改" } });
  fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
  await screen.findByText("配置已保存，待重启生效");
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
  fireEvent.click(screen.getByRole("button", { name: "新建配置" }));
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
  await screen.findByText("配置已保存，待重启生效");
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
  await screen.findByText("配置已保存，待重启生效");
  expect(writes).toHaveLength(1);
  expect(screen.getByLabelText("配置名称")).toHaveValue("回执丢失");
});

it("copies and deletes a saved non-default profile without deleting the default", async () => {
  const writes = fixture();
  await screen.findByLabelText("配置名称");
  expect(screen.getByRole("button", { name: "删除" })).toBeDisabled();
  fireEvent.click(screen.getByRole("button", { name: "复制" }));
  fireEvent.click(screen.getByRole("button", { name: "保存配置" }));
  await screen.findByText("配置已保存，待重启生效");
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

it("maps local Orca agents, inherits deployment material and strips it remotely", async () => {
  const writes = fixture({ helper: true, delay: true });
  await screen.findByDisplayValue("本机日常");
  fireEvent.change(screen.getByLabelText("执行链路"), {
    target: { value: "orca-terminal" },
  });
  await screen.findByRole("option", { name: "codex-local" });
  fireEvent.change(screen.getByLabelText("Agent"), { target: { value: "pi" } });
  fireEvent.change(screen.getByLabelText("Agent"), {
    target: { value: "dsh" },
  });
  await screen.findByRole("option", { name: "dsh-local" });
  await new Promise((r) => setTimeout(r, 160));
  expect(screen.getByLabelText("模型")).toHaveValue("dsh-local");
  expect(screen.getByLabelText("Thinking effort").textContent).not.toContain(
    "ultra",
  );
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
      agent: "dsh",
      agent_launch: {
        executable: "dsh.cmd",
        model: "dsh-local",
        reasoning_effort: "high",
        runtime_executable: "C:/deployment/helper.exe",
        runtime_sha256: "a".repeat(64),
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
it("keeps local directory querying available without helper and labels missing Pi catalogs", async () => {
  fixture();
  await screen.findByDisplayValue("本机日常");
  fireEvent.change(screen.getByLabelText("执行链路"), {
    target: { value: "orca-terminal" },
  });
  fireEvent.change(screen.getByLabelText("Agent"), { target: { value: "pi" } });
  await screen.findByText("Pi 未注册目录；可手动填写");
  expect(screen.getByText("helper 未部署")).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "手动填写" }));
  fireEvent.change(screen.getByLabelText("模型标识"), {
    target: { value: "provider/manual" },
  });
  expect(screen.getByLabelText("Thinking effort").textContent).not.toContain(
    "ultra",
  );
  expect(screen.getByRole("button", { name: "保存配置" })).toBeDisabled();
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
