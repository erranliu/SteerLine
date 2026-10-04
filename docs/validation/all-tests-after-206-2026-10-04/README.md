# 2026-10-04 最新 main 全范围测试修复验收

按 `tests/e2e/README.md` 和 `docs/testing.md` 重跑 Full、L1、完整 L2、L3 controlled/live 的所有登记场景，以及 E2E 基础设施与编译检查。

## 候选版本与环境

- 起始 main：`28b81d82f4afb3e158cf96bd5cbf257fa48b7adc`（#206）。
- 测试期间 main 合并 #209，仅新增/修改文档，再次 rebase 到 `95237e6abc76fbee743ebc347aa1ef599a76efab`。
- 最终代码候选：`56c4740b8e6b225fcbd3b44dd0f61446767e8623`，包含自查补齐的 IPv6 URL 边界回归。冻结 Full 在此提交上启动。
- rebase 前的 `9d3e23c` 与 rebase 后的 `7b9bddf` 的 `cmd`、`internal`、`tests`、`scripts` Git tree 和 `go.mod`、`go.sum` blob 完全一致；rebase 只加入文档。随后 56c4740 补齐 IPv6 回归；最终 Full/L1 在它上面重新执行。原报告保留真实 source_commit，不改写报告身份。
- Windows/amd64，Go 1.26.4，codex-cli 0.159.2。真实场景使用 gpt-6-luna / medium、danger-full-access / approval_policy=never；L2/L3 串行执行。
- 原始证据根目录：`C:/Users/lmas1/AppData/Local/Temp/steerline-all-tests-20261004-fc837a26`。

## 修复与失败证据

1. **Markdown PR 线索被错误拼接。** 首轮 `l3-live/e2e-1.jsonl` 中 `delivery.local-codex` 返回正确的 `[URL](URL)`，原正则把两段链接合成一个候选，触发“PR URL host/scheme or format mismatch”。PR 线索提取现在将 Markdown 的括号和反引号作为边界；保持顺序、去重和多个不同候选。补充相同链接的文字/目标、不同目标及行内代码回归；测试先复现失败再通过。自查另补 IPv6 主机地址（http://[::1]:3000）的 Markdown 链接回归，确认只排除必要的文档分隔符，保留合法 URL 方括号。provider 的 host、repository、branch、SHA 核验保持。
2. **Windows 测试进程清理早于子进程退出。** 同一失败场景中 `logs_2.sqlite` 短暂仍被占用。测试工具现在枚举自己的 Job，保留并确认每个进程仍属于原 Job 后终止该树，等待实际进程句柄完成退出，再发布 Done、关闭 Job。再次检查新出现的原 Job 子进程；观察/等待/关闭错误会作为清理失败传播。回归分别验证主进程自然退出和主动停止，八个子进程全部退出后立即删除独占文件，无删除重试。首轮场景清理错误保留在原报告；整个原运行结束后 runner 的正常清理已移除其 scratch。
3. **配置测试仍使用已退役 publisher。** 首轮 `full/other-packages/test.jsonl` 的 `TestConfigKeepsOrcaCodexNativeByDefault`、`TestGlobalExecutionProfilesSharedAcrossProjects`、`TestProjectExecutionSettingsAreRejected` 失败；合法夹具改用现有 worker 发布配置，保留原断言。缺失 task_authority 测试也使用合法的其他字段，避免被无关策略遮挡。
4. **集成测试未跟随当前环境与恢复契约。** `native-integration-diagnosis` 稳定复现两项原终端测试与 `TestEmptyExecutionRequirementsKeepMandatoryChecks` 失败。丢失创建回执时等待正式环境 blocker，通过针对原身份的 recheck_environment 控制重查，不再要求额外的人工 decision。重命名终端恢复从已确认的业务回合开始，检查重启前后原 process/session/turn、turn number 和两次派发不变。发布准入负例使用已退役的 host_service，合法 worker 继续验证强制 CLI/远端权限检查。三项修复后定向通过。

未新增旧版读取、兼容或回退路径。未放宽场景期限、PR/资源身份或 usage 断言，未新增跳过项。

## 验证命令与结果

所有下列入口均显式使用本次证据根目录下的新 `-out` 子目录；省略该参数后的命令见表。

