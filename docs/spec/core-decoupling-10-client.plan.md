# Plan 产品客户端适配层与CLI接入

任务 ID：71；任务名称：产品客户端适配层与CLI接入；目标：`CORE-IMPLEMENTATION-DECOUPLING`。唯一执行依据：[Spec](core-decoupling-10-client.spec.md)及[共同约束](core-maintenance-constraints.md)。硬前置：#68 整体交付；一个实现 PR。

## 实施顺序

1. 盘点 CLI/TUI 产品动作、项目引用、连接/日志/查询/控制 DTO、当前默认源与资源所有权；划清外层安装/窗口入口和产品查询/命令。建立一对一消费者映射，复用 query/projection，不增加业务动作。
2. 提取唯一产品纯契约，明确身份、版本、游标、回执/错误分类与资源生命周期。应用组装注入配置解析、读取/命令工厂和连接探测端口，adapter 不依赖 SQL 实现或组装包。
3. 把默认客户端 I/O 归外层 adapter，接通 CLI 实际查询与变更入口。TUI 原默认源调用新 adapter 的必要薄映射，登记 #72 删除清单，不保留旧并行 I/O。
4. 核对稳定请求 ID、资格、离线只读、关闭语义及查询结果独立性，迁移相关测试和模块说明；与存储线只协调构造段，不调整其端口。

## 测试计划

环境：Windows、本地 Go/Git、临时项目 SQLite 与文件 control；不启动真实模型。实施前先登记以下选择，稳定后逐项执行；原始 Go JSON 日志保存到系统临时目录 `steerline-cm71-evidence`，quick 保存到其独立子目录。

| 级别 / 组 / 场景 | 命令 | 执行时点 | 通过条件 / 证据 |
| --- | --- | --- | --- |
| L1 / CM71 adapter | `go test -json ./internal/steerlineclient -count=1` | adapter 稳定后 | 注入、关闭、同配置身份、错误分类、原请求查询、异内容重用、离线只读代表断言均实际命中；`client.jsonl` |
| L1 / CM71 CLI 与组装 | `go test -json ./cmd/steerline ./internal/steerlineapp -run 'Test(NativeTaskBatchCLI\|CLIExecutionProfile\|WaitServiceCommand\|CM71\|FileQuerySource)' -count=1` | 真实消费者接通后 | 原 CLI batch/show/list/graph/export/discard、profile、命令查询与默认源合并快照断言保留；`consumers.jsonl` |
| L1 / 会话与日志 | `go test -json ./internal/steerline/query ./internal/steerline/console -count=1` | 迁移后 | 现有正文/raw 独立读取、UTF-8/字节预算、游标及旧 Attempt 隔离、TUI 映射均命中；`query-console.jsonl` |
| L1 / #67 守卫 | `go test -json ./internal/steerlineintegration -run '^TestCD67CoreBoundary$' -count=1` | 完整 diff 自查后 | 全标签源码及 Windows/Linux 依赖检查通过；`boundary.jsonl` |
| L1 / 固定 quick | `go run ./cmd/test-feedback -quick -out <证据目录>/quick` | 最终稳定后一次 | 所有指定入口非零命中且无失败/skip；`quick/summary.json` 与组日志 |
| 静态检查 | `git diff --check` | 提交前 | 无空白错误；交付记录 |

新增最小 adapter 资源与错误测试，不新增 E2E 环境能力。旧 `console` 默认源项目快照测试迁入组装层，旧活动日志断言随唯一实现迁到 adapter，保持断言一对一。测试日志逐项核对 run/pass 事件，零命中不算通过。未选 L2/L3 delivery/recovery/live：本次不改变执行、交付、恢复或原生权限映射；现有确定性 query/control/CLI 与 #67 守卫足以验证客户端职责，避免重复真实传输/认证矩阵。未选 full：按项目最小入口约定使用定向回归与一次 quick。

## 实施记录与 #72 交接

客户端工厂构造新增到 `internal/steerlineapp/client.go`；`console.go` 仅修改客户端注入段，服务/存储构造段保持原归属。quick 入口沿用既有入口，不改共享选择。#72 删除 `console/source.go` 的 ProjectRef/ConfigSource 与客户端薄映射、console 中的中心别名和配置视图；改为直接消费 clientcontracts 与注入小接口。本次不实施导航/渲染的完整迁移。

## 验证与交付

CM71-1 用契约 import 扫描、实际 CLI 调用及默认源唯一性证明；CM71-2 从现有 CLI/control/feedback 身份、版本、重复请求和丢回执用例选择必要代表；CM71-3 复用当前只读错误分类、会话正文/游标及连接关闭断言。

协议动作与旧入口按表逐项核对，包括变更 action 的目标、幂等键、版本和返回状态；不可用动作明确沿原限制拒绝。所有所选测试记录实际命中和旧到新映射，稳定后定向追加与 quick 一次运行，复用 #67 守卫，不运行 full 或 HTTP/原生认证。运行 git diff --check。

PR 附产品契约/默认实现/CLI 的所有者、资源关闭与错误语义、删除清单和 TUI 暂存桥交接清单。向 #72 交付已消费接口，不以空包或复制 Runtime 代替接通。规划阶段不改代码或数据库。

客户端候选输入检查已归 adapter，TUI 桥只映射；移除 console 的 raw normalizer 字段/接口与日志实现。原 `console` 的四个 `TestConsoleSource*`、`TestProjectSnapshotReuseRequiresSameLiveIdentityAndDatabaseVersion`、`TestNativeCandidateSnapshotKeepsDispatchOrderBeyondDetailPage` 同名迁入 `steerlineclient`；原 CLI `TestWaitServiceCommandResolvesIdentityAndKeepsPolling` 迁入 adapter，以注入 reader 验证相同原 ID、accepted、超时及错误传播；原 CLI profile 断言同名迁入 app。实际 CLI batch 用例追加控制文件请求/未知回执、异内容冲突、外项目回执拒绝与客户端关闭不改变服务身份的代表断言。产品依赖检查为 `TestCM71ProductDependencyBoundary`，随上述 app 定向命令的 `TestCM71` 选择命中。详细证据见 [验证记录](../validation/core-decoupling-10-client/README.md)。
