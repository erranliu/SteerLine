# 待合并可选反馈修复

## 范围

2026-10-07 用户报告的 SteerLine 交互缺陷。现场为 SteerlineTest 的任务 101：worker 已发布 PR，界面仍提示 Continue，用户提交后重复开启无待办的新轮次。此记录是本次直接修复的验收活动，不分配新的执行 DAG Task ID。

实现规则见[决策与反馈](../product-design/decisions.md)与[控制台操作](../product-design/execution-console.md)。待合并保留现有 Decision/Command 的自由文字反馈能力，默认不展示 Continue，不计为待交互；按 a 主动打开，非空反馈及 F2 才发起下一轮。原生问题、环境阻断、恢复资格和原请求身份核验继续生效。

## 自动验证

环境：Windows amd64、Go 1.26.4，本工作树基于 `2d0e811b83bafb49b23e6bec2020953f5f8132de` 加本次工作区修改；源码与安装程序以实际差异及构建摘要核对。

| 项 | 命令 / 场景 | 通过条件 | 状态与证据 |
| --- | --- | --- | --- |
| WPF-A1 控制台 | `go run ./cmd/test-feedback -- ./internal/console -count=1`；`TestWaitingPRShowsMergeAndRequiresExplicitFeedback`、`TestWaitingPRDoesNotHideRequiredInteraction`、`TestWaitingPRRecoveryGateDoesNotRequireAFeedbackTurn` | 待合并展示、交互计数为零、Enter 不打开反馈、空输入不外发；主动非空反馈保持原目标；真实问题/环境/恢复门槛保持 | 已验收；最终回归 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261007T110429.149497100Z/summary.json` |
| WPF-A2 L1 与核心反馈 | `go run ./cmd/test-feedback -level=L1 -- ./internal/execution -run '^(TestWaitingPRFeedbackIsOptionalAndDoesNotRepeatTurns\|TestWorkerFlowServiceFirstFeedbackKeepsSessionAndWorkspace\|TestWorkerFeedbackPhaseCommitIsDurableAndRejectsStaleActor\|TestRestartDoesNotSendPreviouslyAcceptedFeedback\|TestWorkerFlowPRBinding\|TestWorkerFlowDeliveryGateUsesSessionAndPRWithoutStepResults)$' -count=1` | 固定 L1 与所匹配定向用例通过；重复观测不新建轮次，反馈续跑保留会话/工作区，拒绝旧服务答案与过期事务 | 已验收；`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261007T105034.681046300Z/summary.json` |
| WPF-A3 Orca 反馈接线 | `go run ./cmd/test-feedback -- ./tests/integration -run '^(TestNativeOrcaOpenPRFeedbackResumesSavedTask\|TestNativeOrcaConsecutiveFeedbackKeepsOriginalSession)$' -count=1`；待合并用例单独复跑 | 普通及待合并反馈在重开与连续轮次后保留原 session/physical terminal；不丢失或重复 Dispatch | 已验收；普通反馈在首轮报告通过，待合并场景复跑通过；最终报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261007T110157.970457200Z/summary.json` |
| WPF-A4 构建与运行现场 | `./build.ps1`；安装后二进制 `check`；使用真实产品 client 加载任务 101，再调用控制台 Model 渲染并只读检查服务快照 | 构建/配置检查通过，实际页面突出待合并，无 Continue 默认问题；运行身份不变，无新轮次 | 已验收；`C:/SteerlineRuntime/.fix-waiting-pr-466d27787dbd4a75a15adc913e91b6af/live-console.txt`、`service-status.json`；现场仍为 run `64d36df6ae23101d8674f8d99732ce87`、三轮、`waiting_pr_merge`、无错误 |
| WPF-A5 已结束 PR 的恢复 | `go run ./cmd/test-feedback -- ./internal/execution -run '^(TestCompletedPRRecoveryKeepsOptionalFeedback\|TestWaitingPRFeedbackIsOptionalAndDoesNotRepeatTurns\|TestWorkerFlowServiceFirstFeedbackKeepsSessionAndWorkspace\|TestRestartDoesNotSendPreviouslyAcceptedFeedback)$' -count=1` | 恢复确认已结束的 PR 后保持待合并，更新反馈恢复身份，保留恢复门槛；不要求或启动新轮次 | 已验收；`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261007T110147.197031500Z/summary.json` |

