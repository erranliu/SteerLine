import { afterEach, expect, it, vi } from "vitest";
import {
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
} from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { configSnapshot } from "../../test/execution-config";
import { ContinuationSuggestionSettings } from "./continuation-suggestion-settings";
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});
it("uses the explicit local Codex model catalogue and preserves unrelated settings", async () => {
  let saved = structuredClone(configSnapshot);
  const writes: unknown[] = [];
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url, init) => {
      if (String(url).endsWith("/discover")) {
        expect(JSON.parse(String(init.body))).toMatchObject({
          kind: "models",
          host: "local",
          adapter: "codex-app-server",
          agent: "codex",
        });
        return new Response(
          JSON.stringify({
            state: "ready",
            models: [
              {
                id: "chosen",
                label: "Chosen model",
                default_effort: "high",
                efforts: [
                  { value: "medium", label: "Medium" },
                  { value: "high", label: "High" },
                ],
              },
            ],
          }),
        );
      }
      if (init?.method === "PUT") {
        const edit = JSON.parse(String(init.body));
        writes.push(edit);
        saved = { ...saved, config: edit.config, saved_digest: "next" };
      }
      return new Response(JSON.stringify(saved));
    }),
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <ContinuationSuggestionSettings
        home="isolated"
        csrf="csrf"
        applicationConnected
      />
    </QueryClientProvider>,
  );
  await screen.findByRole("button", { name: "读取本地 Codex 模型" });
  await waitFor(() =>
    expect(
      screen.getByRole("button", { name: "读取本地 Codex 模型" }),
    ).toBeEnabled(),
  );
  expect(screen.getByText("当前设置：gpt-6.1-sol / high")).toBeVisible();
  fireEvent.click(screen.getByRole("button", { name: "读取本地 Codex 模型" }));
  await screen.findByRole("option", { name: "Chosen model" });
  fireEvent.change(screen.getByLabelText("对话续作 · 提示词建议 模型"), {
    target: { value: "chosen" },
  });
  fireEvent.change(screen.getByLabelText("对话续作 · 提示词建议 思考强度"), {
    target: { value: "medium" },
  });
  fireEvent.click(
    screen.getByRole("button", { name: "保存对话续作 · 提示词建议 设置" }),
  );
  await waitFor(() => expect(writes).toHaveLength(1));
  expect(writes[0]).toMatchObject({
    config: {
      ...configSnapshot.config,
      continuation_suggestion: { model: "chosen", reasoning_effort: "medium" },
    },
    saved_digest: configSnapshot.saved_digest,
    profile_edits: [],
  });
  expect(
    await screen.findByText(
      "对话续作 · 提示词建议 设置已保存，重启 SteerLine 后生效。",
    ),
  ).toBeVisible();
  client.clear();
});
