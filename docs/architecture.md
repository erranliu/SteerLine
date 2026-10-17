# 整体架构与职责边界

> 2026-10-15：TUI 已移除，Web 为唯一交互前端；后台 CLI、服务及原生会话保留。实现和验证范围见[TUI 退役](acceptance/tui-retirement.md)。

> 2026-10-17：已按当前代码重新核对运行边界、包依赖与资源归属；三张现状图及进一步解耦建议见[代码架构](architecture/current-code-architecture.md)。第一轮已将 Web 路由白名单收口到纯 HTTP 契约层、统一候选不存在的错误映射，并补上 Web/taskrunner 依赖守卫；执行状态机继续归独立 Runner。结果见[第一轮验收](acceptance/architecture-decoupling-round1.md)。

> 同日第二轮：历史与任务候选读取改为 SQLite 在原事务内返回候选事实，产品查询层在事务后生成历史/详情 DTO；SQLite 的六项查询层例外已全部移除。同快照读取与原子交付保持，范围与验证见[第二轮验收](acceptance/architecture-decoupling-round2.md)。

> 同日第三轮：会话、正文及活动读取也改为存储事实与产品投影分离。SQLite 和存储端口已无产品包生产依赖；活动记录流保持绑定与事件同一读事务，查询层按原公开 JSON 字节预算投影并关闭记录流。原身份、游标、正文和预算验证见[第三轮验收](acceptance/architecture-decoupling-round3.md)。

SteerLine 以 roadmap 组织任务 DAG，连接 worker 的原会话、人工反馈和真实 PR 交付。执行已迁到独立 SteerRunner；当前代码/运行边界见[代码架构](architecture/current-code-architecture.md)，实际后端与验收支持范围见[Runner 接入](integration/steerrunner.md)及专属验收。后续目标见[产品愿景](roadmap/product-vision.md)。

本文描述 SteerLine 产品运行的业务权威。普通开发由当前聊天直接实现；用户明确要求 roadmap 调度时，才由 roadmap-runner 使用[execution-map](roadmap/execution-map.md)管理已确认的任务与依赖。roadmap 流程见[RoadmapGuide](roadmap/RoadmapGuide.md)，不要求外部中心准入。

## 职责与权威

| 所有者 | 职责 |
| --- | --- |
| 人 | 决定目标与任务范围、处理产品取舍和权限问题、审阅并合并 PR |
| 任务应用 | 核验固定任务契约、DAG、领取资格与核验后的任务结果 |
| SteerLine 调用方（taskrunner / runnerbridge） | 冻结任务材料、保存候选与原 CLI 请求、对账公开 Run/事件/PR、转交授权输入、核验唯一交付及持久化停止义务 |
| 独立 SteerRunner | 准备工作区、管理普通/审计 workflow 与原会话、冻结逐轮输入/模型、核验执行 PR 事实并结算原生资源 |
| worker | 按任务 Spec/Plan 实现、测试、修复、提交和创建 PR；不写中心业务状态 |
| 执行后端 | 提供真实进程、终端、原生会话/轮次、平台消息和资源事实 |
| PR 平台 | 提供 PR 身份、head/base 和实际合并事实 |
| Web 与 HTTP 客户端 | 仅经 HTTP 查询产品 DTO、提交绑定原身份/版本的命令；保存自己的偏好、草稿和原请求，不读业务库或宿主业务文件 |
| HTTP 服务端与内部产品客户端 | 认证、协议映射、原请求登记/对账及应用调用；领域资格和状态转移仍由 Go 业务应用核验 |
| 应用与服务宿主 | 组装公开 Runner 客户端、端口和 Task 存储，承载进程、实例核验、控制传输、锁、心跳及退出观察 |
| SQLite 存储 | 保存唯一业务状态，核验身份、前值、版本和唯一约束后原子提交 |

Task、候选参与关系与 TaskDelivery 由 SteerLine 管理；Runner 的 Run、Execution、Attempt、问题/输入与执行 Command 分别表达执行事实。现有产品 DTO 与 checkpoint 仍保留部分 WorkflowRun、Decision、Operation 等命名，不表示 SteerLine 继续持有相应原生执行状态机。SteerLine 通过固定 Runner CLI 与执行后端协作；Runner 的进程、会话和仓库事实属于执行身份，不替代中心业务 Task 或 DAG。配置、服务 checkpoint、Runner 状态投影、UI 缓存和诊断日志都不能成为第二业务权威。

当前拓扑是一个共享 SteerLine 后端宿主和独立 Web 程序。`steerline start` 协调启动共享后端与 Web；后端装载配置项目，在同一进程内为每个项目建立隔离的运行时、Task actor、存储和 Runner CLI 客户端。每个项目的 SteerRunner 仍独立运行。Web 与 CLI 通过统一应用 API 使用项目用例；Web 不直接访问业务数据库、调度器或 Runner 私有实现。当前模块与进程边界见[代码架构](architecture/current-code-architecture.md)，启动、项目加载和关闭契约见[统一启动 Spec](spec/unified-startup.spec.md)。

