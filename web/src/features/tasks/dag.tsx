import { useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import {
  ReactFlow,
  Background,
  Controls,
  Handle,
  Position,
  type NodeProps,
  type Node,
  type Edge,
} from "@xyflow/react";
import { GitBranch, List, Network } from "lucide-react";
import { useWorkspace } from "../../app/context";
import { Button } from "../../shared/ui/button";
import { Status } from "../../shared/ui/status";
import type { GraphPage, TaskPage } from "../../shared/api/types";
import { TaskDetail } from "./task-detail";
import "@xyflow/react/dist/style.css";
type TaskNode = Node<
  { title: string; task: string; status: string; blocked: boolean },
  "task"
>;
function TaskCard({ data, selected }: NodeProps<TaskNode>) {
  return (
    <div className={"graph-task " + (selected ? "selected" : "")}>
      <Handle type="target" position={Position.Left} />
      <span className="eyebrow">TASK #{data.task}</span>
      <strong>{data.title}</strong>
      <Status value={data.status} />
      {data.blocked && <span className="muted">首次启动条件未满足</span>}
      <Handle type="source" position={Position.Right} />
    </div>
  );
}
const nodeTypes = { task: TaskCard };
export function layout(graph: GraphPage, selected: string) {
  const visible = new Set(graph.nodes.map((n) => n.task_id));
  const level = new Map<string, number>();
  for (let pass = 0; pass < graph.nodes.length; pass++) {
    let changed = false;
    for (const node of graph.nodes) {
      if (level.has(node.task_id)) continue;
      const parents = graph.edges
        .filter(
          (e) => e.successor === node.task_id && visible.has(e.predecessor),
        )
        .map((e) => e.predecessor);
      if (parents.every((p) => level.has(p))) {
        level.set(
          node.task_id,
          parents.length
            ? Math.max(...parents.map((p) => level.get(p)!)) + 1
            : 0,
        );
        changed = true;
      }
    }
    if (!changed) break;
  }
  const counts = new Map<number, number>();
  const nodes: TaskNode[] = graph.nodes.map((n) => {
    const depth = level.get(n.task_id) ?? 0;
    const index = counts.get(depth) ?? 0;
    counts.set(depth, index + 1);
    return {
      id: n.task_id,
      type: "task",
      position: { x: depth * 310, y: index * 170 },
      selected: n.task_id === selected,
      data: {
        title: n.title,
        task: n.task_id,
        status: n.status,
        blocked: !n.dependency_ready || !!n.startup_blockers?.length,
      },
    };
  });
  const edges: Edge[] = graph.edges
    .filter((e) => visible.has(e.predecessor) && visible.has(e.successor))
    .map((e) => ({
      id: e.predecessor + "-" + e.successor,
      source: e.predecessor,
      target: e.successor,
      type: "smoothstep",
    }));
  return { nodes, edges };
}
export function Dag() {
  const { api, online } = useWorkspace();
  const [params, setParams] = useSearchParams();
  const selected = params.get("task") ?? "";
  const [view, setView] = useState<"graph" | "list">("graph");
  const cursorKey = view === "graph" ? "graph_cursor" : "list_cursor";
  const cursor = params.get(cursorKey) ?? "";
  const graphCursor = params.get("graph_cursor") ?? "";
  const listCursor = params.get("list_cursor") ?? "";
  const graph = useQuery({
    queryKey: [
      api.identity.project_id,
      api.identity.instance_id,
      "graph",
      graphCursor,
    ],
    queryFn: ({ signal }) =>
      api.get<GraphPage>(
        "/graph?" + new URLSearchParams({ limit: "100", cursor: graphCursor }),
        signal,
      ),
    enabled: online,
    refetchInterval: 2500,
  });
  const tasks = useQuery({
    queryKey: [
      api.identity.project_id,
      api.identity.instance_id,
      "tasks",
      listCursor,
    ],
    queryFn: ({ signal }) =>
      api.get<TaskPage>(
        "/tasks?" + new URLSearchParams({ limit: "100", cursor: listCursor }),
        signal,
      ),
    enabled: online && view === "list",
    refetchInterval: 2500,
  });
  const nodes = useMemo(
    () =>
      graph.data ? layout(graph.data, selected) : { nodes: [], edges: [] },
    [graph.data, selected],
  );
  const select = (task: string) =>
    setParams((previous) => {
      previous.set("task", task);
      return previous;
    });
  const current = view === "graph" ? graph.data : tasks.data;
  const queryError = view === "graph" ? graph.error : tasks.error;
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">PROJECT / TASKS</div>
          <h1>任务 DAG</h1>
          <p>依赖、交付与执行资格，一处查看。</p>
        </div>
        <GitBranch className="heading-icon" size={30} />
      </div>
      <div className="toolbar">
        <div className="segmented">
          <Button
            variant={view === "graph" ? "default" : "ghost"}
            onClick={() => setView("graph")}
          >
            <Network size={15} />
            任务图
          </Button>
          <Button
            variant={view === "list" ? "default" : "ghost"}
            onClick={() => setView("list")}
          >
            <List size={15} />
            任务列表
          </Button>
        </div>
        <span className="muted">
          {graph.data?.nodes.length ?? 0} 个节点 · 图 v
          {graph.data?.graph_revision ?? "—"}
        </span>
      </div>
      <div className="dag-layout">
        <div className="graph-panel">
          {queryError && (
            <p className="notice amber">
              {String(queryError)}
              {cursor && (
                <Button
                  onClick={() =>
                    setParams((previous) => {
                      previous.delete(cursorKey);
                      return previous;
                    })
                  }
                >
                  刷新第一页
                </Button>
              )}
            </p>
          )}
          {view === "graph" && graph.data?.boundary.truncated && (
            <p className="notice amber">
              有界图 · 未显示 {graph.data.boundary.omitted_node_count} 个节点 ·{" "}
              {graph.data.boundary.reasons?.join("；")}
            </p>
          )}
          {view === "graph" ? (
            nodes.nodes.length ? (
              <div className="graph-canvas">
                <ReactFlow
                  nodes={nodes.nodes}
                  edges={nodes.edges}
                  nodeTypes={nodeTypes}
                  fitView
                  minZoom={0.3}
                  maxZoom={1.5}
                  nodesDraggable={false}
                  nodesConnectable={false}
                  onNodeClick={(_, node) => select(node.id)}
                >
                  <Background color="#314039" gap={22} />
                  <Controls showInteractive={false} />
                </ReactFlow>
              </div>
            ) : (
              <div className="empty">
                {graph.isPending ? "正在读取正式任务…" : "还没有正式任务"}
              </div>
            )
          ) : (
            <div className="task-table">
              {tasks.error && (
                <p className="notice amber">{String(tasks.error)}</p>
              )}
              {tasks.data?.tasks.map((task) => (
                <button
                  key={task.task_id}
                  className={
                    "task-row " + (selected === task.task_id ? "active" : "")
                  }
                  onClick={() => select(task.task_id)}
                >
                  <span className="task-number">#{task.task_id}</span>
                  <strong>{task.title}</strong>
                  <Status value={task.status} />
                </button>
              ))}
            </div>
          )}
          {current?.has_more && (
            <div className="pagination">
              <span>当前为有界快照页</span>
              <Button
                onClick={() =>
                  setParams((previous) => {
                    previous.set(cursorKey, current!.next_cursor!);
                    return previous;
                  })
                }
              >
                下一页
              </Button>
            </div>
          )}
          {cursor && (
            <Button
              onClick={() =>
                setParams((previous) => {
                  previous.delete(cursorKey);
                  return previous;
                })
              }
            >
              返回第一页
            </Button>
          )}
        </div>
        <aside className="detail-panel">
          {selected ? (
            <TaskDetail key={selected} id={selected} />
          ) : (
            <div className="empty">
              <GitBranch size={26} />
              <p>选择一个任务</p>
              <span>查看固定规格、依赖与交付</span>
            </div>
          )}
        </aside>
      </div>
    </>
  );
}
