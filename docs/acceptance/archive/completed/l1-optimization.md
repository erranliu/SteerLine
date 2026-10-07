# L1 精简与并行验证（2026-10-07）

实现范围及测试计划见 [Plan](../../../spec/l1-test-feedback.plan.md)，日常与定向入口见[测试总入口](../../../testing.md)。固定选择见 [scoped.go](../../../../cmd/test-feedback/scoped.go)。

## 结果

| 项目 | 结果 |
| --- | --- |
| L1 命令 | `go run ./cmd/test-feedback -level=L1 -out=<本次产物>/l1-final` |
| 验证源码 commit | `2dbc38d868c7bf7abc9a0e3b51f21a4e6cb2bd39`，干净工作树，已 rebase main 并修复测试样本 |
| 准备 / 执行 / 总计 | 0.72 / 98.17 / 98.89 秒 |
| 通过 / 完整 | `passed=true` / `complete=true`，退出码 0 |
| 分组 | 12 组全部通过，无失败、skip 或覆盖错误 |
| 并发 | 配置三个 worker；按组起止时间核对实际峰值为 3 |
| 旧通过基线 | [固定历史 l1-release](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/product-simplification-01-console/l1-release/summary.json)，准备加执行 1151.42 秒 |
| 耗时减少 | 91.41%，由固定范围精简和进程并行共同取得，非同覆盖范围纯性能比较 |

固定集成组实际仅运行架构边界、领取事务回滚/原回执与 SQLite 接线；长恢复、多回合、候选调度及重服务查询仍在测试库存与 Full 中，按改动定向选择。普通任务、产品、存储、控制台规则及生产/E2E 边界继续执行。

首次优化验证通过，耗时 128.61 秒。随后 rebase 到 #233 合入后的 main，复验因 `TestConversationMapperNativeTwoTurnFixture` 所需历史样本被删除而失败；保留该次 `l1-rebased` 原始报告。将三个仍被 Codex 测试消费的脱敏样本原样迁入对应包 `testdata`，双回合、取消/失败映射定向测试通过，样本字节与固定历史 Git blob 一致。修复后的完整 L1 在新目录 `l1-final` 再次通过，上表为最终结果，不将中间失败改写为通过。

runner 定向回归通过：默认/显式 worker 参数、并发上限、稳定报告次序、分层集成选择、追加定向不替换固定覆盖，以及缺失/重复/skip/截断/意外恢复用例和 Full 覆盖守卫。随后单独补充并通过定向零匹配的正反例；这一步仅增加 runner 测试，不修改已通过 L1 的运行实现或固定选择。

rebase 后再次执行 runner 定向测试，额外覆盖原始产物过滤及仓库外输出；主分支所有入口默认临时输出的实现保持。最终实现源码验证完成后，仅更新本文和精简结果索引。

```powershell
go test ./cmd/test-feedback -json -run 'Test(Quick|Scoped|LevelSelections|FullCoverage)' -count=1
go test ./cmd/test-feedback -json -run '^TestQuickTargetedCoverageRejectsZeroMatches$' -count=1
go test ./cmd/test-feedback -json -run 'Test(Quick|Scoped|LevelSelections|FullCoverage|FilterGeneratedRunOutput|GeneratedRunOutput)' -count=1
go test ./internal/execution/adapters/codex -json -run '^Test(ConversationMapperNativeTwoTurnFixture|HistoryTerminalOutcomeFixturesKeepFailureAndCancellationDistinct)$' -count=1
python scripts/check_doc_links.py
git diff --check
```

未执行 Full/L2/L3：本次调整测试范围与调度，不修改产品业务，不需要真实模型或外部 PR 验收。串行配置与调度经过定向测试，未重复运行串行 L1。

## 证据

本任务的自动验收 ID 与 Plan 对应：L1A1 为固定覆盖/定向追加，L1A2 为三 worker/隔离，L1A3 为结果完整性守卫，L1A4 为扫描/样本与新基线。各行为的实际命令、原结果与限制均见上文及下列索引；补 ID 不新增通过结论，也不重标当前源码已验。后续有实现变化时按 Plan 运行受影响入口，再在本页分别登记结果。

[结果索引](l1-optimization-results.json)保存耗时、实际并发峰值、命令及原始报告位置；精确固定选择在原始报告与对应源码中。原始产物目录：

```text
C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\l1-optimization-20261007-101252202
```

`l1/summary.json`、`l1-rebased/summary.json` 与 `l1-final/summary.json` 及各组 `test.jsonl` / `stderr.txt` 保留三次原始 L1 证据。`runner.jsonl`、`runner-zero-match.jsonl`、`runner-rebased.jsonl` 及对应 stderr 保存 runner 回归；`codex-fixtures.jsonl` 及对应 stderr 保存样本定向回归，`fixture-integrity.json` 保存字节摘要与一致性。产物目录独立于仓库，系统临时目录被清理后，仓库中的精简结果仍可查阅，但不能替代原始日志重放。
