# Worker 停服恢复：2026-09-30 本机验证

实施基线：`origin/main` `472d61e`（含 #144、#148、#149）；分支 `codex/windows-local-job-host` 的独立 worktree。宿主 `LIUMING-ASUS`（用户提供地址 `192.168.50.47`），Go `1.26.4 windows/amd64`，真实 `codex-cli 0.157.1`。本记录对应[Spec](../../spec/worker-service-recovery.spec.md)和[Plan](../../spec/worker-service-recovery.plan.md)。验证使用临时 Git 仓库、临时 SQLite 与合成 Task；没有使用 `C:\SteerlineRuntime`、#101 或现有 Orca 终端。

## 已验证

| 范围 | 方法与结果 |
| --- | --- |
| Windows 受限 Job 内启动与停服 | `TestWindowsOwnedWorkerRunsInJobAndStopsWithOwner` 在本机创建受管父子进程树，确认子进程属于同一 Job；关闭归属 Job 后父子进程均退出。worker 启动不依赖 `CREATE_BREAKAWAY_FROM_JOB`；独立服务启动的脱离核验仍然严格。 |
| 本地停服人工继续 | `STEERLINE_WORKER_RECOVERY_LIVE=1 go test ./internal/steerline -run '^TestLocalCodexManualRecoveryLive$' -count=1 -timeout=12m -v` 通过。子进程在原生问题待答时 `os.Exit`，原受管 app-server 随归属 Job 关闭。重开只创建待决策记录，没有新 worker/turn；人工选择“原 Codex 会话继续”后创建新 Attempt/进程，恢复原 thread，完成不同的新 turn。 |
| 本地进程与原生身份 | 最新主线真机输出：旧 PID `44576`、新 PID `32712`；thread `01a0f0e0-7863-7180-8297-2b74b7ffa2d3`；旧 turn `01a0f0e0-79cb-7b03-ac0f-1098470ca286`、新 turn `01a0f0e0-a547-77c2-8380-7c6e33d7e2fb`；旧 Attempt `049af837bad13737be3cae2597896f16`、新 Attempt `ae57f3d35b03806dca804d5911b29774`。旧问题为“Should I reply A or B?”；旧原生 request 不被重用。测试结束清理新进程。 |
| 本地模式、历史与重复 | `TestWorkerFlowServiceFirstFeedbackKeepsSessionAndWorkspace` 覆盖自由文字、选项加备注、等待合并 PR 的反馈及明确的“新会话继续”。断言当前 HEAD/工作区保留、旧新 `process.json` 分属各 Attempt、旧 Attempt 保存进程/thread、迟到旧事件不能推进新 Attempt、重复应用不增建 Attempt。“新会话继续”在 mock 中以新 Attempt 的 `thread/start` 回执核对；mock 返回固定 thread ID，因此不作为不同真实 UUID 的证明。 |
| 启动与已保存输入 | `TestServiceRecoveryGateOnlyForPriorOwner` 证明首次接管新领取任务照常启动，换服务实例时建立门槛。`TestRestartDoesNotSendPreviouslyAcceptedFeedback` 证明重开不会投递旧的已保存答案。恢复命令负载固定来源 Attempt 与恢复版本；过期命令不投递。 |
| Orca 隔离 fake | `TestNativeOrcaCenterReusesWorkerAfterRestartAndSettlement` 通过。重开核对原 terminal/Dispatch，未自动新增 Dispatch、reply 或 send；新提交的答案投递到原问题。`TestNativeOrcaFixedLunaLaunchReconcilesOriginalTerminal` 通过：重开读到结果 relay 文件时不自动 `worker_done`，创建人工确认 Decision；确认后仅向原 Dispatch 补报一次，并沿原回执结算。fake 还覆盖原工作区、coordinator、Run、reply/ack 回执丢失的原身份对账。没有操作真实 Orca terminal。 |
| 最小回归与跨平台编译 | 最新主线运行 `go run ./cmd/test-feedback -quick` 通过，摘要在本机临时目录 `steerline-test-feedback/20260930T055315.246536900Z/summary.json`。受影响的 local、Orca fake、受管 Job 与 codexworker 定向测试通过；`GOOS=linux GOARCH=amd64 go build ./internal/steerline ./internal/steerline/workerflow ./internal/steerline/codexworker` 通过。 |

真实测试的旧进程消失只用作进程事实；原轮次缺少可信完成事件时被判为中断，旧问题被保留并转成关联的恢复 Decision。人工继续的新提示先要求核对当前工作区和持久历史。原 thread 在这版 Codex CLI 上经新 app-server 的 `thread/resume` 可恢复，并接受新 turn；这项结论只覆盖上述本机版本及测试条件。

## 能力边界

Orca agent/terminal 已退出、runtime 或 incarnation 无法核实时，当前代码保留未知事实并禁用替代 worker。尚无不操作现有 Orca 终端就能证明“同一工作区安全创建新 agent、排除旧 Dispatch 写者、绑定新身份”的真机证据，因此没有把新 Orca terminal 标作可用的手动继续方式。Orca 的存活重接、既有 `worker_done` 结算及新人工 reply 已用 fake 验证；真实新 agent 恢复仍需单独授权的隔离宿主验收。

正常停服对归属本地进程尝试有界 `turn/interrupt`，随后关闭拥有的 Job；进程退出不被伪造成原生 `turn/completed`。本次真机闭环采用待答时宿主进程直接退出，验证了没有停服标记时的恢复路径。Windows 服务启动器自身要求独立生命周期的 `startDetachedProcess` 仍执行 Job 外核验；本次没有宣称当前桌面宿主可由该入口启动独立服务。

旧的 `192.168.0.56` Job 外验收见[历史记录](../local-codex-app-server/acceptance-2026-09-29.md)，只能说明旧独立存活方案在另一宿主通过，不能证明本机新恢复语义。

## 质量审计补验

审计发现领取事务与随后 Service 保存之间存在窗口：若此时退出，未保存服务归属的 Execution 可能被下一个实例当作首次启动。现在领取事务直接写入服务实例；缺少归属的已领取 Execution 进入人工恢复门槛。已接受的旧答案也会先经过来源 Attempt、恢复版本和服务实例时间核验，再允许从等待 PR 合并阶段进入反馈阶段。核验未知的 Orca agent 现在进入统一待决策并保持不可回复；原轮次 `worker-turn` 操作结果不明时，人工继续不能跳过该操作。Windows 受管 Job 定向测试又验证了关闭归属后子进程一同退出。相关定向测试和 quick 入口在本次审计后通过。

复审补充修复：已终止 worker 的重复核验曾反复替换恢复 Decision，现按恢复版本和核验状态保留同一问题；本次恢复后新接受的答案不再被当作旧答案重新确认。迟到的 Attempt/恢复版本核验结果仅结束本地观察，不改新执行，也不使服务退出。未知操作检查同时关联 Execution 和 WorkflowRun，覆盖按 WorkflowRun 绑定的本地发布。`TestDeadWorkerRecoveryPreservesDecisionAndFreshAnswer` 用隔离 SQLite 证明重复轮询、刚接受的答案、迟到核验和未知发布均不产生替代执行。受影响 local/Orca 定向回归通过，quick 摘要为 `steerline-test-feedback/20260930T064527.054131500Z/summary.json`。
