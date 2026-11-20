import { describe, it, expect, vi } from "vitest";
import * as journal from "./journal";
import {
  api,
  envelope,
  receipt,
  response,
  setupClientTests,
} from "./client.test-support";
setupClientTests();

describe("immutable browser requests", () => {
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
});
