import { afterEach, expect, it, vi } from "vitest";
import { executionConfigAPI, stable } from "./execution-configs";
afterEach(() => vi.unstubAllGlobals());
it("uses only Web-owned routes and conditional writes", async () => {
  const calls: [string, RequestInit][] = [];
  vi.stubGlobal("fetch", async (url: string, init: RequestInit) => {
    calls.push([url, init]);
    return new Response(JSON.stringify({ saved_digest: "new" }), {
      status: 200,
    });
  });
  const api = executionConfigAPI("csrf");
  await api.discover({
    kind: "models",
    adapter: "orca-terminal",
    host: "orca_environment",
    environment: "machine",
    agent: "codex",
  });
  await api.save(
    {
      version: 3,
      runner: { executable: "test", version: "test", sha256: "test" },
      default_execution_profile: "local",
      execution_profiles: [],
    },
    "original",
  );
  expect(JSON.parse(calls[0][1].body as string).adapter).toBe("orca-terminal");
  expect(calls[0][0]).toBe("/api/web/execution-config/discover");
  expect(calls[1][1].method).toBe("PUT");
  expect(calls[1][1].headers).toMatchObject({
    "If-Match": '"original"',
    "X-CSRF-Token": "csrf",
  });
  expect(stable({ b: [1, 2], a: { y: true, x: "x" } })).toBe(
    stable({ a: { x: "x", y: true }, b: [1, 2] }),
  );
});
