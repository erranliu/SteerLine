import { it, expect } from "vitest";
import { layout } from "./dag";
import type { GraphPage } from "../../shared/api/types";
it("positions visible dependencies and does not invent a missing boundary node", () => {
  const graph = {
    nodes: [
      {
        task_id: "B",
        title: "后继",
        status: "open",
        dependency_ready: false,
      },
      {
        task_id: "A",
        title: "前置",
        status: "closed",
        dependency_ready: true,
      },
    ],
    edges: [
      { predecessor: "A", successor: "B" },
      { predecessor: "missing", successor: "B" },
    ],
  } as GraphPage;
  const result = layout(graph, "B");
  expect(result.nodes.find((n) => n.id === "B")?.position.x).toBeGreaterThan(
    result.nodes.find((n) => n.id === "A")!.position.x,
  );
  expect(result.nodes.find((n) => n.id === "B")?.selected).toBe(true);
  expect(result.edges).toHaveLength(1);
  expect(result.nodes).toHaveLength(2);
});

it.each([
  {
    status: "closed",
    dependency_ready: false,
    startup_blockers: ["task_not_open"],
    blocked: false,
  },
  {
    status: "in_progress",
    dependency_ready: true,
    startup_blockers: ["task_not_open"],
    blocked: false,
  },
  {
    status: "discarded",
    dependency_ready: false,
    startup_blockers: ["task_not_open", "dispatch_paused"],
    blocked: false,
  },
  {
    status: "open",
    dependency_ready: false,
    startup_blockers: [],
    blocked: true,
  },
  {
    status: "open",
    dependency_ready: true,
    startup_blockers: ["dispatch_paused"],
    blocked: true,
  },
  {
    status: "open",
    dependency_ready: true,
    startup_blockers: [],
    blocked: false,
  },
])(
  "shows first-start blockers only for open tasks: $status / $startup_blockers / $dependency_ready",
  ({ blocked, ...task }) => {
    const graph: GraphPage = {
      nodes: [{ task_id: "101", title: "任务 101", blockers: [], ...task }],
      edges: [],
      has_more: false,
      graph_revision: 1,
      database_revision: 1,
      database_id: "test-database",
      boundary: { truncated: false, omitted_node_count: 0, reasons: [] },
    };
    const node = layout(graph, "101").nodes[0];
    expect(node.data.status).toBe(task.status);
    expect(node.data.blocked).toBe(blocked);
  },
);
