# Plan 公共执行契约归包

任务 ID：54；任务名称：公共执行契约归包；目标：`EXECUTION-ISOLATION`。状态：已实施，待合入。唯一执行依据：[Spec](execution-isolation-01-contracts.spec.md)。无本系列未完成硬前置，一个实现 PR 交付。

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

## 实施结果（2026-09-30）

已将共同契约迁入 `execution` 并更新中心、控制台及适配器消费者，删除原定义和无人消费的 StartReceipt；原生问题路由与业务派生判断留在中心桥。原六项回归名称未改变，持久记录/能力 fixture 仍在主包；新增公共包纯测试与必要来源断言，没有重复执行搬迁矩阵。复用项目 `-quick` 并补两项原子绑定/停止接线、一项监督路径结算和一个控制台答案编辑用例，全部通过。

最终命令、实际测试数量、类型所有者和 #55/#56/#57/#58 的剩余接线见[验证与交接记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/execution-isolation-01-contracts/README.md)。权限、发布责任、恢复和配置行为保持，未启动真实 agent、运行全量或修改中心 Task。

交付前整合主干 PR160（`c2c9f95`），保留环境就绪字段及检查，公共选项同步 Receiver/Action；追加两项宿主等待/冻结资源接线和准备成功子用例均通过。最终摘要已更新为整合后的结果。

质量审计修复 Codex wire 选项解释与 Orca 观察器清理边界，并整合 PR163/PR165（`a458ce7`）：共享持久记录直接使用唯一公共执行类型，保留最新 Query/SQLite 接线。相关包、事务内项目快照、控制台及更新后的最小入口验证通过；详情见[最终审计记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/execution-isolation-01-contracts/audit.md)。

后续整合 PR162（`930a4d6`），远端新后端引用唯一共有身份/目标/协议/错误分类，保留独立 Dispatch 生命周期及主干删除项。旧监督 exec journal 用例跟随删除，调用方句柄清理断言并入既有停止回执用例；最小回归与远端两轮/问题回执用例通过。详情见[冲突整合记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/execution-isolation-01-contracts/conflict-pr162.md)。

## 测试计划（2026-10-07 导航维护）

本次仅将已清理的证据导航改为固定 Git 快照，不改原实现契约或追认历史通过。最小范围为文档静态 `doc-links`、`task-identities`、`whitespace`：当前 Windows/Git/Python 3 环境，交付前执行 `python scripts/check_doc_links.py`、`python scripts/roadmap_task_id.py check`、`git diff --check`；通过条件为无缺失本地链接、任务身份一致及无空白错误。固定历史链接的目标另用 `git cat-file --batch-check` 核对存在；结果见[本次清理验证](../../../acceptance/history.md#测试计划与本次结果)。不选择 L1/L2/L3、Full 或原生模型组；导航变更不需要这些验证，不新增场景或环境能力。
