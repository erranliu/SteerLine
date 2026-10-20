import { beforeEach, afterEach, describe, it, expect, vi } from "vitest";
import { Api } from "./client";
import { webcrypto } from "node:crypto";
import * as journal from "./journal";
import type { Identity, Envelope, Receipt } from "./types";
const identity = {
  project_id: "project-a",
  instance_id: "instance-a",
  ready: true,
} as Identity;
const api = new Api(identity, "csrf");
let envelope: Envelope;

it.each([false, true])(
  "reads bounded final bytes across UTF-8 boundaries and verifies SHA (tamper=%s)",
  async (tamper) => {
    const text = "中文🙂的完整最终回复\n下一行";
    const bytes = new TextEncoder().encode(text);
    const digest = Array.from(
      new Uint8Array(await webcrypto.subtle.digest("SHA-256", bytes)),
      (b) => b.toString(16).padStart(2, "0"),
    ).join("");
    vi.stubGlobal("crypto", {
      randomUUID: crypto.randomUUID.bind(crypto),
      subtle: webcrypto.subtle,
    });
    const ref = {
      execution_id: "exec",
      attempt_id: "attempt",
      item_key: "final-message",
      revision: 7,
      sha256: digest,
      bytes: bytes.length,
      source: "canonical",
      completeness: "complete",
    };
    const fetch = vi.fn(async (path: string) => {
      if (path.endsWith("/final"))
        return response(
          { code: "response_too_large", reason: "read body ranges" },
          413,
        );
      if (path.includes("/conversation?"))
        return response({
          page: {
            events: [{ type: "message.completed", body: ref }],
            has_more: false,
          },
        });
      const offset = Number(
        new URL("http://local" + path).searchParams.get("offset"),
      );
      const next = offset === 0 ? 5 : bytes.length;
      const part = bytes.slice(offset, next);
      if (tamper && offset === 0) part[0] ^= 1;
      return response({
        item_key: ref.item_key,
        revision: 7,
        sha256: digest,
        offset,
        next_offset: next,
        total_bytes: bytes.length,
        data: btoa(String.fromCharCode(...part)),
        complete: next === bytes.length,
      });
    });
    vi.stubGlobal("fetch", fetch);
    const result = api.final({
      run_id: "run",
      execution_id: "exec",
      attempt_id: "attempt",
    });
    if (tamper) await expect(result).rejects.toThrow("SHA 校验失败");
    else expect((await result).bodies[0].text).toBe(text);
    expect(fetch).toHaveBeenCalledTimes(4);
  },
);
const response = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
const receipt = (
  status: Receipt["status"] = "applied",
  extra: Partial<Receipt> = {},
): Receipt => ({
  ...envelope,
  original: envelope,
  status,
  sampled_at: new Date().toISOString(),
  ...extra,
});
beforeEach(async () => {
  envelope = {
    request_id: crypto.randomUUID().replaceAll("-", ""),
    project_id: "project-a",
    instance_id: "instance-a",
    action: "start_task",
    target_id: "8",
    expected_version: 4,
    payload: { execution_profile_id: "original" },
  };
  const db = await journal.openJournal();
  await new Promise<void>((resolve, reject) => {
    const tx = db.transaction(["requests", "drafts"], "readwrite");
    tx.objectStore("requests").clear();
    tx.objectStore("drafts").clear();
    tx.oncomplete = () => resolve();
    tx.onerror = () => reject(tx.error);
  });
  const locks = new Map<string, Promise<unknown>>();
  Object.defineProperty(navigator, "locks", {
    configurable: true,
    value: {
      request: (name: string, work: () => Promise<unknown>) => {
        const next = (locks.get(name) ?? Promise.resolve())
          .catch(() => {})
          .then(work);
        locks.set(name, next);
        return next;
      },
    },
  });
});
afterEach(() => vi.unstubAllGlobals());
describe("immutable browser requests", () => {
  it.each([
    ["start_environment_check", "command_not_found"],
    ["stop_environment_check", "command_not_found"],
    ["start_environment_check", "request_not_found"],
    ["stop_environment_check", "request_not_found"],
  ])(
    "keeps unknown %s query-only on %s, including repeat submit",
    async (action, code) => {
      envelope.action = action;
      await journal.saveRequest(envelope);
      await journal.recordResult(
        envelope.request_id,
        undefined,
        "lost response",
      );
      const fetch = vi.fn(async (_path: string, _init?: RequestInit) =>
        response({ code, reason: "original outcome is unavailable" }, 404),
      );
      vi.stubGlobal("fetch", fetch);
      const reopened = new Api(identity, "csrf");
      await expect(reopened.reconcile(envelope)).rejects.toThrow();
      await expect(reopened.submit(envelope)).rejects.toThrow();
      expect(fetch).toHaveBeenCalledTimes(2);
      expect(
        fetch.mock.calls.every(
          ([, init]) => !init?.method || init.method === "GET",
        ),
      ).toBe(true);
      expect(
        (await journal.requestByID(envelope.request_id))?.envelope,
      ).toEqual(envelope);
      expect(
        (await journal.requestByID(envelope.request_id))?.response,
      ).toBeUndefined();
    },
  );
  it("snapshots caller-owned data before waiting for a lock or persistence", async () => {
    const original = structuredClone(envelope);
    const fetch = vi.fn(async (_path, init) => {
      expect(JSON.parse(init.body)).toEqual(original);
      return response({
        ...original,
        original,
        status: "applied",
        sampled_at: "now",
      });
    });
    vi.stubGlobal("fetch", fetch);
    const sending = api.submit(envelope);
    envelope.target_id = "changed-after-submit";
    envelope.payload.execution_profile_id = "changed-profile";
    await sending;
    expect((await journal.requestByID(original.request_id))?.envelope).toEqual(
      original,
    );
  });
  it("does not replay authoritative absence after reconciliation is cancelled", async () => {
    await journal.saveRequest(envelope);
    let resolve!: (value: Response) => void;
    const fetch = vi.fn(
      () =>
        new Promise<Response>((done) => {
          resolve = done;
        }),
    );
    vi.stubGlobal("fetch", fetch);
    const controller = new AbortController();
    const reconciling = api.reconcile(envelope, controller.signal);
    await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
    controller.abort();
    resolve(response({ code: "command_not_found" }, 404));
    await expect(reconciling).rejects.toThrow();
    expect(fetch).toHaveBeenCalledTimes(1);
  });
  it("commits the whole envelope before POST, preserving target and version", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (_path, init) => {
        expect(
          (await journal.requestByID(envelope.request_id))?.envelope,
        ).toEqual(envelope);
        expect(JSON.parse(init.body)).toEqual(envelope);
        return response(receipt());
      }),
    );
    await api.submit(envelope);
    expect(
      (await journal.requestByID(envelope.request_id))?.response?.status,
    ).toBe("applied");
  });
  it("does not send when IndexedDB save fails", async () => {
    vi.spyOn(journal, "saveRequest").mockRejectedValueOnce(
      new Error("storage failed"),
    );
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    await expect(api.submit(envelope)).rejects.toThrow("storage failed");
    expect(fetch).not.toHaveBeenCalled();
  });
  it("retains network failure and queries the original ID after a new client opens", async () => {
    const fetch = vi
      .fn()
      .mockRejectedValueOnce(new Error("response lost"))
      .mockResolvedValueOnce(response(receipt()));
    vi.stubGlobal("fetch", fetch);
    await expect(api.submit(envelope)).rejects.toThrow("response lost");
    expect(await journal.requestByID(envelope.request_id)).toBeDefined();
    await new Api(identity, "new-csrf").reconcile(envelope);
    expect(fetch.mock.calls[1][0]).toBe(
      "/api/v2/application/projects/project-a/commands/" + envelope.request_id,
    );
    expect(fetch.mock.calls[1][1].method).toBeUndefined();
    expect(fetch).toHaveBeenCalledTimes(2);
  });
  it("never resends an unknown native operation", async () => {
    await journal.saveRequest(envelope);
    const fetch = vi
      .fn()
      .mockResolvedValue(response(receipt("outcome_unknown")));
    vi.stubGlobal("fetch", fetch);
    await api.reconcile(envelope);
    expect(fetch).toHaveBeenCalledTimes(1);
    expect(fetch.mock.calls[0][1].method).toBeUndefined();
  });
  it("resends only a formally absent request with exactly its saved envelope", async () => {
    await journal.saveRequest(envelope);
    const fetch = vi
      .fn()
      .mockResolvedValueOnce(
        response(
          { code: "command_not_found", reason: "formal authority absent" },
          404,
        ),
      )
      .mockResolvedValueOnce(response(receipt()));
    vi.stubGlobal("fetch", fetch);
    await api.reconcile(envelope);
    expect(fetch.mock.calls[1][1].method).toBe("POST");
    expect(JSON.parse(fetch.mock.calls[1][1].body)).toEqual(envelope);
  });
  it("does not treat a query failure as absence or retarget to a new instance", async () => {
    await journal.saveRequest(envelope);
    const fetch = vi
      .fn()
      .mockImplementation(async () =>
        response({ code: "backend_unavailable", reason: "SQL failed" }, 503),
      );
    vi.stubGlobal("fetch", fetch);
    await expect(api.reconcile(envelope)).rejects.toThrow("SQL failed");
    expect(fetch).toHaveBeenCalledTimes(1);
    await expect(
      new Api({ ...identity, instance_id: "replacement" }, "csrf").reconcile(
        envelope,
      ),
    ).rejects.toThrow("SQL failed");
    expect(fetch).toHaveBeenCalledTimes(2);
  });
  it("serializes simultaneous sends of the same original request", async () => {
    const fetch = vi.fn().mockResolvedValue(response(receipt()));
    vi.stubGlobal("fetch", fetch);
    await Promise.all([api.submit(envelope), api.submit(envelope)]);
    expect(fetch).toHaveBeenCalledTimes(1);
  });
  it("rejects a changed original envelope and mismatched receipts", async () => {
    await journal.saveRequest(envelope);
    await expect(
      journal.saveRequest({ ...envelope, target_id: "other" }),
    ).rejects.toThrow();
    const fetch = vi
      .fn()
      .mockResolvedValue(response(receipt("applied", { target_id: "other" })));
    vi.stubGlobal("fetch", fetch);
    await expect(api.reconcile(envelope)).rejects.toThrow("不匹配");
    expect(
      (await journal.requestByID(envelope.request_id))?.response,
    ).toBeUndefined();
  });
  it("keeps a committed terminal receipt when a late observation is unknown", async () => {
    await journal.saveRequest(envelope);
    await journal.recordResult(envelope.request_id, receipt());
    await journal.recordResult(
      envelope.request_id,
      receipt("outcome_unknown"),
      "late",
    );
    expect(
      (await journal.requestByID(envelope.request_id))?.response?.status,
    ).toBe("applied");
  });
});

