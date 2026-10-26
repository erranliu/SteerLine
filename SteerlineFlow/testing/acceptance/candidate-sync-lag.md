# 候选状态同步滞后修复

2026-10-26，普通开发，用户明确授权修复及补充测试。源码基线 `6e337edda39ff72bbb3a01cecd1f63022c20ed07`，本工作区修改后验证；不作为干净提交的 Git 测试历史导出。

## 定位与实现

原安装现场 `C:\SteerlineInstall`、项目 `C:\Projects\merge-h-308-code`、Task 18、Candidate `dda2a775bf4c4e8caf598846a91cfe0e` 仅做只读诊断：API revision 27 连续约二十秒未变时，公开 Runner 已到 revision 33；API 自身读取约 0.1 秒。项目有 7 条 failed 和 3 条 cancelled 测试生成，全部已确认 stopped，仍每轮逐条 Inspect；抽样单次 1.5–2 秒。应用心跳独立于候选观察，不能据此证明候选新鲜。

修复沿用串行项目 actor，不引入并行状态写者：

- 候选 Reconcile 并发布在辅助同步之前执行，辅助错误仍记录诊断，但不跳过候选观察。
- 辅助同步共用每轮五秒预算，进度、生成、材料阶段超时后从下一阶段继续；进度和生成每轮至多查询一条，轮转游标在查询前推进，避免慢记录阻断后续记录。
- generated/failed/invalid/cancelled 等终态且资源确认 stopped 的生成记录不再查询或改写；取消且从未提交的记录同样无需外部查询。未取得终态结果或 stopped 证明的记录继续观察。
- 生成队列仍先重建全部记录的占位；已结束生成沿现有 Settled 规则释放，不重复提交、停止或恢复原生运行。

## 测试映射

新增回归归现有 L4 `go.full` owner 范围，不扩大固定 L1；定向命令不代表 Go 全集通过。

| 回归 | 断言 |
| --- | --- |
| `TestTMGenerationTerminalObservation` | 四类终态 × stopped 有无：已停止不重复查询/停止/改写，未核实停止仍查询。 |
| `TestTMGenerationObserveNextDoesNotStarveAfterTimeout` | 首条生成超时后，下一次观察另一条生成，独立错误保留。 |
| `TestProgressObserveNextDoesNotStarveAfterTimeout` | 首条进度查询超时后，另一候选仍能收取有效进度。 |
| `TestTMAuxiliaryStagesAdvanceAfterCancellation` | 阶段取消后让出，下一轮继续下一阶段，错误不丢失。 |
| `TestRunnerActorPublishesCandidatesBeforeAuxiliaryFailure` | 真实隔离 SQLite 中，辅助报错或等待五秒期限前候选新 revision 已发布；后续 tick 仍推进并清除恢复后的诊断。 |
| `TestCandidateStopReliabilityBlockedObservation` | 原候选、辅助生成和普通控制的外部阻塞均可由停止控制打断；生成子场景按新顺序真正进入辅助回调。 |

## 本次结果

原始报告均在仓库外独占 scope `C:\SteerlineDebug\wt-0ffd-sync-lag`。

- 新增回归及原停止回归：`diagnostics\initial-regression\summary.json`，19 个用例全部通过，无跳过，34.658 秒。原命令、包及源码状态在 summary 中固定。
- L1：`test-results\L1\20261026-sync-fix\summary.json`，通过，准备 2.3 秒、执行 18.1 秒。
- L2：`test-results\L2\20261026-sync-fix\summary.json`，`smoke.delivery` 通过，整体约 143 秒；`scenarios\smoke.delivery\scenario.json` 确认 `cleanup_complete=true`。
- 扩展 owner 首轮：`diagnostics\owner-regression\summary.json`，62 用例中 60 通过、2 失败（同一父测试及子测试），无跳过。失败为旧的 `TestRunnerActorBindsReceiptToOriginalHostFile/changed`，要求 TickControl 返回致命错误，与当前“拒绝回执、发布诊断、保持服务运行”的实现不符。
- 未修改源码复核：将基线 `git archive` 到仓库外 `baseline-source`，定向结果在 `diagnostics\baseline-receipt-confirmed\summary.json`，复现相同失败。`diagnostics\baseline-receipt\summary.json` 是修改工作区的定向复现，不作为基线证明。修正旧断言，同时核对无回执和明确的文件变化诊断，没有修改控制业务实现。
- 扩展 owner 修正后：`diagnostics\owner-regression-fixed\summary.json`，62 个用例全部通过，无跳过，65.682 秒；覆盖测试生成/共享队列、进度、项目维护、actor、停止控制及共享 Runner 接线。
- 文档链接检查（582 文件、3641 链接）、测试清单/场景登记一致性和 `git diff --check` 通过。

未部署安装目录、重启活动应用或重新提交原候选；本次自动测试使用隔离夹具，不代表原安装页面已运行修复代码。没有运行完整 L4 Go 集或真实远端模型流程。
