import { afterEach, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import type { Task } from "../../shared/api/types";
import { RunTask } from "./run-task";

afterEach(cleanup);

const task = {
  task_id: "104",
  title: "双前置汇合与交付",
  status: "open",
  dependency_ready: false,
} as Task;

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
