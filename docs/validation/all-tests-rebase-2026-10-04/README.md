# 最新主分支上的完整测试复核

日期：2026-10-04（Asia/Shanghai）。用户要求 rebase 到最新主分支，重新执行全部测试，修复失败并创建 PR。

## 源码与范围

已 fetch 并无冲突 rebase 到 `origin/main` 的 `9da05cb`（SQLite 持久化模块 PR #197）；原修复提交重放后为 `f930c83`。上轮修复及历史证据见 [完整测试修复记录](../all-tests-2026-10-03/README.md)。

按 `tests/e2e/README.md` 和 `docs/testing.md` 的明确范围重新运行 Full、L1、完整 L2、L3 delivery/recovery 的 controlled/live 全部场景，以及带标签的内部环境和 sandbox-setup 单测。不会批量启用独立原生验收开关，也不执行管理员 sandbox 初始化。默认 skip 单独记录，不当作 E2E 通过。

## 本轮新增修复

首次 L2 在构建 controlled helper 时失败：SQLite 模块重构移除了 `steerline.OpenSQLiteReader`，E2E driver 的三处接线仍引用旧入口。修复仅在 driver 适配层：

- 按数据库路径读取 workflow/project view 时使用当前公开的 `sqlite.Factory.OpenReader`，再消费原有公开 query 接口。
- 从项目配置读取诊断视图时使用生产的 `steerline.OpenNativeTaskReader`，保持项目身份校验和只读 reader 所有权。
- 保留原来的同一事务快照、业务断言和关闭 reader 行为；没有增加旧 API 包装或兼容路径。

首次失败报告：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261003T222842.148726800Z/summary.json`。门槛构建失败后没有进入真实模型阶段。

完整回归另发现 `TestCM69PurePortDependencies` 仍把 #70 连接契约对既有 query 读端口的组合判作非法依赖。依据 #70 复用既有 query Reader 的要求，只允许 `connections.go` 引用 query 读契约；其他写端口文件继续保持原纯值所有者清单，禁止 SQL、中心及执行实现。persistence、query 和依赖边界定向回归通过。

第一次进入真实 Orca 的运行在 10 分钟期限内没有启动模型；证据显示唯一 Enter 已得到 bytesWritten 回执，但原 `Pasted Content` 草稿仍在，且未产生 native session 或 PR。修正测试适配层的异步提交观察：草稿稳定门槛从 1 秒改为 3 秒，保持原 25 秒稳定观察上限；唯一一次 Enter 后只读等待原屏幕草稿消失或出现运行指示，最多 25 秒。不重发任务正文或再发送 Enter，不手写 started/settled 回执。未证实提交时保留原回执和最后屏幕并明确失败。新增回归覆盖草稿继续停留、随后清空、截止时间、替换终端和非当前屏幕拒绝。

Orca 按键语义通过当前安装版本 CLI 帮助、`orca-cli` 版本匹配指南及本机实现只读核对；没有更新 Codex/Orca 或修改它们的安装文件。实际原生模型、usage、PR 合并和资源清理仍由正式 E2E 断言验证。

## 复核结果

报告根目录：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/`。各次报告包含原始日志、源码信息；E2E 另包含构建哈希、场景快照、usage 和清理结果。

| 范围 | 命令 | 本轮报告与结果 |
| --- | --- | --- |
| Full | `go run ./cmd/test-feedback -workers=3` | 首次 `20261003T222833.319020300Z` 仅纯端口守卫失败；修复后 `20261003T230248.057097800Z` 执行中 |
| L1 | `go run ./cmd/test-feedback -level=L1` | `20261003T223046.409874600Z`，通过 |
| L2 gated 全部 3 场景 | `go run ./cmd/test-feedback -level=L2` | `20261003T222944.437445800Z` 的 Orca 超时、清理完成；提交观察修复后 `20261003T225907.041937600Z` 全部通过，清理完成 |
| L3 controlled 全部 2 场景 | `go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=controlled` | `20261003T224628.208493900Z` 通过，清理完成 |
| L3 live 全部 4 场景 | `go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=live` | 待执行 |
| 基础设施与边界 | `go test ./cmd/test-feedback ./tests/e2e/internal/... ./internal/testboundary -count=1` | 通过 |
| tagged 内部环境与 CLI | `go run ./cmd/test-feedback -- -tags=e2e ./tests/e2e/internal/... ./tests/e2e/cmd/... -count=1` | 待执行 |
| 所有 E2E 包编译 | `go test -tags=e2e ./tests/e2e/... -run '^$' -count=1` | 通过；编译结果不替代场景执行 |

本机原生版本为 `codex-cli 0.159.2`、Orca `1.4.209`；Orca app/runtime 可达。真实场景仍使用登记的 `gpt-6-luna` / `medium` 及独立测试 home。