| 范围 | 命令 | 证据子目录 | 结果 |
| --- | --- | --- | --- |
| 冻结候选 Full | `go run ./cmd/test-feedback -workers=3` | `full-head/summary.json` | passed/complete=true，1055 条记录，0 失败，18 默认跳过（1461.0 秒） |
| 修复过程 Full | 同上 | `full-fixed/summary.json` | passed/complete=true，1055 条记录，0 失败，18 默认跳过 |
| L1 | `go run ./cmd/test-feedback -level=L1` | `l1-head/summary.json` | passed/complete=true |
| 完整 L2 | `go run ./cmd/test-feedback -level=L2` | `l2-fixed/summary.json` | full-L2，3 场景通过，cleanup_complete=true |
| L3 controlled | `go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=controlled` | `l3-controlled-fixed/summary.json` | 2 场景通过，cleanup_complete=true |
| L3 live | `go run ./cmd/test-feedback -level=L3 '-suite=delivery,recovery' -env=live` | `l3-live-fixed/summary.json` | 4 场景通过，usage/cleanup 完整 |
| E2E 基础设施 | `go run ./cmd/test-feedback -- -tags=e2e ./tests/e2e/internal/... ./tests/e2e/cmd/... -count=1` | `infra-fixed/summary.json` | passed/complete=true，1 个凭据预检 opt-in 默认跳过 |
| 原生集成定向 | `go run ./cmd/test-feedback -- ./internal/steerlineintegration -run '^Test(NativeOrcaFixedLunaLaunchReconcilesOriginalTerminal\|NativeOrcaFixedLunaLaunchReconcilesRenamedTerminalAfterRestart\|EmptyExecutionRequirementsKeepMandatoryChecks)$' -count=1` | `native-integration-fixed/summary.json` | 三项通过 |
| 应用接线包 | `go run ./cmd/test-feedback -- ./internal/steerlineapp -count=1` | `app-fixtures-fixed/summary.json` | passed/complete=true，3 个原生 opt-in 默认跳过 |
| URL 边界最终回归 | `go run ./cmd/test-feedback -- ./internal/steerline/execution ./internal/steerline/codexworker ./internal/steerline/orcaworker ./internal/steerline/publication -count=1` | `pr-boundaries-final/summary.json` | passed/complete=true，239 条记录，1 默认跳过 |
| rebase 后定向 | `go run ./cmd/test-feedback -- ./internal/testprocess ./internal/steerline/execution ./cmd/test-feedback ./internal/testboundary -count=1` | `post-rebase-targeted/summary.json` | passed/complete=true |

`go test -tags=e2e ./tests/e2e/... -run '^$' -count=1` 编译通过；`git diff --check` 通过。Full 的计数含父/子测试记录，不代表独立业务场景数量；默认跳过项不算已验收，独立原生 opt-in 未批量开启。

本轮登记场景：smoke.delivery、smoke.local-codex、smoke.orca-codex、delivery.basic、recovery.waiting-merge-restart、delivery.local-codex、delivery.orca-codex、recovery.local-codex、recovery.orca-codex，全部通过。完整 L2 的最终真实 PR 为 [#57](https://github.com/erranliu/SteerlineTest/pull/57) 和 [#58](https://github.com/erranliu/SteerlineTest/pull/58)，已按匹配 head 合并并清理本次分支；初轮 #55/#56 也已正常合并清理。

## 报告身份

L1、L3 live 和基础设施修复复测在首个功能修复提交 833923e 上启动。后续 532b3fd 仅保留清理成功时原命令错误类型，d63fac8/9d3e23c 仅修复上述确定性测试夹具；各补齐项均独立验证。9d3e23c 的 Full（full-final）也已通过，文档 rebase 后候选 7b9bddf 与它的全部代码/测试树相同，rebase 后定向与 L3 controlled 又分别通过。自查补齐 IPv6 边界后，解析/两种原生实现/provider 全包复测通过，最终冻结 Full/L1 又在包含全部修改的 56c4740 上执行。E2E 全部已通过；最后的边界调整只保留 IPv6 方括号，既有普通 GitHub/loopback URL 提取行为由同组回归再次核对，不重复已通过的真实模型现场。

各报告保留实际启动提交和当时 worktree_status；修复夹具的定向报告包含当时未提交的夹具改动，随后已完整提交。本文件和汇总证据只归档结果，不将失败重跑覆盖为成功。

原始报告的 SHA-256、实际提交/worktree 状态、计数与场景清理/usage/真实 PR 证明摘要保存于 [results.json](results.json)。汇总脚本逐项检查 passed/complete（E2E 检查清理和原生 usage）后才生成归档；不修改原报告。