it.each(["foreign-attempt", "repeated-cursor"])(
  "rejects invalid final pagination: %s",
  async (reason) => {
    const fetch = vi.fn(async (path: string) =>
      path.endsWith("/final")
        ? response({ code: "response_too_large" }, 413)
        : response({
            page: {
              events:
                reason === "foreign-attempt"
                  ? [{ body: { execution_id: "exec", attempt_id: "foreign" } }]
                  : [],
              has_more: true,
              next_cursor: "same",
            },
          }),
    );
    vi.stubGlobal("fetch", fetch);
    await expect(
      api.final({ run_id: "run", execution_id: "exec", attempt_id: "attempt" }),
    ).rejects.toThrow(reason === "foreign-attempt" ? "其他执行" : "游标重复");
    expect(fetch).toHaveBeenCalledTimes(reason === "foreign-attempt" ? 2 : 3);
  },
);

it.each(["start_task", "runner_resume", "stop_candidate"])(
  "blocks a replacement %s request while the original target is uncertain",
  async (action) => {
    envelope.action = action;
    await journal.saveRequest(envelope);
    const fetch = vi.fn();
    vi.stubGlobal("fetch", fetch);
    await expect(
      api.submit({
        ...envelope,
        request_id: crypto.randomUUID().replaceAll("-", ""),
      }),
    ).rejects.toThrow("待核实操作");
    expect(fetch).not.toHaveBeenCalled();
    expect(await journal.listRequests()).toHaveLength(1);
  },
);

it("sends independently confirmed candidates concurrently while preserving same-request deduplication", async () => {
  envelope.action = "start_candidate";
  const second = {
    ...envelope,
    request_id: crypto.randomUUID().replaceAll("-", ""),
  };
  const replies = new Map<string, (response: Response) => void>();
  const fetch = vi.fn(
    (_path, init) =>
      new Promise<Response>((resolve) => {
        replies.set(JSON.parse(init.body).request_id, resolve);
      }),
  );
  vi.stubGlobal("fetch", fetch);
  const one = api.submit(envelope);
  await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(1));
  const two = api.submit(second);
  const replay = api.submit(envelope);
  await vi.waitFor(() => expect(fetch).toHaveBeenCalledTimes(2));
  expect(await journal.listRequests()).toHaveLength(2);
  replies.get(second.request_id)!(
    response(receipt("applied", { ...second, original: second })),
  );
  replies.get(envelope.request_id)!(response(receipt()));
  const results = await Promise.all([one, two, replay]);
  expect(results.map((result) => result.request_id)).toEqual([
    envelope.request_id,
    second.request_id,
    envelope.request_id,
  ]);
  expect(fetch).toHaveBeenCalledTimes(2);
});
