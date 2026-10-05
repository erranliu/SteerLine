# #78 实现归属与调用路径

基线为已 fetch 的 `origin/main`：`4288c43b05287554de0b542e9eeb90d366fe063f`（PR219 / #76 merge）。本报告记录此分支实际源码路径、唯一实现和共享构造所有者，不把尚未落地的目录目标写成主线既有事实。

## 实现与消费者

| #78 范围 | 新的唯一源码所有者 | 实际调用方/消费者 | 所有权说明 |
| --- | --- | --- | --- |
| Codex worker | `internal/execution/adapters/codex` | `steerlineapp.NewBuiltinDirectory`、`steerlineapp.RunSessionLauncher`、E2E evidence/native、execution lifecycle resolver | 原配置 codec key、参数、权限、session/process binding、恢复及 observation 实现与 parent assertions 原样迁入。 |
| Codex app-server 协议 | `internal/execution/adapters/protocol/codexapp` | Codex adapter、`internal/steerlineintegration` 的 app-server wire 验收 | Codex worker 与跨层测试是两个实际消费者；WebSocket/JSON-RPC codec 仅有此定义，无复制包。 |
| 本机 Orca worker | `internal/execution/adapters/orca` | `steerlineapp.NewBuiltinDirectory`、执行生命周期 resolver、E2E native adapter | 原 terminal/runtime/agent/dispatch identity、原请求回执与拒绝路径保持。 |
| 存量 Orca remote adapter | `internal/execution/adapters/orcaremote` | `steerlineapp.NewBuiltinDirectory`、执行生命周期 resolver、协议/config tests | 迁移既有实现与拒绝保护；不增加远端准入或实时能力。 |
| Git workspace | `internal/execution/workspace` | execution 用例、`internal/steerline` 宿主预检与客户端、Codex/Orca adapter、app config | 单一 Git/resource 实现；迁移原 test parents 与 HEAD/base/worktree 断言。 |
| PR publication | `internal/execution/publication` | execution 交付/清理、`internal/steerline` 预检、app config | 单一 provider 实现；保留原 PR 身份、merge receipt 和 unsupported provider 拒绝。 |
| OS process/files | `internal/execution/process` | `executionbackend.ManagedProcessHost`、`steerlineapp.NewProcessHost`、execution 的 process facts/cleanup、Codex adapter | #76 已迁入的只读 process facts 与原 processhost `Host`/OS effects 汇为同一个实现；旧 `internal/steerline/processhost` 源码删除。 |

基线 `internal/steerline/remoteprotocol` 只有 `testdata/verify-*.json` 四份数据，没有 Go 源、`go:embed`、测试引用或生产调用者。数据随 remote adapter 移到 `internal/execution/adapters/orcaremote/testdata/verification/`；没有把该历史 verifier 复活为活动代码，也没有把这些无消费者数据计作通过证据。

当前 adapter 生产 import 图按本分支 Go AST 实际记录于 [当前架构索引](../../architecture/current-code-architecture.md)：

- Codex adapter → `execution/backend`、必要 `execution/model`、`execution/workspace`、Codex 专属协议包及既有 conversation pure contracts。
- Orca adapter → `execution/backend`、必要 `execution/model`、`execution/workspace`、conversation pure contracts，以及既有 Codex 配置/命令/权限实现的唯一消费者。
- Orca remote adapter → `execution/backend` 与被复用的 Codex/Orca profile/protocol helper。

边界守卫把 `internal/execution` 用例根、`internal/task` 应用、`internal/storage`、SQL/SQLite、app、console 和产品 query/client 列为 adapter 禁止依赖；允许 backend/model、专属协议包、workspace/publication/process 资源小包和现存 conversation pure-contract 依赖闭包。该闭包到 `product/api` 标量与 `steerline/taskview` 纯投影只允许由旧 contracts 间接引入，adapter 源码不得直接导入它们。守卫同时扫描全部 OS/build tags 源码，并对 Windows/Linux 做传递依赖检查。各 adapter 不拥有 Task/Execution/Delivery 写端口，不直接提交业务状态。

## 共享构造与字段所有者

