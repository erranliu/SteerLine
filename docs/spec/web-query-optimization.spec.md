# Spec：SSE 前的 Web 查询与缓存优化

2026-10-15 用户确认后续将引入 SSE，要求先规划在 SSE 前值得实施、基本不增加复杂度的正向优化。本任务为普通开发，不分配 roadmap Task ID，不纳入执行 DAG；本次按用户后续明确授权实施。

[Plan](web-query-optimization.plan.md) · [专属验收](../acceptance/web-query-optimization.md) · [现有 Web 设计](../product-design/web-frontend.md)。

## 目标与当前问题

让页面切换、返回已访问内容和打开执行方案尽量复用已有数据，减少重复读取与无关刷新。保留现有 TanStack Query 作为唯一服务查询缓存，后续 SSE 继续消费同一套查询键和定向失效规则。

2026-10-15 rebase 后以 `47fafc12721ceb8fa39ba1ab227872fd14513bcb` 为调查基线，已包含独立 Web 与项目切换（PR #320）。调查时 `main.tsx` 默认 staleTime 为 1 秒；App 分别轮询浏览器会话、项目目录、当前项目 identity，Workspace 轮询 snapshot；DAG、任务详情、输出各自轮询。DAG 列表与执行页读取相同参数的 tasks 首屏，却使用 tasks / run-tasks 两套键。presentation 在详情中每 10 秒轮询。业务操作结束后失效同 home 的整个项目。DAG 列表显示时图查询仍运行，最终回复显示时 activity 查询仍运行。

## 范围与预期行为

### 1. 共用查询定义与缓存

- 在现有 API 层集中定义实际已有的 presentation、snapshot、tasks、task、graph、activity、final 查询键及请求参数；页面和预取直接复用 Query options，不再各写一份同义请求。
- 业务查询沿用现有 `[home_id, project_id, native_instance_id, ...资源与参数]` 前缀；同 home、项目、实例、资源及相同规范化参数使用同一查询键。tasks 首屏统一为 limit=100、空 cursor；不同页码/游标及 limit 不混用。执行页删除 run-tasks 独立缓存。
- 输出额外保留 Run/Execution/Attempt 身份；同号任务、重名项目以及同 Origin 下不同 home 不共享业务数据。不同资源的相似字段不构成可互换的完整数据。
- 保留 App 的既有键：会话 `["browser-session", home_id]`、目录 `[home_id, "projects"]`、项目身份 `[home_id, project_id, "identity"]`；本机请求日志保持 `[home_id, "local-requests"]`。这些查询不强行加入业务实例资源族，不改变现有身份核验顺序。
- 不增加第二个 store、实体规范化层或通用数据访问框架。fetch 的 no-store 保留；它与 Query 内存缓存职责不同。

### 2. 有界基础预加载

- App 通过现有会话、目录、当前项目身份核验并挂载对应 Workspace，该 Workspace 的原 online 条件成立后，后台预取 presentation 和 tasks 首屏。页面已发起的同键请求直接共享；预取不阻塞路由、首屏或当前页面请求。项目目录列出的其他项目不触发业务预取。
- 以当前 home/项目/原生实例的连接生命周期触发，不由目录、identity 或 snapshot 每次采样触发；重新连接或返回该项目可按缓存新鲜度再次预取，不增加独立定时器或调度器。
- 预取继续调用原 Workspace 的 `Api.get`，复用其 activate/deactivate 和 AbortSignal 生命周期。tasks/presentation 的在途共享读取不绑定页面观察者的 signal，避免页面切换取消后重复发起；其他查询仍消费 Query signal。切换项目或原生实例时 deactivate 取消传输并取消原作用域的 Query，停止旧预取。保留按原身份缓存，晚到结果不能改写新 Workspace，也不能触发向新项目预取或发送命令。
- presentation 使用 60 秒 staleTime，取消 10 秒轮询；访问时过期则后台刷新，用户手动刷新可立即更新。该时间约束的是复用新鲜度，不承诺无人访问时每 60 秒更新。
- 其余动态数据保留现有 1 秒 staleTime、可见区域的轮询周期及现有默认缓存回收策略，不统一延长状态新鲜度。
- 不递归拉取后续分页、全部任务详情、历史输出或其他项目。预取错误留给相应页面查询显示，不使整个工作区失效，不自动重试。

### 3. 已有内容先展示

- 命中同身份缓存时立即显示；后台刷新不清空内容、不改回首次加载态。首次无缓存的查询显示明确加载态，不把 pending 误写成“没有任务/没有回复”。
- 查询失败可保留最后观察内容并显示错误；缓存存在不能据此启用业务操作。
- 切换 Task/Run/Execution/Attempt 后不得以旧目标内容填充新目标。此轮不从列表拼造详情响应，不在不同身份间使用 keepPreviousData。
- 不修改布局体系，不增加全局 loading 状态或独立 UI 数据副本。

