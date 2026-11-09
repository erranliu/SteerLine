import { expect, it } from "vitest";
import type { RoadmapTask } from "./roadmaps";
import {
  dependencyReview,
  visibleReviewEdges,
} from "./dependency-review-model";

function task(
  local_key: string,
  local: string[] = [],
  formal: string[] = [],
): RoadmapTask {
  return {
    local_key,
    title: local_key,
    key_origin: "markdown",
    goal: "交付",
    scope: "当前范围",
    constraints: "",
    acceptance: "通过",
    evidence: local_key,
    reason: "",
    existing_task_id: "",
    original_dependencies: { local, formal },
    proposed_dependencies: { local, formal },
  };
}

it("preserves every direct fork/join/independent edge and excludes unrelated formal tasks", () => {
  const tasks = [
    task("tools", [], ["101"]),
    task("seed", ["tools"]),
    task("left", ["seed"]),
    task("right", ["seed"]),
    task("join", ["left", "right"]),
    task("independent", ["tools"]),
    task("isolated"),
  ];
  const before = JSON.stringify(tasks);
  const graph = dependencyReview(tasks, [
    { id: "101", title: "基础" },
    { id: "105", title: "无关" },
  ]);
  expect(graph.nodes.map((node) => node.id)).toEqual([
    "formal:101",
    "local:independent",
    "local:isolated",
    "local:join",
    "local:left",
    "local:right",
    "local:seed",
    "local:tools",
  ]);
  expect(graph.edges.map((edge) => [edge.source, edge.target])).toEqual([
    ["formal:101", "local:tools"],
    ["local:tools", "local:seed"],
    ["local:seed", "local:left"],
    ["local:seed", "local:right"],
    ["local:left", "local:join"],
    ["local:right", "local:join"],
    ["local:tools", "local:independent"],
  ]);
  for (const edge of graph.edges) {
    expect(
      graph.nodes.find((node) => node.id === edge.source)!.position.x,
    ).toBeLessThan(
      graph.nodes.find((node) => node.id === edge.target)!.position.x,
    );
    expect(edge.change).toBe("kept");
  }
  expect(graph.proposedCount).toBe(7);
  expect(JSON.stringify(tasks)).toBe(before);
  expect(
    dependencyReview([...tasks].reverse(), [{ id: "101", title: "基础" }])
      .nodes,
  ).toEqual(graph.nodes);
});

it("compares formal aliases semantically without changing original references or merging a numeric local key", () => {
  const foundation = { ...task("foundation"), existing_task_id: "8" };
  const next = task("next", ["foundation", "foundation", "8"], ["8"]);
  next.proposed_dependencies = { local: ["8"], formal: ["8"] };
  const graph = dependencyReview(
    [foundation, task("8"), next],
    [{ id: "8", title: "基础" }],
  );
  expect(graph.nodes.map((node) => node.id)).toEqual([
    "formal:8",
    "local:8",
    "local:next",
  ]);
  expect(graph.nodes[0].task).toBe(foundation);
  expect(
    graph.edges.map((edge) => [edge.source, edge.target, edge.change]),
  ).toEqual([
    ["formal:8", "local:next", "kept"],
    ["local:8", "local:next", "kept"],
  ]);
  expect(next.original_dependencies.local).toEqual([
    "foundation",
    "foundation",
    "8",
  ]);
});

it("keeps reversed and removed-only relations out of proposed layout while retaining them for comparison", () => {
  const a = task("A", [], ["9"]);
  a.proposed_dependencies = { local: ["B"], formal: [] };
  const b = task("B", ["A"]);
  b.proposed_dependencies = { local: [], formal: [] };
  const graph = dependencyReview([a, b], [{ id: "9", title: "旧前置" }]);
  expect(graph.nodes.find((node) => node.id === "formal:9")?.originalOnly).toBe(
    true,
  );
  expect(
    visibleReviewEdges(graph.edges, "proposed").map((edge) => [
      edge.source,
      edge.target,
      edge.change,
    ]),
  ).toEqual([["local:B", "local:A", "added"]]);
  expect(visibleReviewEdges(graph.edges, "original")).toHaveLength(2);
  expect(visibleReviewEdges(graph.edges, "diff")).toHaveLength(3);
  expect(
    graph.nodes.find((node) => node.id === "local:B")!.position.x,
  ).toBeLessThan(graph.nodes.find((node) => node.id === "local:A")!.position.x);
});

it("uses unambiguous edge identities even when keys contain separators", () => {
  const graph = dependencyReview(
    [task("a"), task("a->b"), task("b->c", ["a"]), task("c", ["a->b"])],
    [],
  );
  expect(new Set(graph.edges.map((edge) => edge.id)).size).toBe(2);
  expect(graph.edges).toHaveLength(2);
});
