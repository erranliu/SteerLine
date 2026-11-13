import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import {
  act,
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

test("confirms an existing DAG predecessor and admits only after verified merge on the same page", async () => {
  workspace.submit.mockClear();
  const task = {
    id: "single",
    kind: "task",
    title: "One task",
    status: "ready",
    revision: 2,
    card: {
      goal: "Goal",
      scope: "Scope",
      constraints: "",
      acceptance: "Acceptance",
    },
  };
  const record = {
    id: "documents",
    purpose: "documents",
    revision: 1,
    created_at: "2026-11-13",
    state: "documents-review",
    source: {
      id: task.id,
      planning_id: task.id,
      path: task.title,
      body: "",
      digest: "fixed",
    },
    documents: [],
    formal_mapping: {} as Record<string, string>,
    cancelled: false,
    review_digest: "review",
    publication: undefined as { url: string; state: string } | undefined,
  };
  let generated = false;
  workspace.api.get.mockImplementation(async (url: string) => {
    if (url.endsWith("/predecessors"))
      return [{ id: "7", title: "Existing task" }];
    if (url.endsWith("/history")) return { entries: [] };
    if (url.endsWith("/single")) return task;
    if (url.endsWith("/documents")) return { ...record };
    if (url === "/roadmaps")
      return {
        sources: [],
        records: generated ? [{ ...record }] : [],
        documents_available: true,
        preparation: {
          files: [
            "SteerlineFlow/Roadmap/document-rules.md",
            "SteerlineFlow/spec/SpecGuide.md",
          ].map((path) => ({ path, ready: true })),
        },
      };
    return { available: true, tasks: [task] };
  });
  workspace.api.envelope.mockImplementation(
    (action, target, revision, payload) => ({
      action,
      target,
      revision,
      payload,
    }),
  );
  workspace.submit.mockImplementation(async (request) => {
    if (request.action === "roadmap_task_generate") generated = true;
    if (request.action === "roadmap_publish") {
      record.state = "awaiting-merge";
      record.publication = { url: "https://example.com/pull/1", state: "pr" };
      record.revision++;
    }
    if (request.action === "roadmap_observe") {
      record.state = "awaiting-admission";
      record.publication = {
        url: "https://example.com/pull/1",
        state: "merged",
      };
      record.revision++;
    }
    if (request.action === "roadmap_admit") {
      record.state = "admitted";
      record.formal_mapping = { T1: "8" };
      record.revision++;
    }
    return { status: "applied" };
  });
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  });
  render(
    <QueryClientProvider client={client}>
      <MemoryRouter
        initialEntries={["/projects/project/roadmap-planning?task=single"]}
      >
        <RoadmapPlanning />
      </MemoryRouter>
    </QueryClientProvider>,
  );
  fireEvent.click(await screen.findByLabelText("#7 Existing task"));
  fireEvent.click(
    screen.getByRole("button", { name: "确认并生成 Spec / Plan" }),
  );
  await waitFor(() =>
    expect(workspace.submit).toHaveBeenCalledWith(
      expect.objectContaining({
        action: "roadmap_task_generate",
        target: task.id,
        payload: expect.objectContaining({ predecessors: ["7"] }),
      }),
    ),
  );
  fireEvent.click(
    await screen.findByRole("button", { name: "确认发布文档 PR" }),
  );
  await screen.findByRole("link", { name: "文档 PR" });
  expect(workspace.submit).toHaveBeenCalledWith(
    expect.objectContaining({
      action: "roadmap_publish",
      payload: { review_digest: "review", merge: false },
    }),
  );
  expect(screen.queryByRole("button", { name: "加入 DAG" })).toBeNull();
  fireEvent.click(screen.getByRole("button", { name: "查询原操作结果" }));
  fireEvent.click(await screen.findByRole("button", { name: "加入 DAG" }));
  await waitFor(() =>
    expect(workspace.submit).toHaveBeenCalledWith(
      expect.objectContaining({ action: "roadmap_admit", target: record.id }),
    ),
  );
  const dag = await screen.findByRole("link", { name: "查看正式任务 DAG" });
  expect(dag).toHaveAttribute("href", "/projects/project/dag?task=8");
  expect(screen.getByRole("heading", { name: "One task" })).toBeVisible();
});

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
      steer_agent: { model: "gpt-6.1-sol", reasoning_effort: "high" },
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
  let historyFailed = false;
  workspace.api.get.mockImplementation(async (url: string) => {
    if (url.endsWith("/history")) {
      if (historyFailed) throw new Error("history unavailable");
      return { entries: [{ role: "user", text: "上次规划意见" }] };
    }
    return url.endsWith("/question-task")
      ? task
      : {
          available: true,
          tasks: [task],
          steer_agent: { model: "gpt-6.1-sol", reasoning_effort: "high" },
        };
  });
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
  await screen.findByText("上次规划意见");
  const submit = screen.getByRole("button", { name: "提交回答" });
  expect(submit).toBeDisabled();
  fireEvent.change(screen.getByLabelText("执行顺序"), {
    target: { value: "先规划" },
  });
  fireEvent.change(screen.getByLabelText("理由"), {
    target: { value: "明确依赖" },
  });
  historyFailed = true;
  await act(() =>
    client.refetchQueries({
      queryKey: [
        "test",
        "project",
        "instance",
        "/roadmap-planning",
        "question-task",
        "history",
      ],
    }),
  );
  await screen.findByText("对话历史刷新失败，显示上次结果。");
  expect(screen.getByText("上次规划意见")).toBeVisible();
  expect(screen.getByLabelText("理由")).toHaveValue("明确依赖");
  expect(submit).toBeEnabled();
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
