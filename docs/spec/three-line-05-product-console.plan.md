# Plan：产品查询与控制台契约收窄

任务 ID：77；任务名称：产品查询与控制台契约收窄。唯一执行依据：[Spec](three-line-05-product-console.spec.md)。硬前置：#74、#38。一个独立实现 PR 交付。

## 实施步骤

实现基于合入 #76（PR #219，merge `4288c43b05287554de0b542e9eeb90d366fe063f`）后的 main，分支起点 `a5fa7fc2`。按用户确认的实施顺序，先消费 #76 的实际执行 namespace 和接口，再迁真实消费者；不保留旧路径别名、转发或第二份 SQL。

任务 ID：77。Canonical Spec：`docs/spec/three-line-05-product-console.spec.md`；SHA-256：`75930b2a156ae9bdf71c78ffe4cf19135258738c127f43b4a63dba3cbd694b42`。实现 Plan 持续更新，不改变 Spec 范围。

1. [完成] 将页面字段映射到独立、可序列化的 `internal/product/api` 类型；移出候选、运行、恢复、决策和命令的内部记录与 Go 句柄。Go 错误 cause 留在客户端内部。验证：`product-02`。

2. [完成] 将唯一 Go 客户端迁到 `internal/product/client`，以窄的 Reader/Command/Control/Decision 注入端口组成。把产品只读查询和页面映射迁到 `internal/product/query`，图过滤/游标归 `taskgraph`。共享任务图快照归 `storage/ports`，让 SQLite、执行和产品查询共用同一采样值。验证：`product-02`、`sqlite-snapshot-01`。

3. [完成] 将 UI 迁至 `internal/console`，只消费产品 API。更新 app 工厂、CLI、runner 精确组与边界守卫。应用层继续委派执行/命令写者；项目概览继续使用同一个 `ReadProjectView` SQLite 事务。验证：`console-02`、`app-query-02`、`boundary-01`、`wiring-02`。

4. [完成] 保持 `docs/validation/three-line-05-product-console/ownership.md` 和 `test-mapping.json` 记录实际文件、接口、构造者、测试 parent/subtest、断言所有者和证据状态。验证：本目录报告。

## 测试计划

运行环境为 Windows；`cmd/test-feedback` 为每次运行创建隔离 `STEERLINE_HOME` 并保存原始 Go JSON、stderr、summary。所有命令不启用原生 opt-in。每行列出级别/组或场景 ID、环境、命令、时点、通过条件和证据目录；命令须至少命中一个目标测试，skip 或零匹配不算通过。

| 级别/组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 定向 TM05.1/TM05.3：API、client、query、taskgraph、storage ports | Windows；隔离 runner；无原生 opt-in | `go run ./cmd/test-feedback -out docs/validation/three-line-05-product-console/product-02 -- ./internal/product/api ./internal/product/client ./internal/product/query ./internal/product/query/taskgraph ./internal/storage/ports -count=1` | DTO、注入端口和投影迁移后 | 5 packages、33 test instances 通过；覆盖私有字段、Unicode、原请求、独立读取能力、规范活动身份和同一图快照 | `docs/validation/three-line-05-product-console/product-02/` |
| 定向 TM05.2/#38 CC 与 UX：完整 console | Windows；隔离 runner；无原生 opt-in | `go run ./cmd/test-feedback -out docs/validation/three-line-05-product-console/console-02 -- ./internal/console -count=1` | UI 迁移后 | 1 package、78 test instances 通过；保留输入、草稿、历史、焦点、阻塞、停止和本地/Orca fixture 行为 | `docs/validation/three-line-05-product-console/console-02/` |
| 定向 TM05.1/TM05.3：app 工厂与项目读取 | Windows；真实 app 适配、临时 SQLite | `go run ./cmd/test-feedback -out docs/validation/three-line-05-product-console/app-query-02 -- ./internal/steerlineapp -run '^(TestFileQuerySourceReadsCombinedProjectView\|TestCLIExecutionProfileIsBoundIntoStartTaskRequest)$' -count=1` | app/client 接线后 | 2 tests 通过；项目页面使用组合快照，冻结 profile digest 绑定原 start request | `docs/validation/three-line-05-product-console/app-query-02/` |
| TM05.3：SQLite 项目快照同事务 | Windows；临时 SQLite | `go run ./cmd/test-feedback -out docs/validation/three-line-05-product-console/sqlite-snapshot-01 -- ./internal/steerline/sqlite -run '^TestProjectView(RejectsNilReader\|SharesTransactionAcrossWriterCommit\|KeepsReadOnlySectionsWithoutCheckpointOrSelection)$' -count=1` | 读取接线后 | 3 tests 通过；writer commit 前后仍由同一 pinned transaction 采样 | `docs/validation/three-line-05-product-console/sqlite-snapshot-01/` |
| TM05 边界：全 OS/build-tag 源码、负向 fixture、Windows/Linux 依赖图 | Windows runner 计算两平台图；不把依赖扫描当跨系统运行 | `go run ./cmd/test-feedback -out docs/validation/three-line-05-product-console/boundary-01 -- ./internal/steerlineintegration -run '^TestCD67CoreBoundary$' -count=1` | 包迁移和守卫更新后 | 36 test instances 通过；无旧包、产品/执行反向边或平台依赖违规 | `docs/validation/three-line-05-product-console/boundary-01/` |
| TM05.1–TM05.3：真实候选控制台接线 | Windows；临时 Git/SQLite、受控服务与后端 | `go run ./cmd/test-feedback -out docs/validation/three-line-05-product-console/wiring-02 -- ./internal/steerlineintegration -run '^TestCandidateConsoleWiring$' -count=1` | 最终共享接口调整后 | 非零命中并通过；真实 app/client/SQLite/control 构造和候选命令保留 task/execution/session/revision/profile 身份 | `docs/validation/three-line-05-product-console/wiring-02/` |
| 固定最小入口 L1 | Windows；runner 隔离 home；无原生 opt-in | `go run ./cmd/test-feedback -level=L1 -out docs/validation/three-line-05-product-console/l1-02` | 更新精确包名、目录边界测试后执行 | 固定最小规则、构造、守卫组非零命中并通过；不启用 live/native 组 | `docs/validation/three-line-05-product-console/l1-02/` |

不选 Full、L2、L3 或 live：本次只调整 API/包所有权；完整 console 包、真实 SQLite 同事务、真实候选接线、两平台依赖闭包覆盖变更。#38 的终端行为由迁移后完整 console 测试承担。#39 原生环境无需重验，因为本次没有原生适配、策略或进程改动。跨平台只检查依赖图，不宣称运行通过。新增场景/环境能力前先更新 [E2E README](../../tests/e2e/README.md) 与本表。

## 自查与交付

逐项对照 Spec 检查 API 字段、提交屏障、同快照、身份/版本和唯一结果；确认迁出路径与无消费者桥接删除。检查所有目标命令命中、回执和测试 summary 后，汇总 [验证证据](../validation/three-line-05-product-console/README.md)、提交并创建一个实现 PR。
