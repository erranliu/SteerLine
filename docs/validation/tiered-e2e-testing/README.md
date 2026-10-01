# 分级测试最小实现验证

补充：用户要求完整基线并 rebase 最新主干后的结果见[测试基线](baseline-2026-10-01/README.md)。分级 E2E 和环境检查经夹具换行修正后通过，整体 Go 回归仍有 47 个顶层测试失败；以下为最初候选的最小验证历史记录。

日期：2026-10-01。Spec：[分级测试与共享 E2E 环境](../../spec/tiered-e2e-testing.spec.md)；Plan：[实施与测试计划](../../spec/tiered-e2e-testing.plan.md)。源码基线 `49d71c9` 加本实现工作区改动；每份原始报告保留当时的源码 commit、工作区状态、工具/机器条件及构建 SHA-256。本次为 Windows controlled 验证，未运行 full、真实 Codex/Orca、远端或 UI 测试。

## 结果

| 范围 | 实际入口 | 结果 |
| --- | --- | --- |
| 选择/汇总/纯辅助逻辑 | `go test ./cmd/test-feedback ./internal/testfeedback ./tests/e2e/internal/... -count=1` | 通过；含非法组合、空/重复选择、缺失证据、skip/失败/截断/重复日志、清理错误与写报告错误检查 |
| 项目环境基础设施 | `go test -tags=e2e ./tests/e2e/internal/... -count=1` | 通过；testenv 48.762 秒，两项目真实任务/数据库独立、A 修改和清理不改变 B，取消的部分准备保留日志并不报成功 |
| L1 | `go run ./cmd/test-feedback -level=L1` | 通过；准备 0.69 秒，执行 25.57 秒，[报告](l1/summary.json)、[核心日志](l1/quick-core.jsonl)、[规则日志](l1/quick-rules.jsonl) |
| L2 | `go run ./cmd/test-feedback -level=L2 -env=controlled` | 通过；准备 3.20 秒，执行 68.66 秒，[报告](l2/summary.json)、[原始日志](l2/e2e.jsonl)、[场景/清理](l2/scenario.json) |
| L3 | `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled` | 通过；准备 3.14 秒，执行 86.03 秒，[报告](l3/summary.json)、[原始日志](l3/e2e.jsonl)、[场景/清理](l3/scenario.json) |

汇总器最后一个定向复跑 `go test ./cmd/test-feedback ./internal/testfeedback -count=1` 通过，用于确认报告写入失败时的退出/显示接线。未扩大为全量测试。

L2 公开快照记录 A 关联 open PR，B 阻塞；真实隔离 remote 合并后 A closed、有唯一 Delivery，B ready/open 且没有 WorkflowRun。受控组件只创建一个 session、一个业务 dispatch；再次查询保持同一 Delivery。[合并前](l2/delivery-open.json)、[交付后](l2/delivery-completed.json)、[再次读取](l2/delivery-rechecked.json)。

L3 在等待合并阶段停止并重启正式服务。项目/数据库、WorkflowRun、Execution、session、工作区/分支及 Attempt 数量保持，B 仍阻塞，session/dispatch 计数保持 1；随后合并交付原工作流，B ready 且未启动。[重启前](l3/recovery-before-restart.json)、[重启后](l3/recovery-after-restart.json)、[交付后](l3/recovery-completed.json)、[再次读取](l3/recovery-rechecked.json)。断言所用任务/执行事实由产品公开 ProjectView 在同一读事务中采样。

## 架构与证据边界

产品命令的依赖检查没有 `tests/e2e` 或 `internal/testprocess`；共享进程管理仅被测试执行器和测试辅助代码引用。普通 `go list ./tests/e2e/...` 只列三个辅助包，L2/L3 场景及会启动项目的基础设施测试均需显式 e2e tag。原 quick/full 默认覆盖保持。

复用原 `TestRunOwnedKillsDescendantsOnCancellation`，通过有界取消实际父/子进程检查，确认共享 Windows Job 结束本次进程树。两个最终 E2E 的 scenario/summary 均记录清理成功，scratch 已删除，证据目录保留。目录边界拒绝逃逸删除也由两项目检查覆盖。

按当前本机正式发布契约，worker 只修改/暂存并输出固定发布标记，中心执行 commit/push/create。正式 Git 命令配置中的受控传输将 GitHub remote 网络操作映射到本项目 loopback HTTP `git http-backend`，原样返回真实 Git 输出，未改变生产权限或发布规则。创建任务用当前 CLI；领取用控制台同样使用的公开控制客户端提交完整 Profile 选择，正式服务核验，不直接写库。

仓库中的报告、场景快照和 JSON 日志为原始产物副本，报告内绝对路径指向执行时的主机位置；构建二进制、逐命令回执和服务日志保留在报告的原始输出根，不纳入 Git。三个最终输出根分别为系统临时目录 `steerline-test-feedback/20261001T025808.517996600Z`、`20261001T025923.718981800Z`、`20261001T030225.133702400Z`。

以上证明受控外部组件下的真实产品内部闭环及指定恢复场景，不构成原生工具权限或真实托管平台认证，也未覆盖活动轮恢复/强制退出故障矩阵。各个新增业务组和真实环境仍按后续 Plan 独立扩展。
