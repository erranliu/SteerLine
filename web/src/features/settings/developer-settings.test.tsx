import { afterEach, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { MemoryRouter } from "react-router-dom";
import { SettingsPage } from "./settings";
import { configSnapshot } from "../../test/execution-config";
import type { ConfigSnapshot } from "../../shared/api/execution-configs";

// These tests exercise developer/generation sibling reconciliation. New settings
// have their own transport and component tests, so supply their independent reads.
vi.mock("../../shared/api/billing", async () => {
 const actual = await vi.importActual<typeof import("../../shared/api/billing")>("../../shared/api/billing");
 return { ...actual, billingAPI: () => ({ read: async () => ({schema:"steerline-billing/v1",generation:0,active_policy_version:"",policies:[],saved_digest:"",configured:false}) }) };
});
vi.mock("../../shared/api/usage-collection", async () => {
 const actual = await vi.importActual<typeof import("../../shared/api/usage-collection")>("../../shared/api/usage-collection");
 return { ...actual, usageCollection: async () => ({status:"ready",instance_id:"i",usage_policy:actual.defaultUsagePolicy(),usage_policy_digest:"p",usage_policy_revision:"i",requirements:[]}) };
});

const clients: QueryClient[] = [];
afterEach(() => {
  cleanup();
  clients.splice(0).forEach((c) => c.clear());
  vi.unstubAllGlobals();
});

function setup({
  available = true,
  readOnly = false,
  conflict = false,
  lost = false,
  skip = false,
} = {}) {
  let saved: ConfigSnapshot = structuredClone({
    ...configSnapshot,
    developer_available: available,
    writable: !readOnly,
  });
  saved.config.developer = { skip_environment_readiness: skip };
  const writes: unknown[] = [];
  const digests: string[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (_url, init) => {
      if (init?.method === "PUT") {
        const config = JSON.parse(String(init.body)).config;
        writes.push(config);
        const digest = new Headers(init.headers).get("If-Match")!;
        digests.push(digest);
        if (conflict || digest !== '"' + saved.saved_digest + '"')
          return new Response(
            JSON.stringify({
              error: { code: "configuration_changed", message: "conflict" },
            }),
            { status: 412 },
          );
        saved = {
          ...saved,
          config,
          saved_digest:
            writes.length === 1 ? "saved" : "saved-" + writes.length,
        };
        if (lost) throw new Error("response lost");
      }
      return new Response(JSON.stringify(saved), { status: 200 });
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  clients.push(client);
  const mount = (loadedDigest: string) => (
    <QueryClientProvider client={client}>
      <MemoryRouter>
        <SettingsPage
          home="developer-home"
          csrf="csrf"
          loadedDigest={loadedDigest}
          applicationConnected
        />
      </MemoryRouter>
    </QueryClientProvider>
  );
  const view = render(mount("loaded"));
  return {
    writes,
    digests,
    client,
    restart: () => view.rerender(mount("saved")),
  };
}

it("saves a global switch with restart semantics and preserves other configuration", async () => {
  const { writes, digests, restart } = setup();
  const toggle = await screen.findByRole("switch", {
    name: "环境未就绪时阻止执行",
  });
  expect(toggle).toBeChecked();
  expect(screen.getByText("当前生效状态：环境就绪阻塞已开启")).toBeVisible();
  fireEvent.click(toggle);
  // Editing the draft must not present it as effective.
  expect(screen.getByText("当前生效状态：环境就绪阻塞已开启")).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "保存开发者配置" }));
  await screen.findByText("全局配置已保存，待重启生效");
  expect(writes).toEqual([
    {
      ...configSnapshot.config,
      developer: { skip_environment_readiness: true },
    },
  ]);
  expect(digests).toEqual(['"loaded"']);
  expect(screen.getByRole("button", { name: "保存开发者配置" })).toBeDisabled();
  restart();
  await screen.findByText("当前生效状态：环境就绪阻塞已关闭");
});

