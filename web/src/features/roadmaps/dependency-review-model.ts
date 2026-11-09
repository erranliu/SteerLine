import type { Dependencies, RoadmapRecord, RoadmapTask } from "./roadmaps";

export type ReviewMode = "proposed" | "original" | "diff";
export type ReviewEdge = {
  id: string;
  source: string;
  target: string;
  change: "kept" | "added" | "removed";
};
export type ReviewNode = {
  id: string;
  label: string;
  title: string;
  formalID?: string;
  task?: RoadmapTask;
  originalOnly: boolean;
  position: { x: number; y: number };
};

// A display projection of one saved analysis, never a graph authority.
export function dependencyReview(
  tasks: RoadmapTask[],
  existing: RoadmapRecord["existing"],
) {
  const byKey = new Map(tasks.map((task) => [task.local_key, task]));
  const identity = (key: string): string => {
    const formal = byKey.get(key)?.existing_task_id;
    return formal ? `formal:${formal}` : `local:${key}`;
  };
  const nodes = new Map<string, ReviewNode>();
  const formalNode = (formalID: string) => {
    const id = `formal:${formalID}`;
    if (!nodes.has(id)) {
      nodes.set(id, {
        id,
        label: `#${formalID}`,
        title:
          existing?.find((task) => task.id === formalID)?.title ??
          "快照未提供标题",
        formalID,
        originalOnly: false,
        position: { x: 0, y: 0 },
      });
    }
    return id;
  };
  for (const task of tasks) {
    const id = task.existing_task_id
      ? formalNode(task.existing_task_id)
      : identity(task.local_key);
    nodes.set(id, {
      id,
      label: task.existing_task_id
        ? `#${task.existing_task_id} · ${task.local_key}`
        : task.local_key,
      title: nodes.get(id)?.title ?? task.title,
      formalID: task.existing_task_id || undefined,
      task,
      originalOnly: false,
      position: { x: 0, y: 0 },
    });
  }
  const edgesFor = (
    field: "original_dependencies" | "proposed_dependencies",
  ) => {
    const edges = new Map<string, Omit<ReviewEdge, "change">>();
    for (const task of tasks) {
      const deps: Dependencies = task[field];
      const target = identity(task.local_key);
      const sources = [
        ...deps.local.map(identity),
        ...deps.formal.map(formalNode),
      ];
      for (const source of sources) {
        const id = JSON.stringify([source, target]);
        edges.set(id, { id, source, target });
      }
    }
    return edges;
  };
  const original = edgesFor("original_dependencies");
  const proposed = edgesFor("proposed_dependencies");
  const edges: ReviewEdge[] = [...new Map([...original, ...proposed])].map(
    ([id, edge]) => ({
      ...edge,
      change: !original.has(id)
        ? "added"
        : !proposed.has(id)
          ? "removed"
          : "kept",
    }),
  );
  const proposedNodes = new Set(
    [...proposed.values()].flatMap((edge) => [edge.source, edge.target]),
  );
  const ordered = [...nodes.values()].sort((a, b) =>
    a.id < b.id ? -1 : a.id > b.id ? 1 : 0,
  );
  const levels = new Map<string, number>();
  // Only proposed edges determine columns. The diff union can contain a cycle.
  for (let pass = 0; pass < ordered.length; pass++) {
    let changed = false;
    for (const node of ordered) {
      if (levels.has(node.id)) continue;
      const parents = [...proposed.values()]
        .filter((edge) => edge.target === node.id)
        .map((edge) => edge.source);
      if (parents.every((id) => levels.has(id))) {
        levels.set(
          node.id,
          parents.length
            ? Math.max(...parents.map((id) => levels.get(id)!)) + 1
            : 0,
        );
        changed = true;
      }
    }
    if (!changed) break;
  }
  const offsets = new Map<number, number>();
  for (const node of ordered) {
    const level = levels.get(node.id) ?? 0;
    const y = offsets.get(level) ?? 0;
    node.position = { x: level * 240, y };
    node.originalOnly = !node.task && !proposedNodes.has(node.id);
    // Titles occupy two lines; reserve space for full wrapping draft keys.
    offsets.set(level, y + 132 + 24 * Math.ceil(node.label.length / 10));
  }
  return { nodes: ordered, edges, proposedCount: proposed.size };
}

export function visibleReviewEdges(edges: ReviewEdge[], mode: ReviewMode) {
  return edges.filter(
    (edge) =>
      mode === "diff" ||
      (mode === "original"
        ? edge.change !== "added"
        : edge.change !== "removed"),
  );
}
