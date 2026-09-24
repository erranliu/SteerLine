# Spec：任务关系视图与只读导出

任务：`DAG-VIEW`。状态：待实施。配套 [Plan](dag-view.plan.md)。本文件只定义该任务，由一个独立实现 PR 交付。

## V1：范围与前置

前置：[Beads 退出 Spec](beads-exit.spec.md) 已验收并合入，底层 [固定 DAG Spec](task-dag.spec.md) D4 关系读模型与 [控制台 Spec](pr-04-console.spec.md) 查询边界均已交付。引用 [原生任务共同约束](native-task-constraints.md) 全部条款及 [原生任务模型](task-core.spec.md) C3 的查询版本约定。

本任务交付图查询筛选、TUI 上下游/阻塞详情、DOT/JSON 和可选 SVG 导出。只消费前置任务权威，不实现任务发布、迁移或状态转换；不增加拆分、回退、拖线编辑、Web 服务或交互式 Web 图。图展示失败不能影响调度。

## V2：查询、导出与布局

图由 QueryService 返回 native-task-graph/v1，包含 [原生任务模型 Spec C3](task-core.spec.md) 的采样元数据、nodes、edges、boundary_nodes、truncated 和过滤条件。节点至少展示稳定 ID、标题、状态、dependency_ready 与 blocker；边统一前置 → 后继。交付来源和规格/运行定位可在详情查询，不把所有日志装入节点。

`task graph` 支持 project、status 和以 task_id 为中心的 upstream/downstream/both、depth（默认 1、1–10）。默认最多 500 个完整节点，显式上限最大 1000；边只来自同一采样快照。每个显示节点的未显示前置/后继必须有边界 ID 或明确计数，选中详情可分页追踪；truncated 不得被标成完整图。

`task export -format json|dot|svg -output <path>` 接受同样筛选。JSON 是查询快照格式，不是可直接再次发布的 task-batch/v1；不把已发布 task_id 导出后用于重复建图。无筛选的完整导出超过上限返回 export_limit_exceeded，不悄悄裁剪。带筛选的导出保留边界与采样信息。

DOT 为必需输出；SVG 通过可选 Graphviz `dot` 生成。渲染工具缺失返回 renderer_unavailable，不影响发布、领取、TUI 或 DOT/JSON。可配置原生可执行文件路径，通过固定参数数组和 stdin 传 DOT；默认超时 30 秒、最大 SVG 20 MiB，超限/失败保留原输出文件，不影响业务状态。输出写到同目录临时文件后原子发布；已有文件无显式 overwrite 参数时拒绝。

节点标题、ID 等按纯文本转义，不解释为 DOT 语法、HTML 标签、终端控制序列或外部图片/脚本 URL。SVG 不嵌入任务提供的任意链接或资源；首版通过导出文件查看，不引入 Web 服务或浏览器内控制面。TUI 经中心查询展示前置、后继、阻塞原因及规格/交付定位，刷新保留 task_id 选择，不直接读 Beads。

依赖与就绪算法直接复用 [固定 DAG Spec D4](task-dag.spec.md)，本任务不重写。布局采用 [Graphviz dot](https://graphviz.org/docs/layouts/dot/)，渲染版本记入验证证据；渲染顺序不决定任务调度。

必须覆盖 100/500/1000 节点的链、分支汇合及不连通图，中文与长标题，以及超时、缺渲染器和超限；1000 节点的渲染须在上述预算内完成或返回明确失败，不能阻塞服务调度循环。交互式 Web 图、React Flow/Dagre/ELK.js 选型不属于本阶段。

## V3：命令与错误

`task graph` 与 `task export` 接受 -config 和 V2 筛选参数，业务只读；导出文件是本任务唯一输出副作用。错误至少含 renderer_unavailable、render_failed、export_limit_exceeded 与文件存在/写入失败；查询版本失效沿用前置 cursor_invalidated。

控制台只通过 QueryService 查看关系与执行事实。离线或静态导出明确显示采样时间/版本，不把布局位置或过期 ready 当作启动许可；所有任务变更仍走既有中心命令。

## 验收

| 编号 | 场景与通过条件 |
| --- | --- |
| NT18 | 同一读取快照图/列表一致；ID、方向、交付来源、边界、离线时间正确；过期游标不得拼接 |
| NT19 | 中文/长标题、100/500/1000 节点、布局失败/超限/缺工具可解释，服务调度不被渲染阻塞 |
| NT20 | 文本转义、无脚本/外链注入、输出不意外覆盖；UI/CLI 无拆分、回退或编辑已发布依赖入口 |
| NT21-VIEW | 既有任务列表、选择/阅读位置、watch、会话和离线显示回归通过；无直接 Beads 读取或业务写入，布局/缺工具不阻塞调度。 |

本任务的全部验收必须在一个独立实现 PR 内完成；配套 Plan 只负责本任务。证据不足不得标为完成。
