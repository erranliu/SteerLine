import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, screen } from "@testing-library/react";
import { Api } from "../../shared/api/client";
import { identity, mountFeature, run, testClient } from "../../test/workspace";
import { LastMessage } from "./last-message";
import { RunUsage } from "./run-usage";

afterEach(cleanup);
const next = {
  ...run,
  run_id: "next",
  execution_id: "next-execution",
  attempt_id: "next-attempt",
};
it.each([LastMessage, RunUsage])(
  "%s switches the real query binding and ignores the old response",
  async (Component) => {
    const label = Component === LastMessage ? "当前候选消息" : "用量暂不支持";
    const reply = (target: typeof run) =>
      Component === LastMessage
        ? {
            run_id: target.execution_id,
            attempt_id: target.attempt_id,
            message: {
              run_id: target.execution_id,
              attempt_id: target.attempt_id,
              session_id: "session",
              turn_id: "turn",
              kind: "assistant",
              text: target === next ? label : "原候选消息",
              observed_at: "2026-10-22T00:00:00Z",
            },
          }
        : {
            run_id: target.run_id,
            execution_id: target.execution_id,
            usage: null,
            cost: null,
            reason: target === next ? "USAGE_UNSUPPORTED" : undefined,
          };
    const api = new Api(identity, "csrf", "home");
    const client = testClient();
    let signal: AbortSignal | undefined, finish!: (value: unknown) => void;
    const get = vi
      .spyOn(api, "get")
      .mockImplementation((path, requestSignal) => {
        if (path.includes(next.run_id))
          return Promise.resolve(reply(next)) as never;
        signal = requestSignal;
        return new Promise((resolve) => {
          finish = resolve;
        }) as never;
      });
    function Selected() {
      const [target, select] = useState(run);
      return (
        <>
          <button onClick={() => select(next)}>切换候选</button>
          <Component run={target} />
        </>
      );
    }
    mountFeature(<Selected />, api, client);
    fireEvent.click(screen.getByText("切换候选"));
    expect(signal?.aborted).toBe(true);
    await screen.findByText(label);
    await act(async () => finish(reply(run)));
    expect(screen.getByText(label)).toBeVisible();
    expect(
      get.mock.calls.some(([path]) => path.includes(next.execution_id)),
    ).toBe(true);
    client.clear();
  },
);