## 执行与交付

领取时冻结任务契约、仓库/base、执行配置及原身份。SteerLine 在外部调用前保存候选与完整 CLI outbox，Runner 返回公开事实和回执；身份或结果不确定时沿原请求对账，不盲重发、不切换后端或派替代 worker。worker 在同一任务的原会话中处理实现和反馈；Runner 管理 development/1 或 development-audited/1 的步骤与审计修复预算；SteerLine 选择预设并按公开观察展示，不重建执行状态机。

不同任务可以人工并行，同一任务可人工比较多个独立候选。候选各有自己的会话、工作区、分支、PR 与反馈，Task 只接受一个核验过的交付。产品不提供单候选放弃操作；Task 丢弃与执行停止分别记录。

自然语言成功、轮次结束、进程退出、worker_done 或 PR 创建都不证明交付。人合并任一已绑定候选 PR 后，中心核验该候选 PR 身份与合并事实，再在同一事务记录唯一 TaskDelivery、关闭 Task 和释放后继；其他候选已合并或查询未知不阻塞该交付。closed 后停止该任务全部候选的 PR 状态查询，重启保持。数据与状态细节见[数据模型](product-design/data-model.md)。

## 生命周期、反馈与恢复

工作区准备、代理启动/初始化、输入接受/开执行、结果结算及收尾的节点异常和重试规则统一归[Worker 流程异常处理与重试](product-design/worker-lifecycle.md)。具体适配器提供原生事实与能力，Runner 的单写服务决定并提交执行动作；SteerLine 单独提交调用方事实和 TaskDelivery。文档中的待补设计不代表当前实现已经支持。

关闭 Web 只结束客户端观察，不停止共享后端或 worker。统一宿主停机时逐项目核实关闭结果、Runner 资源结算和锁释放；共享后端进程退出本身不证明原生 worker 已停止。停止、清理和释放按精确资源身份、实际后端能力及原 Operation 回执判断，未知结果继续保留诊断。统一启动及停机契约见[Spec](spec/unified-startup.spec.md)，当前操作入口见[Web 指南](../web/README.md)。

反馈分别保存问题、答案、发送意图、平台回执和原生轮次事实，不能把“已保存”或“已接受”显示为业务要求已落实。宿主登录/权限阻断与 worker 业务问题分开，未知动作只对账。服务重启先恢复观察和原请求；继续输入或重建 worker 按当前恢复规则取得人工授权，未验证的路径保持不可用。细节见[决策与反馈](product-design/decisions.md)。

未发布开发版本默认不兼容旧配置、数据库或协议；不自动迁移、双写、回退或清空原现场。不支持格式在副作用前拒绝，保留数据和原外部绑定。同构建当前格式重启与跨构建升级分别判断，未来版本管理不能从已有恢复证据推定。

## 文档与实现

技术架构的主要工程目标是提高 AI 开发的有效并行吞吐，使功能能按明确契约独立理解、实现、验证和交付，减少共享修改、等待与返工。上述业务权威、权限、恢复和唯一交付边界保持。每次规划先读[RoadmapGuide](roadmap/RoadmapGuide.md)，再用[并行开发指南](architecture/parallel-development.md)核对功能所有者、共享冲突及开工/集成条件；模块多或任务多本身不代表有效并行。

本文维护项目整体职责和原则。具体配置、数据、反馈、会话和 UI 设计归[产品设计](product-design/README.md)；代码文件、当前模块与接口归[源码索引](architecture/current-code-architecture.md)。产品管理的任务执行要求以独立 Spec/Plan 和中心固定契约为准，产品说明不直接改变派发资格。测试方法见[测试指南](testing.md)。

## Worker 流程与调用方交付

当前工作流归独立 Runner，普通 development/1 与审计 development-audited/1 分别选择。Runner 管理实现、报告、审计、修复与发布；SteerLine 在候选事务冻结原预设和提交，消费公开观察，核验合并后提交唯一 TaskDelivery。workflow 版本、CLI 协议和 Task 的 worker-flow-v2 是不同版本轴。冻结边界见[协作 Spec](spec/runner-collaboration.spec.md)，迁出前 development/v3 的[流程契约](spec/worker-pr-flow.spec.md)保留历史追溯。

SteerLine 在交付前经 Runner CLI 刷新所选 PR，在自己的同一事务中核验原绑定、Task/图/候选版本和合并证据，记录唯一 TaskDelivery、关闭 Task、释放后继及保存候选停止义务。Runner 步骤完成或公开状态达到 `pr_ready` 不替代该事务；正确已合并结果也不要求先经过 `pr_ready`。
