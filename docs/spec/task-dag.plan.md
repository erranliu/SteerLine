# Plan：固定依赖 DAG 调度

任务：`TASK-DAG`。唯一执行依据：[Spec](task-dag.spec.md)，共同要求见 [原生任务共同约束](native-task-constraints.md)。前置 [原生任务模型](task-core.spec.md) 已验收并合入；本任务只实现发布、依赖、领取和交付，不承担生产接入、迁移或展示。

## 基线确认

从最新主线核对前置的 Task/契约、权威 CAS、存储版本和 native-tasks/v1。确认公共类型归属、现有 start_task applied 门槛与 Operation/outbox；沿用一个中心写者，不新建独立 ready/claim 服务。测试仅使用隔离原生项目和替身，生产执行仍等待 BEADS-EXIT 验收。

## 实施步骤

1. 实现 task-batch/v1 严格解码和引用校验。固化 A → B → C、菱形汇合、断开组件、循环/悬空/重复/跨项目边和规格错误的样本；不引入父子节点、自动拆分或修图算法。
2. 接入内部 GraphAlgorithms 适配层。核验 dominikbraun/graph 的发布版本和必要 API，固定 go.mod/go.sum 与版本证据；封装环路径、稳定遍历/拓扑行为，不将库类型写入数据库或 DTO。不因第三方排序不稳定改变候选优先级。
3. 增加 ValidateTaskBatch 和 PublishTaskBatch，CLI 使用中心命令通道传入固定 payload。批次映射、任务、契约、边、事件、revision 和命令回执同事务提交；实现同 request_id 同 payload 复用及异 payload 冲突。
4. 实现同项目硬前置查询和稳定候选顺序。把 dependency_ready 与资源/暂停/能力阻塞分开，并返回具体前置。发布成功不主动启动 worker；明确 auto 模式下正常轮询可开始领取。
5. 将 ready/claim 准入接到既有 start_task 与自动派发路径，复用槽位与命令生命周期。固定主线 base、核验前置成果可达后，事务保存领取、运行和外部意图；提交后走原工作区/启动操作。不得把 claim 提交当作 start_task applied。
6. 增加中心原生交付事务：选用结果和固定门槛、精确 PR/merge 核验 → TaskDelivery/closed/WorkflowRun completed/事件提交。取消原生路径对 Beads close 的依赖；不要把完成写成后继状态批量更新。
7. 通过平台/worker 替身验证重试、迟到消息、领取竞争、提交前后中断、回执丢失和 unknown 守卫。控制端不得因测试简化而添加通用 task.status 更新入口。
8. 提供图查询的公共 DTO、database revision/graph_revision 和分页失效语义供后继 DAG-VIEW 使用；只固定业务查询，不在此阶段渲染图。更新中心状态/数据契约文档及 CLI 示例。

## 验证与完成

本任务验收：NT04–NT11、NT21-DAG。重点验证发布/领取/交付提交前后崩溃与响应丢失、相同请求幂等、并发领取、错误/迟到 PR 证据、前置 merge/base 可达、稳定排序和状态/投影一致性；先运行针对性测试，再按 [共同约束](native-task-constraints.md) 的验证原则运行完整 Go 测试、静态检查及差异检查。

证据保存在 `docs/validation/task-dag/`，逐项列出验收编号、输入摘要、工具/协议版本、命令、实际结果与证据位置。未运行项不得以 skipped/fake 记为完成。当前文档调整不代表实现验收通过。

本 Plan 的所有实施步骤构成同一个任务，由一个独立实现 PR 交付；不与前置或后继任务合并，也不把内部步骤各自当成独立交付 PR。完成自己的全部验收、兼容回归和文档更新后再交付，不代替后继任务验收。
