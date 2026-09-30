# Plan Orca 执行适配器迁移

任务 ID：58；任务名称：Orca 执行适配器迁移；目标：`EXECUTION-ISOLATION`。状态：待实施。唯一执行依据：[Spec](execution-isolation-05-orca.spec.md)。硬前置：#57 合入，一个实现 PR 交付。

## 迁移步骤

1. 接收固定启动命令和回执契约，列出 native/supervised 每类资源、意图、原生 ID、消息消费与中心提交点。
2. 以 orca_worker_prepare/dispatch/observation/evidence 为现有入口，在 orcaworker 迁配置、CLI 操作和业务事实转换，复用 event/message_identity/saved_receipt。依次接通工作区/终端、coordinator/Run、Task/Dispatch、输出/question/done、relay 和结算。资源创建回执分步提交，不把 prepare 变成不透明包内事务。
3. 替换 Orca 内的 Codex 参数生成，使用中心注入的固定命令。保留 shell 引号与注入策略；同步固定命令 fixture，证明搬迁前后实际 argv 相同。
4. 接通观察/反馈/恢复及准备事实，保留中心 WorkerEnvironment 和可信 HostTarget 的动作受理。平台继续解析输入/HostFact/工具/收据，中心保存 readiness/business、预算、DeferredDecision/Feedback 与重查并发前提。迁移原身份/未知请求/精确 cleanup，PR 线索走公共 helper，持久化后 ack。
5. 用原消息和 CLI fixture 核验已保存输入、迟到消息、relay、确认顺序与保留资源，不启用 supervised 或重建未知 terminal。

## 验证

```powershell
go test ./internal/steerline/orcaworker -count=1
go list -deps ./internal/steerline/orcaworker
go test ./internal/steerline -count=1 -run '^(TestNativeOrcaFixedLunaLaunchReconcilesOriginalTerminal|TestNativeOrcaRejectsLateDispatchMessages|TestFixedOrcaAgentTerminalRejectsReplacedIncarnation|TestSupervisedTurnEvidenceRetainsQuestionAndEarlierPR|TestUnsupportedOrcaWorkerFlowConfigurationCannotClaim|TestWorkerEnvironmentER1ProbeAndSettlementGate|TestWorkerEnvironmentER3RecheckKeepsOriginalDispatch|TestEnvironmentRecheckCannotClearNewFaultObservedDuringPoll)$'
```

OC1 用依赖与调用审查；OC2 用原资源与旧消息；OC3 复用 ER1/ER3/重查竞争、原 relay fixture 与包内现有收据/工具/确认测试，迁移而不复制矩阵；OC4 用 supervised fixture 和不可领取断言。名称改变记录映射，不把零命中或 skip 算通过。无启动/权限变化时复用已交付原生材料，不重跑全量或真实业务闭环。

## 交付

列明三类身份（业务、适配器、平台）、原生事实消费者及剩余中心事务接线。#59/#60 使用完整接口开展项目策略；不把本轮必须删除的重复命令或解析推迟到 #61。