### 4. 隐藏内容不主动查询

- DAG 的 graph 仅在图视图可见时启用；tasks 仅在列表视图可见时作为页面观察查询启用。启动 tasks 首屏的一次有界预取属于上一节明确例外。
- activity 仅在活动标签可见时启用，final 仍仅在最终回复标签可见时启用。切回时先读缓存，再依现有新鲜度刷新/轮询。
- 列表视图不为工具栏节点计数继续轮询图；计数使用已观察图数据并明确其含义，或仅在图视图显示，实施优先选后者。
- App 会话、项目目录、当前项目 identity，Workspace snapshot、请求回执对账及浏览器后台行为维持现有规则，不新建可见性管理器，不按任务状态设计自适应轮询。

### 5. 操作后按资源族刷新

只用少量显式规则，不建设每字段依赖图。所有范围限于发送/对账入口捕获的原 home 和原命令的项目与原生实例；本机请求日志仅失效原 home 的键。不得使用查询键旧下标或当前 UI 选择推断命令归属。

| 操作 | 立即失效的服务查询 |
| --- | --- |
| set_dispatch_mode / pause_dispatch / resume_dispatch | snapshot、tasks、task、graph；派发改变可能影响所有任务启动资格 |
| start_task / start_candidate | snapshot、tasks、task、graph；本轮保守覆盖该实例全部任务详情，避免维护逐任务依赖 |
| answer_decision / stop_candidate | 上述动态概览与任务查询，加原候选的 activity/final；原候选身份可由原信封或原快照核对，不能使用当前 UI 选择猜测 |
| open_native_session | 无业务查询；只更新本机请求日志 |
| 未覆盖动作，或无法可靠定位受影响候选 | 原项目/实例全部动态查询；仍不刷新 presentation |

- 普通业务动作不再失效 presentation、其他项目/实例；候选定位明确时不刷新无关候选输出，无法定位时采用表中的保守范围。定向失效默认只立即重查活动观察查询，隐藏缓存标记过期、下次访问再查。
- mutation 完成和原请求对账观察到结果变化时复用上述规则；未知/拒绝/版本冲突时刷新有关观察，不推断业务成功。不改变提交、补交、对账及重试规则。当前原生实例读取旧实例回执时，业务失效仍使用信封中的旧实例；不得改写为当前实例或补交旧命令。
- 用户明确点击工作区手动刷新时，刷新同 home 会话、目录、当前项目 identity，以及当前原生实例的已有业务查询（包括 presentation）和同 home 请求日志。已有入口负责此调用，不另建会话管理层；不失效其他项目/实例，不预取其业务资源或所有隐藏资源。

## 与 SSE 的关系

SSE 是后续确定方向，本轮不实现连接、事件协议、序号、重连续读或断线补偿。后续 SSE 在核对 home/项目/原生实例和对象身份后，可直接失效上述资源键，或在事件提供完整且有版本约束的数据时更新同一 Query 缓存；页面不增加另一条数据路径。本轮保留的轮询届时再由 SSE 任务决定替换范围。

不为 SSE 预建事件总线、订阅适配器、通用同步引擎或“轮询/SSE”配置开关。

## 非目标与保留约束

- 不增加 IndexedDB 服务查询持久缓存、离线写队列、跨标签查询同步、Service Worker、ETag、聚合/增量 API、SQL/索引优化、前后端版本协议。
- 不做悬停全量预取、下一页递归预取、final 永久缓存或按“已结束”状态直接停止所有输出查询；现有 DTO 未提供充分的永久不变承诺。
- 不做业务状态乐观成功；不缓存或跳过回答提交前的 decision-eligibility 核验。不改变原信封、Web Locks、取消与身份/版本检查。
- 不修改 worker 流程、恢复和重试。在已实现的[项目切换契约](web-project-switch.spec.md)上实施，沿用 home/项目/原生实例隔离、深链接和 Workspace/Api 生命周期，不重建会话或项目管理，不恢复旧项目级 Web 路由/CLI。
- 不建长期性能监控系统或以未测量的毫秒承诺作为验收；以请求复用、隐藏查询停止和渲染时序作为可验证结果。

## 验收要求

自动项 WQ-A1–A6 与人工项 WQ-M1 逐项定义于[专属验收](../acceptance/web-query-optimization.md)。自动验证覆盖同键复用、有界预取、缓存展示/隔离、可见查询、失效范围和真实浏览器接线。人工体验可后置；未实现、未运行的项目不得登记通过。
