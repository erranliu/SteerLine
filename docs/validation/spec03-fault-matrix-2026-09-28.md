# Spec 03 R01–R19 故障矩阵核对（2026-09-28）

逐项按[Spec 03 第 8 节](../spec/03-remote-execution-and-viewer.md)核对当前代码、自动化和原 Orca/SQLite 回执。下表的“自动”只证明替身与存储边界；Spec 要求的 R01、R02、R06、R07、R08、R16、R18 真机样本单独列出。已结算失败的早期尝试不改写为成功。

| 门槛 | 当前证据 | 判定 |
| --- | --- | --- |
| R01 正常问答到合并 | [原生问答 PR #17](spec03-native-question-2026-09-28.md)，后续[可见 viewer 与 PR #18/#19](spec03-viewer-latency-2026-09-28.md)核对五步、原问题/答案应用证明、2 秒画面、固定 head 合并、Task close 和 Dispatch release | **真机通过**，限已测 `SteerlineTest` 与版本组合 |
| R02 启动后丢回执 | [原 worker-start 回执丢失](../spec/orca-probe/samples/spec03-supervised-r02-worker-start-response-loss-2026-09-26.json)与[精确结算](../spec/orca-probe/samples/spec03-supervised-r02-worker-settlement-2026-09-26.json)保留一个 Dispatch/unknown，不再派一个 | **真机通过** |
| R03 消息收到但本地未提交 | `TestSupervisedDeliveryIsNotAckedWhenLocalCommitFails` 验证 SQLite 提交失败时不发 Orca ack、消息未落库；`TestConversationProcessExitBeforeCommitRecoversAllBoundaries` 验证进程退出后的事务回滚与重放 | **自动通过** |
| R04 提交后 ack 前退出 | `TestSupervisedDeliveryAckPersistsPlatformReceiptAndDeduplicates`，以及[R19 原 publish Delivery 丢失重放](../spec/orca-probe/samples/spec03-supervised-r19-retry2-publish-2026-09-26.json)按原 ID 对账 | **自动及真机通过** |
| R05 双 viewer 回答冲突 | `TestDecisionAnswerFirstWinsIdempotencyAndConflictAreTransactional`、两个 OS 客户端仲裁及控制台 stale-draft 测试共用中心命令 | **自动通过**；没有声称两块可见桌面同时操作 |
| R06 答案断线/回复丢失 | [原 reply 丢回执](../spec/orca-probe/samples/spec03-supervised-reply-loss-2026-09-25.json)与[答案保存后离线](../spec/orca-probe/samples/spec03-supervised-answer-offline-2026-09-25.json)核对原问题/Attempt，不盲发第二次 | **真机通过** |
| R07 步骤完成窗口中心中断 | [可见服务关窗后的 prepare](../spec/orca-probe/samples/spec03-native-r07-visible-window-2026-09-27.json)、[implement 在中心离线期间完成并由原库接回](spec03-r07-implement-2026-09-28.md)，以及原 Dispatch 的 [verify](../spec/orca-probe/samples/spec03-supervised-r07-verify-restart-2026-09-26.json)、[review](../spec/orca-probe/samples/spec03-supervised-r07-review-complete-2026-09-26.json)、[publish](../spec/orca-probe/samples/spec03-supervised-r07-publish-complete-2026-09-26.json)完成窗口已测 | **真机通过完成窗口**；新增 implement 样本后续三次提示需人工 Enter，不能宣称无人值守可靠 |
| R08 网络/runtime/身份变化 | [home runtime 替换](../spec/orca-probe/samples/spec03-supervised-r08-runtime-replacement-2026-09-26.json)与[旧绑定拒绝](../spec/orca-probe/samples/spec03-supervised-r08-adapter-refusal-2026-09-26.json)，原 worker 仍在原 Dispatch 结算 | **真机通过** |
| R09 无结果/错误身份/重复完成 | `TestSupervisedWorkerDoneWithoutStructuredResultBlocksCompletion`、`TestSupervisedResultCannotCompleteAnotherAttempt`、`TestSupervisedResultDeliveryReplayUsesPersistedJSONContent`；成功 worker_done 无原结构化结果拒绝、外来 Attempt 不落结果文件、同 ID 重放只接受相同内容 | **自动通过** |
| R10 分页/source_changed/淘汰 | `TestOrcaReadOutputBindsCursorToExactSource`、`TestOrcaReadOutputReportsRetentionGapOnInitialRead`、`TestConversationLiveSourceFailureKeepsGapAndMapperPosition` | **自动通过**，不把保留缺口伪造为完整正文 |
| R11 旧 Attempt 迟到消息 | `TestConversationLateOldAttemptCannotCrossNewAttempt`，R09 的外来 Dispatch/Attempt 结果守卫 | **自动通过** |
| R12 PR 合并/未合并关闭/部分结项 | `TestNativeDeliveryNeedsExactMergedPRAndReleasesSuccessorOnce`、`TestInconsistentDeliveryStaysVisibleAndBlocksSuccessorClaim`、closed_unmerged 测试；真实 PR #17/#18/#19 均回读固定 head 后结项 | **自动及真机通过** |
| R13 中心路径与旧接口预检 | `TestNativeOrcaProjectStepsUseFixedTaskContextAndCenterDelivery` 拒绝非 target 执行地点，`TestNativePreflightDoesNotRequireBeadsAndRejectsLegacyStepEntrypoints` 拒绝旧入口 | **自动通过** |
| R14 viewer/窗口/取消边界 | viewer `q` 退出仍留中心运行、[可见关窗](../spec/orca-probe/samples/spec03-native-r07-visible-window-2026-09-27.json)保留原远端资源；`TestLoadConfigRequiresExplicitRemoteCancellationFalse` 将远程取消固定为 unavailable | **真机及自动通过该 fail-closed 范围**；不宣称远程取消可用 |
| R15 claim 回执/领取竞争 | `TestNativeClaimRechecksReadySlotAndPersistsReservationWithoutAppliedReceipt`、`TestNativeClaimCompetitionCreatesOnlyOneWorkflowReservation` | **自动通过** |
| R16 有界修复/预算 | [真实失败验证→修复→复验](../spec/orca-probe/samples/spec03-supervised-r16-repair-cycle-2026-09-26.json)及[零预算 review 阻断](../spec/orca-probe/samples/spec03-supervised-r16-review-budget-2026-09-26.json)，无越步 PR | **真机通过** |
| R17 verify 后代码/旧结果 | `TestMinimalWorkflowRevisionGatesAndRepairLimit`、`TestSupervisedCandidateWorkspaceDoesNotReplacePreparedInputBeforeGate` 与 R09 当前 Attempt 身份守卫，固定候选/检查提交不符即拒绝 | **自动通过** |
| R18 同工作树独立步骤 | [Task 91024 五个固定 Dispatch](spec03-native-question-2026-09-28.md)和[后续两个完整任务](spec03-viewer-latency-2026-09-28.md)复用同一远端工作树、逐步释放，无重叠修改 | **真机通过** |
| R19 越步发布/回执丢失 | [真实 PR 已创建后的 publish Delivery 丢失与同 ID 重放](../spec/orca-probe/samples/spec03-supervised-r19-retry2-publish-2026-09-26.json)，[固定 head 合并/原 Task close](../spec/orca-probe/samples/spec03-supervised-r19-retry2-complete-2026-09-26.json)；门槛测试拒绝提前 PR | **真机及自动通过** |

**仍需完成：** R01–R19 的上述替身/存储分支及要求真机的七项已有对应证据；[R07 新样本](spec03-r07-implement-2026-09-28.md)同时暴露 verify/review 启动需要三次受控人工 Enter，生产无人值守可靠性未通过。[SteerLine 项目能力报告](spec03-production-project-capability-2026-09-28.md)只覆盖受监管 Orca 基础能力，未运行本项目原生五步 Task。Windows 管理员/UAC 可见设置提示属 P10，精确 TTY turn/final 与 Orca 自身同句柄换代属 #3，均不由本矩阵替代。当前 `SteerlineTest` 远端取消保持 unsupported。