it("preserves input after a concurrent save conflict and supports cancellation", async () => {
  const { writes } = setup({ conflict: true });
  const toggle = await screen.findByRole("switch", {
    name: "环境未就绪时阻止执行",
  });
  fireEvent.click(toggle);
  fireEvent.click(screen.getByRole("button", { name: "保存开发者配置" }));
  await screen.findByText(/配置已被其他页面修改，当前输入已保留/);
  expect(toggle).not.toBeChecked();
  expect(writes).toHaveLength(1);
  fireEvent.click(screen.getByRole("button", { name: "取消开发者配置修改" }));
  expect(toggle).toBeChecked();
  expect(screen.getByRole("button", { name: "保存开发者配置" })).toBeDisabled();
});

it.each([false, true])(
  "reconciles a lost save response in either direction, initial skip=%s",
  async (skip) => {
    const { writes } = setup({ lost: true, skip });
    fireEvent.click(
      await screen.findByRole("switch", { name: "环境未就绪时阻止执行" }),
    );
    fireEvent.click(screen.getByRole("button", { name: "保存开发者配置" }));
    await screen.findByText("开发者配置已保存。重启 SteerLine 后生效。");
    expect(writes).toHaveLength(1);
  },
);

it("hides the developer category when the backend build disables it", async () => {
  setup({ available: false });
  await screen.findByLabelText("生成最大并发数");
  await waitFor(() =>
    expect(
      screen.queryByRole("region", { name: "开发者配置" }),
    ).not.toBeInTheDocument(),
  );
  expect(screen.queryByRole("switch")).not.toBeInTheDocument();
});

it("respects frozen configuration", async () => {
  setup({ readOnly: true });
  expect(
    await screen.findByRole("switch", { name: "环境未就绪时阻止执行" }),
  ).toBeDisabled();
  expect(screen.getByRole("button", { name: "保存开发者配置" })).toBeDisabled();
});

it.each(["developer", "generation"])(
  "tracks saved and loaded state independently of the %s draft",
  async (draftPanel) => {
    const { restart } = setup();
    const toggle = await screen.findByRole("switch", {
      name: "环境未就绪时阻止执行",
    });
    const limit = await screen.findByLabelText("生成最大并发数");
    fireEvent.click(toggle);
    fireEvent.change(limit, { target: { value: "7" } });
    const sibling = draftPanel === "developer" ? "生成并发" : "开发者配置";
    fireEvent.click(screen.getByRole("button", { name: "保存" + sibling }));
    await screen.findByText(sibling + "已保存。重启 SteerLine 后生效。");
    const panel = within(
      screen.getByRole("region", {
        name: draftPanel === "developer" ? "开发者配置" : "生成并发设置",
      }),
    );
    const pending =
      draftPanel === "developer"
        ? "全局配置已保存，待重启生效"
        : "配置已保存，待重启生效";
    expect(panel.getByText(pending)).toBeVisible();
    restart();
    await waitFor(() =>
      expect(panel.queryByText(pending)).not.toBeInTheDocument(),
    );
    expect(toggle).not.toBeChecked();
    expect(limit).toHaveValue(7);
    expect(
      panel.getByRole("button", {
        name: "保存" + (draftPanel === "developer" ? "开发者配置" : "生成并发"),
      }),
    ).toBeEnabled();
    if (draftPanel === "developer") {
      expect(panel.getByText("当前生效状态：环境就绪阻塞已开启")).toBeVisible();
      expect(panel.getByText("开发者配置有未保存的修改")).toBeVisible();
    }
  },
);

