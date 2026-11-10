import { useMemo, useState } from "react";
import { Link } from "react-router-dom";
import {
  ReactFlow,
  Controls,
  Handle,
  Position,
  MarkerType,
  type Node,
  type NodeProps,
} from "@xyflow/react";
import type { Dependencies, RoadmapRecord } from "./roadmaps";
import { dependencyReview, type ReviewNode } from "./dependency-review-model";
import "@xyflow/react/dist/style.css";

const changeLabels = { kept: "保留", added: "新增", removed: "移除" };
type PreviewNode = Node<ReviewNode, "review">;
function PreviewTask({ data }: NodeProps<PreviewNode>) {
  return (
    <div className={`roadmaps-review-node${data.formalID ? " formal" : ""}`}>
      <Handle type="target" position={Position.Left} />
      <small>{data.formalID ? "已有正式任务" : "草稿任务"}</small>
      <strong>{data.label}</strong>
      <span>{data.title}</span>
      <Handle type="source" position={Position.Right} />
    </div>
  );
}
const nodeTypes = { review: PreviewTask };

export function DependencyReview({
  analysis,
  existing,
  dag,
}: {
  analysis: NonNullable<RoadmapRecord["analysis"]>;
  existing: RoadmapRecord["existing"];
  dag: (id: string) => string;
}) {
  const graph = useMemo(
    () => dependencyReview(analysis.tasks, existing),
    [analysis.tasks, existing],
  );
  const [selected, setSelected] = useState("");
  const visible = graph.proposedEdges;
  const node = graph.nodes.find((item) => item.id === selected);
  const task = node?.task;
  const changes = graph.edges.filter((edge) => edge.change !== "kept");
  const neighbors = new Set(
    visible
      .filter((edge) => edge.source === selected || edge.target === selected)
      .flatMap((edge) => [edge.source, edge.target]),
  );
  const label = (id: string) =>
    graph.nodes.find((item) => item.id === id)?.label ?? id;
  const dependencies = (value: Dependencies) =>
    [
      ...value.local.map((key) => `本 Roadmap ${key}`),
      ...value.formal.map((id) => `正式 #${id}`),
    ].join("、") || "无";
  const nodes: PreviewNode[] = graph.nodes
    .filter((item) => !item.originalOnly)
    .map((item) => ({
      id: item.id,
      type: "review",
      position: item.position,
      data: item,
      selected: item.id === selected,
      ariaLabel: `${item.formalID ? "已有正式任务" : "草稿任务"} ${item.label} ${item.title}`,
      className:
        selected && !neighbors.has(item.id) && selected !== item.id
          ? "review-dimmed"
          : undefined,
    }));
  const edges = visible.map((edge) => ({
    ...edge,
    type: "smoothstep",
    markerEnd: { type: MarkerType.ArrowClosed },
    className: `review-edge${selected && edge.source !== selected && edge.target !== selected ? " review-dimmed" : ""}`,
    ariaLabel: `${label(edge.source)} → ${label(edge.target)} · ${changeLabels[edge.change]}`,
  }));
  return (
    <div className="roadmaps-review">
      <p className="roadmaps-review-summary">
        {analysis.tasks.length} 项规划（新增{" "}
        {analysis.tasks.filter((item) => !item.existing_task_id).length} ·
        关联已有{" "}
        {analysis.tasks.filter((item) => !!item.existing_task_id).length}）
        {" · "}
        {graph.proposedCount} 条拟议依赖 · 新增{" "}
        {changes.filter((edge) => edge.change === "added").length} / 移除{" "}
        {changes.filter((edge) => edge.change === "removed").length} · 待澄清{" "}
        {analysis.questions.length}
      </p>
      <p className="roadmaps-review-legend">
        前置 → 后继 · 仅显示本次规划的建议直接依赖
      </p>
      <div className="roadmaps-review-layout">
        <div className="roadmaps-review-canvas" aria-label="依赖预览图">
          <ReactFlow
            nodes={nodes}
            edges={edges}
            nodeTypes={nodeTypes}
            fitView
            fitViewOptions={{ padding: 0.08, maxZoom: 1 }}
            minZoom={0.1}
            maxZoom={1.5}
            nodesDraggable={false}
            nodesConnectable={false}
            onNodeClick={(_, item) => setSelected(item.id)}
            onEdgeClick={(_, edge) => setSelected(edge.target)}
            onPaneClick={() => setSelected("")}
          >
            <Controls showInteractive={false} />
          </ReactFlow>
        </div>
        <section
          className="roadmaps-review-detail"
          aria-label="所选任务详情"
          aria-live="polite"
        >
          <h3>{node ? `${node.label} · ${node.title}` : "任务详情"}</h3>
          {!node && <p>选择图中节点或下方任务，查看范围、依赖与原文依据。</p>}
          {node?.formalID && (
            <p>
              <Link to={dag(node.formalID)}>
                正式 #{node.formalID} · {node.title}
              </Link>
            </p>
          )}
          {node && !task && <p>已有任务的解析快照；实时信息请查看正式任务。</p>}
          {task && (
            <>
              <p>
                {task.key_origin === "markdown" ? "沿用原代号" : "建议临时代号"}
              </p>
              <dl>
                {task.title !== node.title && (
                  <>
                    <dt>规划标题</dt>
                    <dd>{task.title}</dd>
                  </>
                )}
                <dt>目标</dt>
                <dd>{task.goal}</dd>
                <dt>范围</dt>
                <dd>{task.scope}</dd>
                <dt>约束</dt>
                <dd>{task.constraints || "未另列约束"}</dd>
                <dt>新增测试提供</dt>
                <dd>{task.provides_tests?.join("、") || "无新增"}</dd>
                <dt>通过条件</dt>
                <dd>{task.acceptance}</dd>
                <dt>草稿原有依赖</dt>
                <dd>{dependencies(task.original_dependencies)}</dd>
                <dt>拟议依赖</dt>
                <dd>{dependencies(task.proposed_dependencies)}</dd>
              </dl>
              <h4>原文依据</h4>
              <blockquote>{task.evidence}</blockquote>
              {task.reason && (
                <>
                  <h4>核对理由</h4>
                  <p>{task.reason}</p>
                </>
              )}
            </>
          )}
          {node && (
            <ul aria-label="所选任务直接依赖">
              {graph.edges
                .filter((edge) => edge.target === node.id)
                .map((edge) => (
                  <li key={edge.id}>
                    {label(edge.source)} → {node.label} ·{" "}
                    {changeLabels[edge.change]}
                  </li>
                ))}
            </ul>
          )}
        </section>
      </div>
      <section aria-label="依赖变更" className="roadmaps-review-changes">
        <h3>依赖变更</h3>
        {!changes.length && <p>与草稿依赖一致</p>}
        {graph.nodes
          .filter((item) => changes.some((edge) => edge.target === item.id))
          .map((item) => (
            <div key={item.id}>
              <button onClick={() => setSelected(item.id)}>
                查看变更 · {item.label}
              </button>
              <ul>
                {changes
                  .filter((edge) => edge.target === item.id)
                  .map((edge) => (
                    <li key={edge.id}>
                      {changeLabels[edge.change]}：{label(edge.source)} →{" "}
                      {item.label}
                    </li>
                  ))}
              </ul>
              <p>{item.task?.reason}</p>
            </div>
          ))}
      </section>
      <details className="roadmaps-review-tasks" open>
        <summary>
          完整任务清单（{analysis.tasks.length} 项规划，
          {graph.nodes.filter((item) => !item.task).length} 项正式引用）
        </summary>
        <ul>
          {graph.nodes.map((item) => (
            <li key={item.id}>
              <button
                aria-pressed={selected === item.id}
                onClick={() => setSelected(item.id)}
              >
                查看 {item.label} · {item.title}
              </button>
              <span>
                {item.task
                  ? `拟议前置：${dependencies(item.task.proposed_dependencies)}`
                  : `已有正式任务${item.originalOnly ? " · 仅原始引用" : " · 直接引用"}`}
              </span>
              {item.formalID && (
                <Link to={dag(item.formalID)}>
                  正式 #{item.formalID} · {item.title}
                </Link>
              )}
            </li>
          ))}
        </ul>
      </details>
    </div>
  );
}
