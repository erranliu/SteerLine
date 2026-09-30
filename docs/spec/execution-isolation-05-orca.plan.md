# Plan Orca 执行适配器迁移

任务 ID：58；任务名称：Orca 执行适配器迁移；目标：`EXECUTION-ISOLATION`。状态：待实施。唯一执行依据：[Spec](execution-isolation-05-orca.spec.md)。硬前置：#57 合入，一个实现 PR 交付。

## 迁移步骤

1. 接收固定启动命令和回执契约，列出 native/supervised 每类资源、意图、原生 ID、消息消费与中心提交点。
2. 以 orca_worker_prepare/dispatch/observation/evidence 为现有入口，在 orcaworker 迁配置、CLI 操作和业务事实转换，复用 event/message_identity/saved_receipt。依次接通工作区/终端、coordinator/Run、Task/Dispatch、输出/question/done、relay 和结算。资源创建回执分步提交，不把 prepare 变成不透明包内事务。
3. 替换 Orca 内的 Codex 参数生成，使用中心注入的固定命令。保留 shell 引号与注入策略；同步固定命令 fixture，证明搬迁前后实际 argv 相同。
4. 接通共同观察/反馈/恢复契约，中心仍负责派发授权和业务事务。迁移 native incarnation 和 supervised target 校验、未知 request 对账及精确 cleanup；PR 线索调用公共 helper，不导入 Codex 包。删除原平台接线和重复操作，保留持久化之后 ack 的事务顺序。
5. 用原消息和 CLI fixture 核验已保存输入、迟到消息、relay、确认顺序与保留资源，不启用 supervised 或重建未知 terminal。

## 验证

```powershell
go test ./internal/steerline/orcaworker -count=1
go list -deps ./internal/steerline/orcaworker
go test ./internal/steerline -count=1 -run '^(TestNativeOrcaFixedLunaLaunchReconcilesOriginalTerminal|TestNativeOrcaRejectsLateDispatchMessages|TestFixedOrcaAgentTerminalRejectsReplacedIncarnation|TestSupervisedTurnEvidenceRetainsQuestionAndEarlierPR|TestUnsupportedOrcaWorkerFlowConfigurationCannotClaim)$'
```

OC1 用依赖与调用审查；OC2 用原资源与旧消息；OC3 使用原不确定注入/relay 恢复 fixture 与包内确认顺序测试；OC4 用 supervised fixture 和不可领取断言。迁移后名称改变记录映射，不把未执行或 skip 计为通过。默认不启动真实 Orca 任务或全量测试。

## 交付

列明三类身份（业务、适配器、平台）、原生事实消费者及剩余中心事务接线。#59/#60 使用完整接口开展项目策略；不把本轮必须删除的重复命令或解析推迟到 #61。
