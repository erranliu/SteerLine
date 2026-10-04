# #76 执行应用用例归包验收记录

实现分支现已集成最新已合入 #74/#75/#39/#217 的 main；#77 产品查询与控制台工作接续本阶段。执行 actor、命令、配置、conversation journal cursor 和纯执行记录重建现由 `internal/execution` 唯一持有；应用完整注入任务 Work/Result 能力、生命周期目录、SQLite command store、ProcessHost 和宿主端口。详细归属见 [ownership.md](ownership.md)，对外签名和新依赖方向见 [interface-handoff.md](interface-handoff.md)。

## 最小验收

| 范围 | 命令/真实 parent | 结果与证据 |
| --- | --- | --- |
| app 候选/退出/五个宽松检查 | `go run ./cmd/test-feedback -out docs/validation/three-line-04-execution-usecases/candidates/app-20261005 -- ./internal/steerlineintegration -run '^TestMC[1356789]$' -count=1`；MC1/3/5/6/7/8/9 | 通过；[summary](candidates/app-20261005/summary.json)；七项全部 PASS |
| intent/receipt/wiring/recovery | `go run ./cmd/test-feedback -out docs/validation/three-line-04-execution-usecases/wiring/final-app-20261005 -- ./internal/steerlineintegration -run '^TestTM04ExecutionWiring$' -count=1` | 通过；[summary](wiring/final-app-20261005/summary.json)；3 个嵌套场景和父用例均通过：事务失败与原请求重放、真实 app-server 两轮、原 session 的人工恢复与 ack |
| #218 app feedback/restart | `go run ./cmd/test-feedback -out docs/validation/three-line-04-execution-usecases/recovery/latest-main-20261005 -- ./internal/steerlineintegration -run '^TestNativeOrca(ConsecutiveFeedbackKeepsOriginalSession|OpenPRFeedbackResumesSavedTask)$' -count=1` | 通过；2 个 app/testprotocol parent 保留原 session/task；[summary](recovery/latest-main-20261005/summary.json) |
| 边界/Journal/重建 | `go run ./cmd/test-feedback -out docs/validation/three-line-04-execution-usecases/reconstruction -- ./internal/execution/reconstruction ./internal/coreboundary ./internal/execution -run '^(TestBuildOwnsRecordsAndMetadataAcrossRefreshes|TestProjectionRejectsBrokenBindingsWithoutReusingOldViews|TestProjectionRetainsDeliveryAndReservationRules|TestExecutionProjectionKeepsRemotePathAndWhitespaceSemantics|TestTM04ExecutionBoundary|TestJournal.*)$' -count=1` | 通过；20 个测试通过、无 skip、3 个包通过；[summary](reconstruction/run-20261005/summary.json) |
| 最小 L1 | `go run ./cmd/test-feedback -level L1 -out docs/validation/three-line-04-execution-usecases/l1/final-main-20261005` | 通过；9 组/254 项全过、无 skip；[#39/#218 execution 回归](l1/final-main-20261005/quick-execution/test.jsonl) 与 [summary](l1/final-main-20261005/summary.json) |
| 控制恢复 | `go run ./cmd/test-feedback -level L3 -suite recovery -env controlled -scenario recovery.waiting-merge-restart -out docs/validation/three-line-04-execution-usecases/controlled/post-218-20261005` | 通过；[summary](controlled/post-218-20261005/summary.json)，`TestWaitingMergeRestart` 命中并通过 |

原生 adapter 的既有验收证据复用最新 main 的 [#39 记录](../candidate-native-acceptance/README.md)；本阶段重新验证的 pending-PR 行为和 controlled recovery 均在集成 #39 后执行。MC 定向命令均在 Windows 执行，隔离 Git/SQLite/provider/进程 protocol fixture；`STEERLINE_NATIVE_ACCEPTANCE` 等原生开关未启用。它们验证 app 确实消费新的 execution actor、数据库事实、public query 和 provider receipt，不代表本机 codex/orca 权限验收。内层 execution `TestMC1–TestMC9` 和 #39/#218 的恢复原 assertions 在执行状态所有者包保留；`TestCandidateOpenPRPollPreservesOriginalPendingInput` 与 `TestCandidateResumeErrorDoesNotStarveSibling` 随 L1 一并命中；外层 MC1/3/5/6/7/8/9 覆盖跨包 wiring。

诊断运行定位并修复了三项测试/runner fixture 问题，所有失败日志保留供复核：[首次候选组合](candidates/final-20261005/summary.json)在尚无 actor Runtime 时解引用 nil，helper 现优先从同一命令 snapshot 读取候选 revision；[首次迁入 snapshot 测试](l1/run-20261005/summary.json)未注入 ResourcePort；[两次 L1 不完整报告](l1/run-20261005-rerun/summary.json)、[L1-final](l1/run-20261005-final/summary.json)发现 runner 没有统计新 owner group 的包终止事件。分别修正后，最终候选组合与 L1 全组通过。最新 main 的 #39 test 初次移动时沿用了旧 package 名导致构建失败（[诊断](controlled/latest-main-20261005/summary.json)），已改为 execution 并再次验证。#218 sibling test 现随 execution L1 固定组；app recovery 定向父用例也已按上表登记。

边界单测、capability snapshot fixture、journal cursor 重定位、执行重建与 L1 均通过。#218 的 sibling 恢复回归加入最终 L1；app 级连续 feedback 和 open-PR restart 使用 testprotocol 定向覆盖。未选择 Full/L2/live；没有修改 backend 协议/权限，也不以迁移前结果宣称平台原生行为。
