import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import {
  ActivityFeedbackProvider,
  SubmissionStatus,
  useActivityFeedback,
} from "./feedback";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
  vi.useRealTimers();
});
function fixture(reduced = false, broken = false) {
  vi.stubGlobal("matchMedia", () => ({ matches: reduced }));
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(
    function (this: HTMLElement) {
      if (broken) throw new Error("no geometry");
      return new DOMRect(
        this.tagName === "BUTTON" && this.textContent === "活动" ? 800 : 100,
        50,
        100,
        32,
      );
    },
  );
  const execute = vi.fn();
  const open = vi.fn();
  function Controls() {
    const feedback = useActivityFeedback();
    return (
      <>
        <button ref={feedback.entry} aria-expanded="false" onClick={open}>
          活动
        </button>
        <button
          onClick={(event) => {
            execute();
            feedback.notify("解析", event.currentTarget);
          }}
        >
          解析
        </button>
        <SubmissionStatus label="提交结果待核实" showActivity />
      </>
    );
  }
  render(
    <ActivityFeedbackProvider>
      <Controls />
    </ActivityFeedbackProvider>,
  );
  return { execute, open };
}
it("decorates a user activation once without opening, taking focus, or queueing other work", () => {
  vi.useFakeTimers();
  const { execute, open } = fixture();
  const button = screen.getByRole("button", { name: "解析" });
  button.focus();
  fireEvent.click(button);
  fireEvent.click(button);
  expect(execute).toHaveBeenCalledTimes(2);
  expect(document.querySelectorAll(".activity-feedback-flight")).toHaveLength(
    1,
  );
  expect(open).not.toHaveBeenCalled();
  expect(button).toHaveFocus();
  act(() => vi.advanceTimersByTime(401));
  expect(document.querySelector(".activity-feedback-decoration")).toBeNull();
  expect(screen.getByRole("status")).toHaveTextContent("提交结果待核实");
  fireEvent.click(screen.getByRole("button", { name: "查看活动" }));
  expect(open).toHaveBeenCalledTimes(1);
});
it.each(["reduce", "narrow", "broken", "hidden"])(
  "keeps submission and static feedback usable with %s decoration",
  (mode) => {
    if (mode === "narrow") vi.stubGlobal("innerWidth", 320);
    if (mode === "hidden")
      vi.spyOn(document, "visibilityState", "get").mockReturnValue("hidden");
    const { execute, open } = fixture(mode === "reduce", mode === "broken");
    fireEvent.click(screen.getByRole("button", { name: "解析" }));
    expect(execute).toHaveBeenCalledTimes(1);
    expect(document.querySelector(".activity-feedback-flight")).toBeNull();
    expect(screen.getByRole("status")).toHaveTextContent("提交结果待核实");
    expect(open).not.toHaveBeenCalled();
  },
);
it("ends stale geometry on viewport change without changing the operation", () => {
  const { execute } = fixture();
  fireEvent.click(screen.getByRole("button", { name: "解析" }));
  expect(document.querySelector(".activity-feedback-flight")).not.toBeNull();
  fireEvent(window, new Event("resize"));
  expect(document.querySelector(".activity-feedback-decoration")).toBeNull();
  expect(execute).toHaveBeenCalledTimes(1);
});
