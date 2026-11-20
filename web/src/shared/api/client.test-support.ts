import { beforeEach, afterEach, vi } from "vitest";
import { Api } from "./client";
import * as journal from "./journal";
import type { Identity, Envelope, Receipt } from "./types";
export const identity = {
  project_id: "project-a",
  instance_id: "instance-a",
  ready: true,
} as Identity;
export const api = new Api(identity, "csrf");
export let envelope: Envelope;
export const response = (body: unknown, status = 200) =>
  new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
export const receipt = (
  status: Receipt["status"] = "applied",
  extra: Partial<Receipt> = {},
): Receipt => ({
  ...envelope,
  original: envelope,
  status,
  sampled_at: new Date().toISOString(),
  ...extra,
});
export function setupClientTests() {
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
        request: (
          name: string,
          options: LockOptions | (() => Promise<unknown>),
          callback?: () => Promise<unknown>,
        ) => {
          const work = typeof options === "function" ? options : callback!;
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
}
