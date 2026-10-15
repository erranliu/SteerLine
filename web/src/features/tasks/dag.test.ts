import { it, expect } from "vitest";
import { layout } from "./dag";
import type { GraphPage } from "../../shared/api/types";
it("positions visible dependencies and does not invent a missing boundary node", () => {
  const graph = {
    nodes: [
      {
        task_id: "B",
        title: "后继",
        status: "pending",
        dependency_ready: false,
      },
      {
        task_id: "A",
        title: "前置",
        status: "delivered",
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
