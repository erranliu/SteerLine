import { afterEach, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { Task } from "../../shared/api/types";
import { RunTask } from "./run-task";

afterEach(cleanup);

const task = {
  task_id: "104",
  title: "双前置汇合与交付",
  status: "open",
  dependency_ready: false,
  predecessors: [
    { task_id: "101", delivered: true },
    { task_id: "102", delivered: false },
    { task_id: "103", delivered: false },
  ],
} as Task;

it("identifies blocked tasks, shows only undelivered predecessors and clears on release", () => {
  const onSelect = vi.fn();
  const { rerender } = render(
    <RunTask task={task} selected candidateCount={0} onSelect={onSelect} />,
  );
  expect(screen.getByText("依赖阻塞")).toHaveClass("status-dependency_blocked");
  expect(screen.getByText("等待 #102、#103 交付")).toBeVisible();
  const button = screen.getByRole("button");
  expect(button).toBeEnabled();
  expect(button).toHaveClass("active", "dependency-blocked");
  fireEvent.click(button);
  expect(onSelect).toHaveBeenCalledOnce();
  rerender(
    <RunTask
      task={{ ...task, dependency_ready: true }}
      selected
      candidateCount={0}
      onSelect={onSelect}
    />,
  );
  expect(screen.getByText("待执行")).toBeVisible();
  expect(screen.queryByText("依赖阻塞")).not.toBeInTheDocument();
  expect(screen.queryByText("等待 #102、#103 交付")).not.toBeInTheDocument();
  expect(button).not.toHaveClass("dependency-blocked");
});

it.each([
  ["in_progress", "执行中"],
  ["closed", "已交付"],
  ["discarded", "已放弃"],
])(
  "preserves %s business state despite first-start dependency facts",
  (status, text) => {
    render(
      <RunTask
        task={{ ...task, status }}
        selected={false}
        candidateCount={1}
        onSelect={() => {}}
      />,
    );
    expect(screen.getByText(text)).toBeVisible();
    expect(screen.queryByText("依赖阻塞")).not.toBeInTheDocument();
  },
);