最终安装二进制 SHA-256：`773E19C46BFF16488852B6520B6B451566EFFCF641111632C2EC0AEC17BB1C9F`。旧程序备份保存在上述运行现场产物目录。

首轮 Orca 报告 `20261007T105034.931000800Z` 中，普通连续反馈通过（219.41 秒）；待合并用例已完成第二轮并保存第三轮反馈，但触及四分钟场景期限而失败（243.64 秒）。仅对包含重开的待合并场景将期限调为六分钟，保持两个后续轮次及原身份/Dispatch 断言；单独复跑在 259.223 秒内通过。未将首轮失败报告标为通过。

本次不跑 Full 或新的 L2/L3 live：修改不涉及原生执行协议或发布方式，选定的控制台、L1、核心反馈与受控 Orca 接线覆盖本次行为；不通过再次驱动真实模型或合并 PR 来验证界面修复。

## 2026-10-09 Rebase 后核验

工作树已 rebase 到 `origin/main` 的 `692a784e05479b11e5818fb5a209971949e3064a`。控制台采用 PR #250 已合入的待合并/可选反馈实现及测试，原本地同功能修改不再重复叠加；服务端自由文字反馈与运行配置隔离修复保留。恢复接线保留主干仅继续原会话的限制及已核验 PR 的等待合并门槛，不恢复新会话入口。

本次 L1 加可选反馈/已结束 PR 的恢复定向测试通过，报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T015847.312785000Z/summary.json`。主干 `TestWaitingPRMerge*` 控制台定向通过，报告 `20261009T020117.425308200Z/summary.json`；新增运行配置保护测试通过，报告 `20261009T020107.237507500Z/summary.json`；原会话反馈、重开不补发、新会话拒绝和原会话不可恢复人工处理四个顶层测试（含三个反馈子例）通过，报告 `20261009T020214.986776800Z/summary.json`。后三份报告均位于上述 `steerline-test-feedback/` 临时根。未重跑 Full/live 或安装程序，原安装摘要及截图只证明 2026-10-07 的现场。Rebase 前完整修改保留在 stash `7ddcfa5b08820e24238149e94a89b6555d57dd94`，未删除备份。

## 2026-10-09 提交前核对

最终候选保持主干 `worker_lifecycle.go` 不变，去掉已由主干真实恢复入口覆盖的重复 helper 和直接调用测试；原 WPF-A5 属于历史候选，不以该直接测试证明最终候选。新增可选反馈无固定 Continue，保留原生问题及原会话恢复限制；integration 的 TestMain 使用独立临时 STEERLINE_HOME，并用子进程检查继承的运行配置没有被 fixture 改写。

`TestWaitingPRFeedbackIsOptionalAndDoesNotRepeatTurns` 与 `TestCD64ReviewReceiptAndRecoveryIsolation`（含从实际恢复入口保留已核验 PR 合并门槛的子例）定向通过，报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T021118.684253100Z/summary.json`。`TestNativeOrcaOpenPRFeedbackResumesSavedTask` 在当前主干加本次差异上通过，142.801 秒、无 skip，报告 `20261009T021129.501554900Z/summary.json`（同一临时根）。保留此前同基线的 L1、控制台、运行配置保护与原会话定向结果；不重跑 Full/live，不覆盖安装程序。规范 Git blob 与报告定位保存于 `C:/Users/lmas1/AppData/Local/Temp/steerline-worker-pr-a9dcadc516e54bc08108d85db6137f90/verification.json`。
