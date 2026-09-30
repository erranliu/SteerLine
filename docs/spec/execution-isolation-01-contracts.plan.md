# Plan 公共执行契约归包

任务 ID：54；任务名称：公共执行契约归包；目标：`EXECUTION-ISOLATION`。状态：待实施。唯一执行依据：[Spec](execution-isolation-01-contracts.spec.md)。无本系列未完成硬前置，一个实现 PR 交付。

## 迁移步骤

1. 枚举 `execution_backend.go`、`adapter_error.go`、`worker_evidence.go`、`codex_worker_evidence.go`、`orca_worker_evidence.go` 及其消费者，标注共同值、中心记录、平台证据。复用现有三个执行小接口、原生事件解析和附件事务，不复制已合入的架构收口。
2. 创建 `execution` 包并移入第一组完整类型。先处理身份、错误、能力、操作与工作区，再处理会话、问题、事件和必要接口。消除对 Run/Worker/父包的引用，保持原序列化含义。
3. 将中心转换留在主包，具体证据解释使用原唯一适配实现。移动通用 PRClues 到公共纯 helper，两个平台通过同一入口提取线索；固定启动命令和原生绑定交接，同步生产调用和 fixture。
4. 删除原定义及无用接口，迁移相关纯测试并建立旧/新用例映射。核验记录读取、来源关联和确认顺序未变。

## 验证

先运行新包纯测试和依赖检查，再跑中心少量接线。下列名称是现有回归入口，搬迁后按映射调整，确认每组实际执行：

```powershell
go test ./internal/steerline/execution -count=1
go list -deps ./internal/steerline/execution
go test ./internal/steerline -count=1 -run '^(TestExecutionBackendCommonCapabilityContract|TestExecutionRecordsPinBindingsAndOperations|TestOperationRecordRejectsCorruptionBeforeUse|TestWorkerProtocolFactsPreserveOriginalNativeIdentity|TestNativeOrcaFactsRejectOldDispatchAndKeepCurrentOutcome|TestWorkerFlowModeUsesFixedExecutionIdentity)$'
```

依赖审查对应 EC1；能力/绑定/损坏对应 EC2；两类事实对应 EC3。不启动真实 agent，不跑全量。补充测试仅限拆除中心/原生类型耦合后缺少的断言。

## 交付

记录类型迁移、消费者、未迁移实现与 #55/#56/#57/#58 接口。合入后 #55/#56 可分别开工；此 Plan 不承担后续权限配置或最终后端注册。
