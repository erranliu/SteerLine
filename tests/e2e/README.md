# 分级测试与 E2E 环境

L1 是当前固定 quick 加按改动选择的定向回归。L2 是固定交付 smoke；L3 按业务组选择，首期只有 recovery。每级独立运行，Plan 按最小必要范围选择，不自动串跑其他级别。

```powershell
go run ./cmd/test-feedback -level=L1
go run ./cmd/test-feedback -level=L2 -env=controlled
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled
```

保留 `-quick` 和 `-workers=3`。后者是完整确定性 Go 回归，不包含带 `e2e` tag 的场景，不是真机验收。L2/L3 首期只支持 Windows、controlled、串行；未知组/环境、未匹配或未登记测试均失败。L3 必须指定组；支持以逗号选择多个已登记组。新增级别与原始 Go 参数、quick/full/build-only 等模式不能混用。`-out` 可指定一个空目录；默认输出到系统临时目录下的 `steerline-test-feedback/<运行时间>`，重复使用已有产物目录会被拒绝。

## 当前场景

| 级别 | 场景 | 验证 |
| --- | --- | --- |
| L2 | `smoke.delivery` | 正式创建/领取 A，中心发布 PR，合并前 B 阻塞，合并后同一 A 交付且 B 就绪但未启动 |
| L3 | `recovery.waiting-merge-restart` | 等待合并后停止/重启，原任务、数据库、工作流、执行和工作区保持，不重派；随后正常交付 |

## 包与环境

场景分别位于 `smoke` 和 `suites/<组>`，共享 `internal/testenv`、`internal/driver` 和 `internal/controlled`。生产代码不导入这些包。各场景保留自己的断言，L3 不调用 L2 的测试函数。环境没有第二套任务状态机。

Run 持有本次构建的产品/helper 和选择清单；Scenario 持有独立 scratch 和证据；Project 持有独立 home、Git/remote、SQLite、服务和 provider；Task 来自正式创建回执，Spec/Plan 真实提交并绑定。每个场景新建现场，同场景多个项目也不共享可写资源。服务重启保留原现场，重跑场景创建新现场。

controlled 使用真实 SteerLine、Git 和 SQLite。辅助进程实现必要 app-server 协议与 GitHub command provider：worker 修改/暂存文件并提交 publication-ready 标记；中心按当前 `host_service` 职责真实 commit/push/create。模拟外部 GitHub 状态只存于独立 provider 文件，不写业务数据库。

当前本机发布策略固定 GitHub URL。测试通过配置中的 Git 命令适配器将网络操作映射到本项目的 loopback HTTP `git http-backend`，保留正式 remote 身份查询；元数据、提交、推送和合并均由真实 Git 执行。Git 命令原始 stdout 原样传回，适配器不合成 Git 结果。它是受控外部传输，不访问真实 GitHub、不证明托管平台或原生权限能力。

任务创建和查询使用当前 CLI。领取通过既有公开 `SubmitControl`/`WaitControlReceipt` 客户端提交完整 Profile 选择，与控制台相同；当前 CLI 未提供 Profile 选择参数。驱动在隔离辅助进程内读取配置，正式服务核验并处理命令。必要执行身份用已有公开只读 reader/query 获取；不直接 SQL 写库或读取私有状态。

每个子进程明确设置项目 home、临时目录、受控状态和 Codex home；清除继承的原生测试开关、Git 全局配置和代理，记录实际环境组成。服务/helper 进程使用共享 `internal/testprocess` 的 Windows Job 所有权管理，取消/超时终止本次进程树。单场景上限 5 分钟，单命令 30 秒，清理 30 秒；基于条件等待，不以固定长 sleep 判定通过。

## 新增场景

1. 在对应包增加带 `//go:build e2e` 的原生 Go 顶层测试，使用 `testenv.NewScenario(t, "固定场景ID")`。
2. 必要时增加小型项目/任务材料或共享环境操作。业务断言保留在测试函数中。
3. 在 `suites.json` 登记 `id/level/suite/package/test/env`，只登记已经支持的环境。
4. 在本次 Plan 写清新增行为、执行命令、通过条件和证据位置。执行相应级别/组一次。

统一入口一次构建当前源码并精确选择测试。直接运行场景包若缺少运行配置会失败并提示入口。场景级别与环境是两个维度；后续真实环境在现有所有权边界内增加，不提前创建认证/调度框架。

## 产物与失败

`summary.json` 保存源码/工作树、构建 SHA-256、命令、选择与结果、准备/执行时间、清理状态；各组保存原始 JSON 日志和 stderr。`scenarios/<id>` 保存公开状态快照、helper 身份/计数、命令回执、服务日志、资源清单和 `scenario.json`。

正常或失败均先保存已有诊断，再停止资源、关闭句柄、删除本次 scratch。证据目录独立于 scratch，始终保留。失败、skip、零匹配、重复/缺失、截断/非法日志、超时、证据缺失、清理或写报告失败均不能通过，不自动重跑。断电后的跨运行回收不属首期范围。

基础设施定向验证：`go test ./cmd/test-feedback ./internal/testfeedback ./tests/e2e/internal/... -count=1` 验证选择/汇总和纯辅助逻辑；`go test -tags=e2e ./tests/e2e/internal/... -count=1` 显式验证两项目隔离与部分准备清理。两项目检查使用真实创建/读取和独立数据库；已有取消回归核对受管后代不会存活。按需运行，普通包测试不启动项目环境或业务 E2E。

## Plan 测试计划示例

| 级别 | 组/场景 | 环境 | 命令 | 时点与通过条件 | 证据 |
| --- | --- | --- | --- | --- | --- |
| L1 | quick + 受影响定向测试 | 本机 | `go run ./cmd/test-feedback -level=L1` | 最终候选一次，固定最小集通过 | 本次 summary/log |
| L2 | `smoke.delivery` | controlled | `go run ./cmd/test-feedback -level=L2` | 正式交付闭环通过 | 本次场景产物 |
| L3 | `recovery.waiting-merge-restart` | controlled | `go run ./cmd/test-feedback -level=L3 -suite=recovery` | 原身份保持、无重派、交付通过 | 重启前后快照 |

仅选择本次修改需要的项；纯文档修改做链接、格式与语义检查，不要求运行以上三项。