| 文件与真实接口/字段 | 单一修改所有者与结果 |
| --- | --- |
| `internal/steerlineapp/app.go`: `NewProcessHost(time.Duration) executionbackend.ManagedProcessHost`、`RunSessionLauncher(ctx, requestPath, host)` | #78。实际构造 `internal/execution/process.Host`，launcher 调用 `internal/execution/adapters/codex.RunSessionLauncher`。 |
| `internal/steerlineapp/directory.go`: `NewBuiltinDirectory`、三条 `ExecutionRegistration`、profile/question codec key、`Backend`/`Lifecycle` factory | #78。原 backend/protocol/agent key、冻结 codec key/version 与 selection/refusal 行为保持。 |
| `internal/steerlineapp/config.go`: Git/workspace/PR 调用 | #78 只改 import 所有者路径；配置字段、JSON、默认值、冻结与拒绝行为不变。 |
| `internal/execution/application_config_types.go`: `Config.ProcessHost` 与 `Config.PR` | 字段/type shape 不变；`PR` 仍是 `publication.Config`，只由真实资源 package 提供。 |
| `cmd/steerline/native_task_test.go`、`internal/steerlineintegration/execution_wiring_test.go`: CLI process/profile consumers 与 `TestTM06ResourceWiring` | #78。CLI test 的 Codex import 改到新 adapter；新增 app factory/resource parent 和 child helper。原 `TestTM04ExecutionWiring`、MC parents/subtests/断言仍归原文件 owners。 |
| `internal/steerlineapp/client_boundary_test.go`、`internal/steerlineintegration/core_decoupling_67_boundary_test.go` | #78 只拥有 adapter/resource 新路径反例与 import closure 对应路径；#77 拥有 product DTO、console root/closure 反例。原 `TestTM04ConsoleQueryBoundary` 及 #76 execution/CAS/ack parents 不变。 |
| `internal/steerline/launchers.go`: `//go:embed launcher/*` | embedded templates 仍归 host package；模板路径、嵌入规则与 native launch argv 未变。Session launcher 的实际实现路径由 `app.go` 修改负责。 |
| `internal/execution/backend/backend.go`: `ManagedProcessHost` | 接口方法/参数/返回类型不变：`StartManaged(*exec.Cmd) (StopHandle,error)`、`InspectProcess(*ProcessIdentity) string`、`NewProcessGeneration() string`、`WriteExecutionFile(string,[]byte) error`、`OutputCommand(context.Context,LaunchCommand) ([]byte,error)`。 |
| `internal/coreboundary/check.go`、`internal/coreboundary/execution_boundary_test.go`、`internal/testboundary/boundary_test.go` | #78 拥有新 adapters/resources prefix、direct/transitive adapter 禁止依赖守卫、TM06 negative fixtures、移入 resources 的 console/E2E 路径；#77 拥有纯 DTO、console root/closure 的规则和反例。既有 `TestTM04ConsoleQueryBoundary` 与 #76 execution/CAS/ack 断言保留原 owner。 |
| `cmd/test-feedback/scoped.go` | #78 拥有迁入 adapter/protocol/workspace/publication 的 `quickOwnerCases` package/parent 映射与 `TestTM06ResourceWiring` 的 boundary group/assignment；#77 拥有产品 api/client/query/taskview/console `quickRulePackages` 与产品图 `quickBoundaryAssigned` 子例。当前改动保留既有组和范围。 |
| `internal/storage/ports`: `TaskGraphSnapshot` 定义及其查询/消费者 | #77 唯一所有者，连同定义和全部消费者切换。本分支未复制、重声明或改写此接口。 |

具体旧/新文件和每个受影响测试父/子例、断言所有者列于 [test-mapping.json](test-mapping.json)。源包没有旧路径 alias、兼容 facade 或转发实现。

## 集成与后续所有者

- #77 继续拥有 product/api、product/query、client、console 与其专属产品工厂。此分支只拥有上表里的 backend/resource 构造段；公共 app 文件如发生整合，以语法段/字段所有者顺序合并。
- #77 正迁移的共享纯 `TaskGraphSnapshot` 由 #77 单点完成定义与消费方切换；本分支未改 storage/ports 相关定义/消费者。
- #80 接收本分支提供的新 backend 工厂和 `execution/process` Host/resource 小端口，不负责补本次遗漏的调用者或编译修复。
- #79 及后续任务不由 #78 启动；它们按各自硬前置执行。
