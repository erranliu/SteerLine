import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { MemoryRouter, Route, Routes } from "react-router-dom";
import { expect, test, vi } from "vitest";
import RoadmapPlanning from "./planning";

const workspace = vi.hoisted(() => ({
  identity: { project_id: "project", instance_id: "instance" },
  online: true,
  api: { home: "test", get: vi.fn(), envelope: vi.fn() },
  submit: vi.fn(),
}));
vi.mock("../../app/context", () => ({ useWorkspace: () => workspace }));

test("reviews selected files and hands the stable link to the published source", async () => {
  const path = "SteerlineFlow/Roadmap/drafts/topic.md";
  const task = {
    id: "task",
    revision: 3,
    title: "Task",
    path,
    status: "ready",
    body: "# Draft",
    busy: false,
    can_message: true,
    can_rebase: true,
    source_id: "",
    review: {
      digest: "fixed-review",
      changes: [
        { path, after: btoa("# Draft") },
        { path: "include.txt", after: btoa("Selected") },
        { path: "keep.txt", after: btoa("Local only") },
      ],
    },
  };
  workspace.api.get.mockImplementation(async (url: string) => {
    if (url.endsWith("/history"))
      return { entries: [{ role: "user", text: "Original decision" }] };
    if (url.endsWith("/task")) return { ...task };
    return {
      available: true,
      tasks: [task],
      profiles: [{ id: "codex", name: "Codex" }],
    };
  });
  workspace.api.envelope.mockImplementation(
    (action, target, revision, payload) => ({
      action,
      target,
      revision,
      payload,
    }),
  );
  workspace.submit.mockImplementation(async () => {
    task.source_id = "published";
    return { status: "applied" };
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter
        initialEntries={["/projects/project/roadmap-planning?task=task"]}
      >
        <Routes>
          <Route
            path="/projects/project/roadmap-planning"
            element={<RoadmapPlanning />}
          />
          <Route
            path="/projects/project/roadmaps"
            element={<p>Published draft destination</p>}
          />
        </Routes>
      </MemoryRouter>
    </QueryClientProvider>,
  );
  await screen.findByText("Original decision");
  const review = screen.getByRole("region", { name: "发布审阅" });
  const extra = within(review).getByText("include.txt").closest("details");
  expect(extra).toBeTruthy();
  fireEvent.click(within(review).getByText("include.txt"));
  fireEvent.click(within(extra!).getByLabelText("纳入 PR"));
  fireEvent.click(screen.getByRole("button", { name: "确认并创建 PR" }));
  await waitFor(() =>
    expect(workspace.submit).toHaveBeenCalledWith({
      action: "roadmap_planning_publish",
      target: "task",
      revision: 3,
      payload: { digest: "fixed-review", paths: [path, "include.txt"] },
    }),
  );
  await screen.findByText("Published draft destination");
});

test("submits all fields against the original native question", async () => {
  workspace.submit.mockReset();
  workspace.submit.mockResolvedValue({ status: "applied" });
  const task = {
    id: "question-task",
    revision: 4,
    title: "Native question",
    path: "SteerlineFlow/Roadmap/drafts/question.md",
    status: "running",
    busy: true,
    can_message: false,
    can_rebase: false,
    question: {
      id: "original",
      native_questions: [
        {
          id: "choice",
          text: "执行顺序",
          options: [{ id: "first", label: "先规划" }],
        },
        { id: "reason", text: "理由", allow_free_text: true },
      ],
    },
  };
  workspace.api.get.mockImplementation(async (url: string) =>
    url.endsWith("/history")
      ? { entries: [] }
      : url.endsWith("/question-task")
        ? task
        : { available: true, tasks: [task], profiles: [] },
  );
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter
        initialEntries={[
          "/projects/project/roadmap-planning?task=question-task",
        ]}
      >
        <RoadmapPlanning />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  await screen.findByText("执行顺序");
  const submit = screen.getByRole("button", { name: "提交回答" });
  expect(submit).toBeDisabled();
  fireEvent.change(screen.getByLabelText("执行顺序"), {
    target: { value: "先规划" },
  });
  fireEvent.change(screen.getByLabelText("理由"), {
    target: { value: "明确依赖" },
  });
  fireEvent.click(submit);
  await waitFor(() =>
    expect(workspace.submit).toHaveBeenCalledWith({
      action: "roadmap_planning_answer",
      target: "question-task",
      revision: 4,
      payload: {
        question_id: "original",
        text: '{"choice":["先规划"],"reason":["明确依赖"]}',
      },
    }),
  );
});
