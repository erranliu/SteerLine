# Plan Orca 执行适配器迁移

任务 ID：58；任务名称：Orca 执行适配器迁移；目标：`EXECUTION-ISOLATION`。状态：待实施。唯一执行依据：[Spec](execution-isolation-05-orca.spec.md)。硬前置：#57 合入，一个实现 PR 交付。

## 迁移步骤

1. 接收本机固定命令与公共回执，列出 native/远程资源和中心提交点。远程以已独立 orca_remote_worker 为唯一协议，不重新搬入本机包；删除的 shell/journal/turn 包不列迁移目标。
2. 以 orca_worker_prepare/dispatch/observation/evidence 为入口，复用 contracts 的 HostFact/探针/环境值和现有 event/message_identity/saved_receipt，迁 CLI 操作与事实转换。依次接资源、Task/Dispatch、消息/工具/relay 和结算；保留单一定义的 DTO 别名，资源回执分步提交，不把 prepare 变成不透明事务。
3. 本机 native 使用 #57 固定命令，保留 shell/注入与 argv；远程保留原生 worker-start 与 terminal 复用、显式 model/effort 和原值路径，不能替换为中心 argv。
4. 接通观察/反馈/恢复及准备事实，保留中心 WorkerEnvironment 和可信 HostTarget 的动作受理。平台继续解析输入/HostFact/工具/收据，中心保存 readiness/business、预算、DeferredDecision/Feedback 与重查并发前提。迁移原身份/未知请求/精确 cleanup，PR 线索走公共 helper，持久化后 ack。
5. 用当前 native 与远程 fixture 核验原输入/消息/relay/ack 和身份；远程 release/协调 terminal 关闭与任务交付分开，取消继续拒绝。不重建未知 terminal，不把已接通远程改回不可启动。

## 验证

```powershell
go test ./internal/steerline/orcaworker -count=1
go list -deps ./internal/steerline/orcaworker
go test ./internal/steerline -count=1 -run '^(TestNativeOrcaFixedLunaLaunchReconcilesOriginalTerminal|TestNativeOrcaRejectsLateDispatchMessages|TestFixedOrcaAgentTerminalRejectsReplacedIncarnation|TestRemoteOrcaTwoTurnsKeepSessionAndRequireMergedDelivery|TestRemoteOrcaQuestionLostReplyAndAckAreReconciled|TestRemoteOrcaRejectsReplacementSession|TestWorkerEnvironmentER1ProbeAndSettlementGate|TestWorkerEnvironmentER3RecheckKeepsOriginalDispatch|TestEnvironmentRecheckCannotClearNewFaultObservedDuringPoll)$'
```

OC1 用依赖/调用审查；OC2 用原资源/旧消息；OC3 复用环境/重查/relay；OC4 用当前 remote Service 的两轮、丢失回复/ack 与身份拒绝，受影响 start 对账另选现有用例，不复制矩阵。删旧 journal 测试映射到现行协议，不把缺省配置拒绝算成远程整体不可用。无行为变化不补双机认证。

## 交付

列明三类身份（业务、适配器、平台）、原生事实消费者及剩余中心事务接线。#59/#60 使用完整接口开展项目策略；不把本轮必须删除的重复命令或解析推迟到 #61。
