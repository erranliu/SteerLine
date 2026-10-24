import { expect, it } from "vitest";
import {
  saveRequest,
  recordResult,
  requestByID,
  setJournalHome,
} from "./journal";
import type { Envelope, Receipt } from "./types";

it.each(["applied", "rejected"] as const)(
  "preserves terminal %s against late query errors",
  async (status) => {
    setJournalHome("stop-journal-" + status);
    const envelope: Envelope = {
      request_id: "original",
      project_id: "project",
      instance_id: "instance",
      action: "stop_candidate",
      target_id: "candidate",
      expected_version: 1,
      payload: { task_id: "18" },
    };
    await saveRequest(envelope);
    const receipt: Receipt = {
      ...envelope,
      original: envelope,
      status,
      sampled_at: new Date().toISOString(),
    };
    await recordResult(envelope.request_id, receipt);
    await recordResult(envelope.request_id, undefined, "late timeout");
    await recordResult(envelope.request_id, {
      ...receipt,
      status: "submitted",
    });
    const row = await requestByID(envelope.request_id);
    expect(row?.response?.status).toBe(status);
    expect(row?.error).toBeUndefined();
  },
);