it.each(["developer", "generation"])(
  "keeps a saved %s value when an earlier GET finishes late",
  async (panel) => {
    const { client } = setup();
    const toggle = await screen.findByRole("switch", {
      name: "环境未就绪时阻止执行",
    });
    const limit = await screen.findByLabelText("生成最大并发数");
    const key = ["developer-home", "execution-config"];
    const old = structuredClone(client.getQueryData<ConfigSnapshot>(key)!);
    let release!: (snapshot: ConfigSnapshot) => void;
    const pending = client
      .fetchQuery({
        queryKey: key,
        staleTime: 0,
        queryFn: ({ signal }) => {
          void signal;
          return new Promise<ConfigSnapshot>((resolve) => {
            release = resolve;
          });
        },
      })
      .catch(() => undefined);
    await waitFor(() => expect(release).toBeTypeOf("function"));
    if (panel === "developer") fireEvent.click(toggle);
    else fireEvent.change(limit, { target: { value: "7" } });
    const label = panel === "developer" ? "开发者配置" : "生成并发";
    fireEvent.click(screen.getByRole("button", { name: "保存" + label }));
    await screen.findByText(label + "已保存。重启 SteerLine 后生效。");
    release(old);
    await pending;
    await waitFor(() =>
      expect(client.getQueryData<ConfigSnapshot>(key)?.saved_digest).toBe(
        "saved",
      ),
    );
    if (panel === "developer") expect(toggle).not.toBeChecked();
    else expect(limit).toHaveValue(7);
  },
);

it.each(["developer", "generation"])(
  "shares the latest saved snapshot between clean panels, first=%s",
  async (first) => {
    const { writes, digests } = setup();
    const toggle = await screen.findByRole("switch", {
      name: "环境未就绪时阻止执行",
    });
    const limit = await screen.findByLabelText("生成最大并发数");
    async function saveDeveloper() {
      fireEvent.click(toggle);
      fireEvent.click(screen.getByRole("button", { name: "保存开发者配置" }));
      await screen.findByText("开发者配置已保存。重启 SteerLine 后生效。");
    }
    async function saveGeneration() {
      fireEvent.change(limit, { target: { value: "7" } });
      fireEvent.click(screen.getByRole("button", { name: "保存生成并发" }));
      await screen.findByText("生成并发已保存。重启 SteerLine 后生效。");
    }
    if (first === "developer") {
      await saveDeveloper();
      await saveGeneration();
    } else {
      await saveGeneration();
      await saveDeveloper();
    }
    expect(digests).toEqual(['"loaded"', '"saved"']);
    expect(writes).toHaveLength(2);
    expect(writes[1]).toMatchObject({
      developer: { skip_environment_readiness: true },
      max_generate_concurrency: 7,
      execution_profiles: configSnapshot.config.execution_profiles,
    });
  },
);

it("retains a dirty panel's original snapshot and never overwrites a sibling save", async () => {
  const { writes, digests } = setup();
  const limit = await screen.findByLabelText("生成最大并发数");
  fireEvent.change(limit, { target: { value: "7" } });
  fireEvent.click(
    await screen.findByRole("switch", { name: "环境未就绪时阻止执行" }),
  );
  fireEvent.click(screen.getByRole("button", { name: "保存开发者配置" }));
  await screen.findByText("开发者配置已保存。重启 SteerLine 后生效。");
  expect(limit).toHaveValue(7);
  fireEvent.click(screen.getByRole("button", { name: "保存生成并发" }));
  await screen.findByText(/配置已被其他页面修改，当前输入已保留/);
  expect(limit).toHaveValue(7);
  expect(digests).toEqual(['"loaded"', '"loaded"']);
  fireEvent.click(screen.getByRole("button", { name: "取消修改" }));
  fireEvent.change(limit, { target: { value: "7" } });
  fireEvent.click(screen.getByRole("button", { name: "保存生成并发" }));
  await screen.findByText("生成并发已保存。重启 SteerLine 后生效。");
  expect(digests[2]).toBe('"saved"');
  expect(writes[2]).toMatchObject({
    developer: { skip_environment_readiness: true },
    max_generate_concurrency: 7,
  });
});
