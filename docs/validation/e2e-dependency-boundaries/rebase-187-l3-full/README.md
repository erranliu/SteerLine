# 新主干全 L3 与完整 Go 回归：本轮未通过

2026-10-02（Asia/Shanghai），候选 `54a1f81`（生产代码与 `a3fb771` 相同，含 main `5bbbfcd` / #187）。按用户要求完整运行一轮，无自动重试或代码修复。

## L3：5/6 通过

| 场景 | 环境 | 结果 | 场景用时 | 已验证原生累计 tokens |
|---|---|---|---|---|
| delivery.basic | controlled | pass | 95.99s | 无模型请求 |
| recovery.waiting-merge-restart | controlled | pass | 135.40s | 无模型请求 |
| delivery.local-codex | live | pass | 131.00s | 64,340 |
| delivery.orca-codex | live | pass | 307.38s | 319,463 |
| recovery.local-codex | live | pass | 149.31s | 96,588 |
| recovery.orca-codex | live | **fail：等待 waiting_pr_merge 超时** | 637.83s（含清理） | 未取得完整 usage，不按零计 |

所有场景 cleanup_complete=true。live 使用完全访问、gpt-6-luna / medium、loopback Git/PR provider，没有外部测试 PR。本地 recovery 重启前/后及交付后的 usage 均为 96,588；原身份和权限断言通过。Orca delivery readiness/business 均成功。

Orca recovery 在 10 分钟场景期限内没有到达 waiting_pr_merge，最终错误包含 `task list ... context deadline exceeded`。最终公开快照中 preparation verified/ready，执行 pending，pending_command/pending_turn 已保存，没有 blocker；未进入服务重启阶段。没有足够证据把原因归为模型、host、writer 或测试驱动中的某一项。失败现场已清理，筛选状态见 `recovery.orca-codex/timeout-state.json`。

成功场景的已验证原生报告合计 **480,391 tokens**；失败 Orca recovery 未取得 usage 样本，缺失不代表没有消耗或计费。本轮不重新发起真实任务。

## 完整 Go：failed / incomplete

命令：`go run ./cmd/test-feedback -workers=3 -out=<fresh>`；准备 3.52s，执行 1198.13s，约 20 分钟。

| 组 | 通过事件 | 失败事件 | 状态 |
|---|---|---|---|
| core-1 | 143 | 9 | failed |
| core-2 | 143 | 7 | failed |
| core-3 | 73 | 4 | incomplete：panic，无 package terminal event |
| other-packages | 366 | 1 | incomplete：构建失败及报告事件格式问题 |

共 21 个失败事件，对应 20 个顶层测试，父/子测试存在重复计数；不能作为完整覆盖通过。core-3 在 `TestSupervisedOrcaRequiresPinnedModelAndKeepsStopUnsupported` 的 fixture 构造处 panic：`remote cancellation is fixed unavailable`，部分登记测试未执行。

other-packages 的 CLI 测试无法编译：`cmd/steerline/main_test.go:28` 仍构造已移除的 `ExecutionProfile.Codex` 与 `Command.WindowsSandbox` 字段。Go 的 build-output/build-fail JSON 事件使用 ImportPath，当前严格报告 parser 将其判作缺少 package/action，所以该组的覆盖统计进一步不完整；这不改变构建失败事实。

失败初步类别（具体条目见 `full/failures.json`）：

- 旧配置/fixture：旧 codex 字段、global execution version、远端 cancellation、旧版本/远端策略 enforcement 证据。
- 原身份/envelope：不 canonical 的 profile payload、错误 binding、缺少 durable operation identity、错误配置变更用例。
- 旧断言/需审计逻辑：原终端 launch reconciliation、session delivery gate、恢复资源与 host recheck，以及 Git write roots。

本轮没有为变绿而放宽断言、添加旧格式兼容、延长期限或忽略失败。需要后续按失败用例做定向审计，确认哪些是过期 fixture、哪些是产品回归；本报告只陈述这轮运行结果。

## 证据与原始运行

本目录保留各 summary/scenario/usage、完整回归 failures 摘要与 CLI build 事件，未复制认证、私有数据库、完整模型输入或 dispatch capability。原始目录位于 `C:/Users/lmas1/AppData/Local/Temp/`：

- `sl-full-rebase187-20261002-193752`
- `sl-l3-rebase187-controlled-20261002-193752`
- `sl-l3-rebase187-delivery-local-20261002-194453`
- `sl-l3-rebase187-delivery-orca-20261002-194840`
- `sl-l3-rebase187-recovery-local-20261002-195626`
- `sl-l3-rebase187-recovery-orca-20261002-200206`

先前 L2/L3 成功记录仍是历史候选的证据，不覆盖或替代本轮失败。
