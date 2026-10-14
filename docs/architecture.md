# 整体架构与职责边界

> 2026-10-14 核对：[PR299](https://github.com/erranliu/SteerLine/pull/299) 已合入，任务 [#91](spec/tui-http-client.spec.md)保留 TUI 并改为 HTTP 业务客户端，#44 WinUI 工程已删除。用户随后直接授权普通开发本机 Web 基座、DAG 与执行页，独立工程见 [web/](../web/README.md)，结果见[专属验收](acceptance/web-frontend.md)。SSE、规划 Agent 与远程浏览器部署仍未实现；#91 原验收保持其[对应范围](acceptance/tui-http-client.md)。

SteerLine 以 roadmap 组织任务 DAG，连接 worker 的原会话、人工反馈和真实 PR 交付。一期包含本地 Codex、本机 Orca 与远端路径交付。后续目标见[产品愿景](roadmap/product-vision.md)，源码归属见[代码架构](architecture/current-code-architecture.md)。

本文描述 SteerLine 产品运行的业务权威。普通开发由当前聊天直接实现；用户明确要求 roadmap 调度时，才由 roadmap-runner 使用[execution-map](roadmap/execution-map.md)管理已确认的任务与依赖。roadmap 流程见[RoadmapGuide](roadmap/RoadmapGuide.md)，不要求外部中心准入。

## 职责与权威

| 所有者 | 职责 |
| --- | --- |
| 人 | 决定目标与任务范围、处理产品取舍和权限问题、审阅并合并 PR |
| 任务应用 | 核验固定任务契约、DAG、领取资格与核验后的任务结果 |
| 执行应用 | 准备工作区、管理固定流程的步骤/status、派发并观察原会话、传递反馈、核验交付和结算资源 |
| worker | 按任务 Spec/Plan 实现、测试、修复、提交和创建 PR；不写中心业务状态 |
| 执行后端 | 提供真实进程、终端、原生会话/轮次、平台消息和资源事实 |
| PR 平台 | 提供 PR 身份、head/base 和实际合并事实 |
| TUI 与 HTTP 客户端 | 仅经 HTTP 查询产品 DTO、提交绑定原身份/版本的命令；保存自己的偏好、草稿和原请求，不读业务库或宿主业务文件 |
| HTTP 服务端与内部产品客户端 | 认证、协议映射、原请求登记/对账及应用调用；领域资格和状态转移仍由 Go 业务应用核验 |
| 应用与服务宿主 | 组装 adapter、端口和存储，承载进程、实例核验、控制传输、锁、心跳及退出观察 |
| SQLite 存储 | 保存唯一业务状态，核验身份、前值、版本和唯一约束后原子提交 |

Task、WorkflowRun、Execution、Attempt、Decision、Command、Operation 和 TaskDelivery 分别表达不同事实。当前 Orca 单终端路径不创建 Orca Run/Task/Dispatch；原 runtime、terminal/incarnation、worktree 和 provider session 属于执行身份，不替代中心业务 Task 或 DAG。配置、服务 checkpoint、Runtime 投影、UI 缓存和诊断日志都不能成为第二业务权威。

当前业务消费链为 `TUI → httpclient/httpcontract → HTTP → httpapi → product/client → Go 任务/执行应用与只读查询`。`consoleapp` 独立组装 TUI 连接；`app` 组装服务、SQLite 和具体执行 adapter。CLI 的内部 Go 产品客户端仍是另一种调用入口，不能据此声称全部 CLI 已经 HTTP 化。现有解耦程度及后续建议见[源码边界评估](architecture/current-code-architecture.md#http-接入后的解耦评估与建议)。

本机 Web 消费链为 `React SPA → 同源浏览器会话/CSRF → web/server → 原 httpclient → native HTTP API`。浏览器入口使用项目配置 `web_address` 的稳定 loopback 地址，原 native endpoint 仍为随机端口、当前用户 Bearer 和无 Origin。两个入口在同一 Go 服务内启动/关闭；Web 不组装业务库或调度器。私有打开凭证放应用 home 的 `ui/web/<project_id>/`，CLI 与原 native discovery 核对实例后签发短票据。客户端 IndexedDB 只保存原信封、回执观察和输入草稿。契约与非目标见[Web Spec](spec/web-frontend.spec.md)。

## 执行与交付

领取时冻结任务契约、仓库/base、执行配置及原身份。中心在外部调用前保存意图，后端返回现场事实和回执；身份或结果不确定时沿原请求对账，不盲重发、不切换后端或派替代 worker。worker 在同一任务的原会话中处理实现和反馈；中心管理下文 development/v3 的固定步骤，测试、审查和修复要求仍由任务 Spec/Plan 决定，不增加自动修复轮数或通用编排。

不同任务可以人工并行，同一任务可人工比较多个独立候选。候选各有自己的会话、工作区、分支、PR 与反馈，Task 只接受一个核验过的交付。产品不提供单候选放弃操作；Task 丢弃与执行停止分别记录。

自然语言成功、轮次结束、进程退出、worker_done 或 PR 创建都不证明交付。人合并任一已绑定候选 PR 后，中心核验该候选 PR 身份与合并事实，再在同一事务记录唯一 TaskDelivery、关闭 Task 和释放后继；其他候选已合并或查询未知不阻塞该交付。closed 后停止该任务全部候选的 PR 状态查询，重启保持。数据与状态细节见[数据模型](product-design/data-model.md)。

## 生命周期、反馈与恢复

关闭控制台或 UI 只结束客户端观察；停止中心服务不证明外部 worker 已退出。停止、清理和释放按精确资源身份、实际后端能力及原 Operation 回执判断。

Windows 中心由用户从安装入口打开的可见服务窗口承载，可最小化；独立控制台不代启后台服务。离线入口显示 window-required 和 service-window.bat 的实际路径。服务窗口关闭结束中心进程；显式停服须同时核对原请求的 applied 回执、原进程退出及仓库/状态锁释放，未知时继续保留诊断，不伪造停止成功。Linux detached 路径按其平台能力展示，不视为 Windows 原生验收证明。当前操作见[执行控制台](product-design/execution-console.md)。

反馈分别保存问题、答案、发送意图、平台回执和原生轮次事实，不能把“已保存”或“已接受”显示为业务要求已落实。宿主登录/权限阻断与 worker 业务问题分开，未知动作只对账。服务重启先恢复观察和原请求；继续输入或重建 worker 按当前恢复规则取得人工授权，未验证的路径保持不可用。细节见[决策与反馈](product-design/decisions.md)。

未发布开发版本默认不兼容旧配置、数据库或协议；不自动迁移、双写、回退或清空原现场。不支持格式在副作用前拒绝，保留数据和原外部绑定。同构建当前格式重启与跨构建升级分别判断，未来版本管理不能从已有恢复证据推定。

## 文档与实现

技术架构的主要工程目标是提高 AI 开发的有效并行吞吐，使功能能按明确契约独立理解、实现、验证和交付，减少共享修改、等待与返工。上述业务权威、权限、恢复和唯一交付边界保持。每次规划先读[RoadmapGuide](roadmap/RoadmapGuide.md)，再用[并行开发指南](architecture/parallel-development.md)核对功能所有者、共享冲突及开工/集成条件；模块多或任务多本身不代表有效并行。

本文维护项目整体职责和原则。具体配置、数据、反馈、会话和 UI 设计归[产品设计](product-design/README.md)；代码文件、当前模块与接口归[源码索引](architecture/current-code-architecture.md)。产品管理的任务执行要求以独立 Spec/Plan 和中心固定契约为准，产品说明不直接改变派发资格。测试方法见[测试指南](testing.md)。

## Worker 分步流程

当前开发流程 `development/v3` 在现有 worker 内用普通代码定义 `implement.execute → implement.report → publish → [publish.report] → await_merge → delivered`；它与配置中的 `worker-flow-v2` 工作流协议是不同版本轴。实现正文自由输出，报告与发布使用严格结果；合法发布结果直接推进；发布结果格式错误时只增加一次 publish.report，禁止重做发布动作，见[发布补报](spec/worker-publish-report.spec.md)。业务模型轮次共用原会话/Attempt。WorkflowRun 的 `status/flow_ref/step_id` 是唯一业务状态权威，Execution.Status 保留原生事实。结果、转移与下一输入意图同事务提交；adapter 只提供规范事实/能力，不解析业务 JSON 或选择下一步。当前流程不保存独立 artifact 快照，报告绑定原输入/结果，无法直接继续的报告恢复需回原会话核实。候选与 PR head 经真实事实核验，真实合并仍走唯一 TaskDelivery。源码规则见 [staged_flow.go](../internal/execution/staged_flow.go) 和 [model/staged_flow.go](../internal/execution/model/staged_flow.go)；历史分步契约及原始验收见 [Spec](spec/worker-staged-flow.spec.md)与[专属记录](acceptance/worker-staged-flow.md)。
