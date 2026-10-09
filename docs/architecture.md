# 整体架构与职责边界

SteerLine 以 roadmap 组织任务 DAG，连接 worker 的原会话、人工反馈和真实 PR 交付。当前运行与验收范围是本地 Codex 和本机 Orca；保留的远端适配和历史试验不表示当前支持。后续目标见[产品愿景](roadmap/product-vision.md)，源码归属见[代码架构](architecture/current-code-architecture.md)。

本文描述 SteerLine 产品运行的业务权威。仓库开发由 roadmap-runner 直接调度 Codex，以[execution-map](roadmap/execution-map.md)管理任务与依赖；开发流程见[RoadmapGuide](roadmap/RoadmapGuide.md)，不要求外部中心准入。

## 职责与权威

| 所有者 | 职责 |
| --- | --- |
| 人 | 决定目标与任务范围、处理产品取舍和权限问题、审阅并合并 PR |
| 任务应用 | 核验固定任务契约、DAG、领取资格与核验后的任务结果 |
| 执行应用 | 准备工作区、管理固定流程的步骤/status、派发并观察原会话、传递反馈、核验交付和结算资源 |
| worker | 按任务 Spec/Plan 实现、测试、修复、提交和创建 PR；不写中心业务状态 |
| 执行后端 | 提供真实进程、终端、原生会话/轮次、平台消息和资源事实 |
| PR 平台 | 提供 PR 身份、head/base 和实际合并事实 |
| 产品客户端与 UI | 通过 Query/Command 展示状态和提交用户命令，不直接写库或推进平台状态 |
| 应用与服务宿主 | 组装 adapter、端口和存储，承载进程、实例核验、控制传输、锁、心跳及退出观察 |
| SQLite 存储 | 保存唯一业务状态，核验身份、前值、版本和唯一约束后原子提交 |

Task、WorkflowRun、Execution、Attempt、Decision、Command、Operation 和 TaskDelivery 分别表达不同事实。Orca 的 Run/Task/Dispatch 是平台执行身份，不替代中心业务 Task 或 DAG。配置、服务 checkpoint、Runtime 投影、UI 缓存和诊断日志都不能成为第二业务权威。

## 执行与交付

领取时冻结任务契约、仓库/base、执行配置及原身份。中心在外部调用前保存意图，后端返回现场事实和回执；身份或结果不确定时沿原请求对账，不盲重发、不切换后端或派替代 worker。worker 在同一任务的原会话中处理实现和反馈，中心不管理 implement/verify/review/publish 五步或自动修复轮数。

不同任务可以人工并行，同一任务可人工比较多个独立候选。候选各有自己的会话、工作区、分支、PR 与反馈，Task 只接受一个核验过的交付；放弃候选不等于其外部资源已经停止。

自然语言成功、轮次结束、进程退出、worker_done 或 PR 创建都不证明交付。人合并后，中心重新查询匹配的 PR 与提交事实，再在同一事务记录唯一 TaskDelivery、关闭 Task 和释放后继。数据与状态细节见[数据模型](product-design/data-model.md)。

## 生命周期、反馈与恢复

关闭控制台或 UI 只结束客户端观察；停止中心服务不证明外部 worker 已退出。停止、清理和释放按精确资源身份、实际后端能力及原 Operation 回执判断。

Windows 中心由用户从安装入口打开的可见服务窗口承载，可最小化；独立控制台不代启后台服务。离线入口显示 window-required 和 service-window.bat 的实际路径。服务窗口关闭结束中心进程；显式停服须同时核对原请求的 applied 回执、原进程退出及仓库/状态锁释放，未知时继续保留诊断，不伪造停止成功。Linux detached 路径按其平台能力展示，不视为 Windows 原生验收证明。当前操作见[执行控制台](product-design/execution-console.md)。

反馈分别保存问题、答案、发送意图、平台回执和原生轮次事实，不能把“已保存”或“已接受”显示为业务要求已落实。宿主登录/权限阻断与 worker 业务问题分开，未知动作只对账。服务重启先恢复观察和原请求；继续输入或重建 worker 按当前恢复规则取得人工授权，未验证的路径保持不可用。细节见[决策与反馈](product-design/decisions.md)。

未发布开发版本默认不兼容旧配置、数据库或协议；不自动迁移、双写、回退或清空原现场。不支持格式在副作用前拒绝，保留数据和原外部绑定。同构建当前格式重启与跨构建升级分别判断，未来版本管理不能从已有恢复证据推定。

## 文档与实现

技术架构的主要工程目标是提高 AI 开发的有效并行吞吐，使功能能按明确契约独立理解、实现、验证和交付，减少共享修改、等待与返工。上述业务权威、权限、恢复和唯一交付边界保持。每次规划先读[RoadmapGuide](roadmap/RoadmapGuide.md)，再用[并行开发指南](architecture/parallel-development.md)核对功能所有者、共享冲突及开工/集成条件；模块多或任务多本身不代表有效并行。

本文维护项目整体职责和原则。具体配置、数据、反馈、会话和 UI 设计归[产品设计](product-design/README.md)；代码文件、当前模块与接口归[源码索引](architecture/current-code-architecture.md)。产品管理的任务执行要求以独立 Spec/Plan 和中心固定契约为准，产品说明不直接改变派发资格。测试方法见[测试指南](testing.md)。

## Worker 分步流程

当前开发流程 `development/v1` 在现有 worker 内用普通代码定义 `implement.execute → implement.report → publish → await_merge → delivered`。实现正文自由输出，报告与发布使用严格结果；正常三个业务模型轮次共用原会话/Attempt。WorkflowRun 的 `status/flow_ref/step_id` 是唯一业务状态权威，Execution.Status 保留原生事实。结果、转移与下一输入意图同事务提交；adapter 只提供规范事实/能力，不解析业务 JSON 或选择下一步。候选与 PR head 经真实事实核验，真实合并仍走唯一 TaskDelivery。完整契约见 [分步 Spec](spec/worker-staged-flow.spec.md)，原始验收见 [专属记录](acceptance/worker-staged-flow.md)。
