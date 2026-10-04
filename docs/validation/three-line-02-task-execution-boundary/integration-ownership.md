# #74 实际集成与后继所有权

本清单依据本 PR 的生产字段和方法生成；逐声明签名、行号与所有者见 [integration-inventory.json](integration-inventory.json)。当前是 #74 实现候选，只有协调聊天独立复审并真实合并后才解除 #75/#76 的硬前置。本聊天只执行 #74；不启动其它任务。#77/#79/#80/#81/#82 按用户 2026-10-04 指令暂停。

## 已接通的窄端口

| 端口 | 实际定义/构造/消费者 | #75/#76 交接 |
| --- | --- | --- |
| task.Reader、WorkPort | `internal/task/work.go`；SQLite `TaskSnapshot` 从一次图读取转换；`steerlineapp/task_boundary_wiring.go` 构造；`execution_dispatch.go` 用 Ready，`native_claim_command.go` 用 Fixed/Claim | #75 拥有 task 及其构造；#76 只消费接口。任务只见 task/model 快照、固定契约和前值。 |
| task.ResultPort | 同一 task 服务的 Declare/Intent/Discard；app 显式注入 NewService/NewCommandService；执行的交付/丢弃协调器消费 | #75 拥有任务声明，#76 拥有结论核验。`CommandService.taskResults`、`Service.taskResults` 不公开；产品/后端契约无此能力。 |
| storage/ports Claims/Results/Discards | 三个固定提交声明；SQLite `task_{claim,result,discard}_commit.go` 各一次事务 | #76 负责执行提交，#75 不持执行写端口或 SQL 回调；#79 后续移目录时保留同一实现。 |
| task-only 客户端适配 | `steerlineapp/client_task_commands.go` 的 Validate/Create/AddPredecessors、小服务接口和独立构造 | #75 可独立替换为 task 应用；该适配器没有 ResultPort。 |
| 执行丢弃客户端适配 | `steerlineapp/client_discard_commands.go`；其执行协调器内部持任务意图/结果声明能力 | #76 独立迁执行核验与清理，产品只提交原 DiscardInput。 |

首次领取原子重检任务/契约/依赖、项目/图版本和 auto 占槽；后续候选不更新 Task。开始前五候选检查保持宽松，提交只重检已核验 PR 集合及共同 base。结果/丢弃提交按原 task/graph/整个候选与执行绑定版本仲裁；交付必须关联同事务的唯一执行交付记录，不能用 success 布尔值关闭。重复原结果返回原任务及交付回执，回滚只读重放事务，不增加数据库/图版本。首次领取已有请求的版本是初次提交前值，原请求已接受后的重放仍按既有行为返回原预留；后续候选的原请求则保留版本和控制摘要。

## 文件唯一修改者

| 实际文件/区域 | 唯一修改者 | 后继协作规则 |
| --- | --- | --- |
| `internal/task/**`、root `native_batch_command.go`、`native_task_predecessors.go`、`native_contract_files.go`、`task_configuration.go`、`task_record_lookup.go` | #75 | 迁创建/依赖/文档读取用例，保持 WorkPort/ResultPort 的固定输入，不重建旧 task 包。 |
| `steerlineapp/task_boundary_wiring.go`、`client_task_commands.go`；`cmd/steerline/task_commands.go`；`cmd/test-feedback/task_scope.go` | #75 | 已有真实调用；任务的 app/CLI/runner 迁移在这些文件完成，不改共同执行构造段。 |
| `command_boundary.go`、`service_state.go`；`application_config_types.go` 的共同定义集成段；`service_actor.go`、`service_loop.go`、`service_task_snapshot.go`、`service_commit.go`、`execution_*.go`、`worker_*.go`、`candidate_*.go`、`control_*.go`、`decision*.go`、native claim/delivery/discard/dispatch/cleanup | #76 | 执行迁包和 actor。Service/CommandService 共同字段变更由 #76 单点集成；#75 不改这些文件。Config 最终迁 app 归 #80，本阶段只有 #76 处理共同定义集成，禁止两个分支各自重写。 |
| `steerlineapp/execution_service_wiring.go`、`execution_command_wiring.go`、`client_discard_commands.go`、`client_command_wiring.go`、`directory.go` 的共同构造；`cmd/steerline/main.go`；`cmd/test-feedback/scoped.go`；`internal/coreboundary/check.go` | #76（单点集成） | #75 提供自己的已编译入口及测试清单，由 #76 顺序接共同段；不把这次构造/守卫或编译修复留给 #80。Task CLI 和 runner 包选择已有独立文件。 |
| `service_console.go`、`command_submission.go`、`query_construction.go`、query/taskview/projection、`steerlineapp/client.go` 其余产品构造及 client/console | #77 | 保留当前产品行为；本轮只更新直接消费者和资格原因，不提前收窄整个产品快照。#75/#76 不重写公共 DTO/客户端字段。 |
| `service_construction.go`、`service_checkpoint.go`、`application_config.go`、`native_project_open.go`、state preflight/宿主、`steerlineapp/app.go` | #80（最终迁移） | 当前代码已经按新端口构造且可运行。#76 需要同步自身新类型时为唯一先行集成者；#80 仅接剩余宿主/组装。 |
| 新 `internal/execution/model/*` 与 backend 定义 | #76（定义拥有者；资源组归 #78 后继） | #75 仅依赖 task 小端口，不消费执行类型。 |
| 新 `storage/ports/task_*.go`、sqlite 三个提交、`persistence/ports.go`、`write_ports.go` | #76（#75/#76 并行期间唯一集成）→ #79 | #75 只改 task-only 声明/读取类型；共用存储契约先由定义拥有者顺序集成，禁止另造 SQL 实现或旧回调。 |

表中的单点集成者允许为已发布接口同步构造签名，但不承担另一条线的实现。#75 的 task 构造、小适配器、CLI 和 runner 范围与 #76 的执行构造/actor 没有同时改同一文件的要求。业务输入发生共同语义变化时先完成唯一接口定义，再迁消费者。

## 字段逐项迁移归属

实际共同声明文件的集成者为 #76；下列“迁移归属”是最终职责的后继，不授予并行改共同结构体的权限。

| 声明 | 实际定义文件 | 迁移归属 |
| --- | --- | --- |
| `Config.Persistence` | `internal/steerline/application_config_types.go` | #80 |
| `Config.ExecutionDirectory` | `internal/steerline/application_config_types.go` | #80 |
| `Config.ProcessHost` | `internal/steerline/application_config_types.go` | #80 |
| `Config.WorkspaceBranchPrefix` | `internal/steerline/application_config_types.go` | #76 |
| `Config.StateRoot` | `internal/steerline/application_config_types.go` | #80 |
| `Config.RepositoryStateKey` | `internal/steerline/application_config_types.go` | #80 |
| `Config.fixedProfileDigest` | `internal/steerline/application_config_types.go` | #76 |
| `Config.frozenExecutionConfig` | `internal/steerline/application_config_types.go` | #76 |
| `Config.requirementsEvaluation` | `internal/steerline/application_config_types.go` | #76 |
| `Config.executionConfigPath` | `internal/steerline/application_config_types.go` | #80 |
| `Config.publicationRemoteURL` | `internal/steerline/application_config_types.go` | #76 |
| `Config.selectedProfileID` | `internal/steerline/application_config_types.go` | #76 |
| `Config.PublicationPolicy` | `internal/steerline/application_config_types.go` | #76 |
| `Config.ExecutionPolicy` | `internal/steerline/application_config_types.go` | #76 |
| `Config.ExecutionRequirements` | `internal/steerline/application_config_types.go` | #76 |
| `Config.policySource` | `internal/steerline/application_config_types.go` | #80 |
| `Config.Repository` | `internal/steerline/application_config_types.go` | #80 |
| `Config.TaskAuthority` | `internal/steerline/application_config_types.go` | #75 |
| `Config.Remote` | `internal/steerline/application_config_types.go` | #80 |
| `Config.Branch` | `internal/steerline/application_config_types.go` | #80 |
| `Config.RunDir` | `internal/steerline/application_config_types.go` | #80 |
| `Config.WorktreeRoot` | `internal/steerline/application_config_types.go` | #80 |
| `Config.Claimant` | `internal/steerline/application_config_types.go` | #76 |
| `Config.Git` | `internal/steerline/application_config_types.go` | #76 |
| `Config.ImplementationData` | `internal/steerline/application_config_types.go` | #76 |
| `Config.ImplementationDisplay` | `internal/steerline/application_config_types.go` | #76 |
| `Config.WorkflowProtocol` | `internal/steerline/application_config_types.go` | #76 |
| `Config.ExecutableTypes` | `internal/steerline/application_config_types.go` | #76 |
| `Config.TaskPoll` | `internal/steerline/application_config_types.go` | #76 |
| `Config.PRPoll` | `internal/steerline/application_config_types.go` | #76 |
| `Config.Heartbeat` | `internal/steerline/application_config_types.go` | #80 |
| `Config.CommandTimeout` | `internal/steerline/application_config_types.go` | #76 |
| `Config.ShutdownGrace` | `internal/steerline/application_config_types.go` | #76 |
| `Config.Agent` | `internal/steerline/application_config_types.go` | #76 |
| `Config.Host` | `internal/steerline/application_config_types.go` | #76 |
| `Config.ExecutionProfiles` | `internal/steerline/application_config_types.go` | #76 |
| `Config.DefaultExecutionProfile` | `internal/steerline/application_config_types.go` | #76 |
| `Config.StallTimeout` | `internal/steerline/application_config_types.go` | #76 |
| `Config.PR` | `internal/steerline/application_config_types.go` | #76 |
| `Config.identity` | `internal/steerline/application_config_types.go` | #80 |
| `Config.workerEnv` | `internal/steerline/application_config_types.go` | #76 |
| `Config.configDigest` | `internal/steerline/application_config_types.go` | #80 |
| `Config.globalExecution` | `internal/steerline/application_config_types.go` | #80 |
| `CommandService.Store` | `internal/steerline/command_boundary.go` | #76 |
| `CommandService.Project` | `internal/steerline/command_boundary.go` | #76 |
| `CommandService.Work` | `internal/steerline/command_boundary.go` | #76 |
| `CommandService.taskResults` | `internal/steerline/command_boundary.go` | #76 |
| `Service.taskWork` | `internal/steerline/service_state.go` | #76 |
| `Service.taskResults` | `internal/steerline/service_state.go` | #76 |
| `Service.Config` | `internal/steerline/service_state.go` | #80 |
| `Service.baseConfig` | `internal/steerline/service_state.go` | #80 |
| `Service.lifecycleResolver` | `internal/steerline/service_state.go` | #76 |
| `Service.conversationSources` | `internal/steerline/service_state.go` | #76 |
| `Service.dispatchConfig` | `internal/steerline/service_state.go` | #76 |
| `Service.dispatchProfile` | `internal/steerline/service_state.go` | #76 |
| `Service.State` | `internal/steerline/service_state.go` | #76 |
| `Service.domain` | `internal/steerline/service_state.go` | #76 |
| `Service.committed` | `internal/steerline/service_state.go` | #76 |
| `Service.runs` | `internal/steerline/service_state.go` | #76 |
| `Service.pendingEvents` | `internal/steerline/service_state.go` | #76 |
| `Service.pendingOperations` | `internal/steerline/service_state.go` | #76 |
| `Service.workerDecisionWrites` | `internal/steerline/service_state.go` | #76 |
| `Service.store` | `internal/steerline/service_state.go` | #76 |
| `Service.domainStore` | `internal/steerline/service_state.go` | #80 |
| `Service.commandStore` | `internal/steerline/service_state.go` | #80 |
| `Service.conversationStore` | `internal/steerline/service_state.go` | #80 |
| `Service.closeStore` | `internal/steerline/service_state.go` | #80 |
| `Service.workers` | `internal/steerline/service_state.go` | #76 |
| `Service.ownedProcesses` | `internal/steerline/service_state.go` | #76 |
| `Service.lifecycleEffects` | `internal/steerline/service_state.go` | #76 |
| `Service.lifecycleMessages` | `internal/steerline/service_state.go` | #76 |
| `Service.appServerRecovering` | `internal/steerline/service_state.go` | #76 |
| `Service.workerMessages` | `internal/steerline/service_state.go` | #76 |
| `Service.runContext` | `internal/steerline/service_state.go` | #76 |
| `Service.forwarders` | `internal/steerline/service_state.go` | #76 |
| `Service.forwardMu` | `internal/steerline/service_state.go` | #76 |
| `Service.forwardedDone` | `internal/steerline/service_state.go` | #76 |
| `Service.conversationReplayRequests` | `internal/steerline/service_state.go` | #76 |
| `Service.conversationReplayErrors` | `internal/steerline/service_state.go` | #76 |
| `Service.Console` | `internal/steerline/service_state.go` | #77 |
| `Service.consoleOut` | `internal/steerline/service_state.go` | #77 |
| `Service.consoleSequence` | `internal/steerline/service_state.go` | #77 |
| `Service.controlSeen` | `internal/steerline/service_state.go` | #80 |
| `Service.stopControl` | `internal/steerline/service_state.go` | #80 |
| `Service.stopAcceptedAt` | `internal/steerline/service_state.go` | #80 |
| `Service.targetTask` | `internal/steerline/service_state.go` | #76 |
| `Service.lastDispatched` | `internal/steerline/service_state.go` | #76 |
| `Service.reservedRunID` | `internal/steerline/service_state.go` | #76 |
| `Service.candidateSnapshot` | `internal/steerline/service_state.go` | #77 |
| `Service.explicitDispatch` | `internal/steerline/service_state.go` | #76 |
| `Service.controlFault` | `internal/steerline/service_state.go` | #80 |
| `Service.startupID` | `internal/steerline/service_state.go` | #80 |

## 方法与文件逐项归属

所有 Service/CommandService/Config receiver 方法均列入 JSON；以下按当前文件列出完整方法名。已有非 receiver 入口的所有者由上面的文件表规定。

| 实际文件 | 字段/方法迁移归属 | 全部 receiver 方法 |
| --- | --- | --- |
| `internal/steerline/application_config.go` | #80 | `Config.LoadedConfigDigest`、`Config.ValidateApplicationPaths`、`Config.BindApplicationConfig`、`Config.Identity`、`Config.ResolveIdentity` |
| `internal/steerline/candidate_control.go` | #76 | `Service.processCandidateControl`、`Service.replayCandidateControl`、`Service.pollCandidateControlReceipts` |
| `internal/steerline/candidate_pr.go` | #76 | `Service.pollRetiredCandidatePRs` |
| `internal/steerline/candidate_resources.go` | #76 | `Service.applyRetiredLifecycleReceipt`、`Service.recordRetiredWorkerEvent`、`Service.finishRetiredCandidate`、`Service.resumeRetiredCandidate` |
| `internal/steerline/candidate_workers.go` | #76 | `CommandService.AbandonNativeCandidate`、`Service.candidateBusinessClosed`、`CommandService.recordCandidateMergeConflict` |
| `internal/steerline/command_boundary.go` | #76 | `Service.commandService` |
| `internal/steerline/command_processor.go` | #76 | `Service.processPendingCommands`、`Service.findDecision`、`Service.processCommandsAndDecisions` |
| `internal/steerline/command_submission.go` | #77 | `CommandService.Submit`、`CommandService.Pending`、`CommandService.Complete`、`CommandService.SetStatus` |
| `internal/steerline/control_service.go` | #76 | `Service.writeReceipt`、`Service.baseReceipt`、`Service.rejectControl`、`Service.finishControl`、`Service.controlDirectories`、`Service.nextControlRequest`、`Service.processNextControl`、`Service.pollControl`、`Service.retainControlPublicationFailure`、`Service.validateReadyTarget`、`Service.applyControlAction`、`Service.recoverControlOperation`、`Service.captureStopControl`、`Service.finalizeStopControl`、`Service.ensureStopApplyingReceipt` |
| `internal/steerline/conversation_replay.go` | #76 | `Service.conversationJournalSources`、`Service.runConversationReplayQueue`、`Service.reconcileConversationRun`、`Service.replayConversationJournal`、`Service.replayConversationTail`、`Service.replayConversationTailGap` |
| `internal/steerline/conversation_service.go` | #76 | `Service.recordConversationEvent` |
| `internal/steerline/conversation_sources.go` | #76 | `Service.discoverConversationHistory`、`Service.collectConversationHistoryPage`、`Service.runConversationHistoryCollector` |
| `internal/steerline/decision.go` | #76 | `Service.requireDecisionReplyCapability`、`Service.applyDecisions` |
| `internal/steerline/execution_backend.go` | #76 | `Service.requireBackendCapabilitiesFor`、`Service.readRunWorkspace`、`Service.inspectRunProcess` |
| `internal/steerline/execution_commands.go` | #76 | `CommandService.AnswerDecision`、`CommandService.AnswerDecisionWithID`、`CommandService.AcceptAnswer` |
| `internal/steerline/execution_config.go` | #76 | `Config.ValidateProfiles`、`Config.hostName`、`Config.agentName`、`Config.Preflight`、`Config.preflightNative` |
| `internal/steerline/execution_delivery_poll.go` | #76 | `Service.nativeCompleteMergedTask`、`Service.pollNativePRs` |
| `internal/steerline/execution_directory.go` | #76 | `Service.workerFlowRegistration` |
| `internal/steerline/execution_discard_cleanup.go` | #76 | `Service.reconcileNativeCleanup`、`Service.reconcileNativePRCleanup`、`Service.reconcileNativeWorkspaceCleanup`、`Service.recordNativeStopCleanup`、`Service.reconcileExitedNativeExecution` |
| `internal/steerline/execution_dispatch.go` | #76 | `Service.dispatchNative` |
| `internal/steerline/execution_helpers.go` | #76 | `Service.workflowRecords`、`Service.workflowAttempt`、`Service.inspectWorkflowWorkspace`、`Service.executionRecord` |
| `internal/steerline/execution_profiles.go` | #76 | `Config.CheckFixedProfile`、`Config.ApplyFixedExecutionProfile`、`Config.bindProjectPolicy`、`Config.acceptedConfigVersion`、`Config.projectExecutionProfile`、`Config.AvailableExecutionProfiles`、`Config.DefaultProfileID`、`Config.ResolveExecutionProfile`、`Config.ApplyExecutionProfile` |
| `internal/steerline/execution_requirements.go` | #76 | `Config.EvaluateRequirements`、`Service.checkCurrentExecutionRequirements`、`Service.resumeRegisteredWorkerFlow` |
| `internal/steerline/lifecycle_effect.go` | #76 | `Service.lifecycleEffectPending`、`Service.dispatchLifecycleEffect`、`Service.applyLifecycleEffect` |
| `internal/steerline/lifecycle_receipt.go` | #76 | `Service.newLifecycleOperation`、`Service.originalLifecycleOperation`、`Service.checkedWorkerTurnReceipt` |
| `internal/steerline/native_batch_command.go` | #75 | `CommandService.ValidateTaskBatch`、`CommandService.CreateTaskBatch` |
| `internal/steerline/native_claim_command.go` | #76 | `CommandService.StartNativeCandidate`、`CommandService.ClaimNativeTask` |
| `internal/steerline/native_cleanup.go` | #76 | `CommandService.RecordNativeWorkspaceCleanupProgress`、`CommandService.RecordNativeCleanupReceipt` |
| `internal/steerline/native_cleanup_stop.go` | #76 | `Service.reconcileRunStop`、`Service.stopRunExecution`、`Service.confirmExecutionStop` |
| `internal/steerline/native_delivery_command.go` | #76 | `CommandService.CompleteNativeTask`、`CommandService.recordDiscardedMergeConflict`、`CommandService.commitNativeDelivery` |
| `internal/steerline/native_discard_command.go` | #76 | `CommandService.DiscardNativeTask`、`CommandService.commitDiscard` |
| `internal/steerline/native_dispatch_command.go` | #76 | `CommandService.SetNativeDispatchState` |
| `internal/steerline/native_task_predecessors.go` | #75 | `CommandService.AddNativeTaskPredecessors` |
| `internal/steerline/operation.go` | #76 | `Service.setOperationState`、`Service.operationByKey` |
| `internal/steerline/profile_data.go` | #76 | `Config.DecodeImplementationConfig` |
| `internal/steerline/publication_snapshot.go` | #76 | `Config.publicationSettings`、`Config.bindPublicationTarget` |
| `internal/steerline/recovery.go` | #76 | `Service.inspectRecoveryClaim` |
| `internal/steerline/runtime_view.go` | #77 | `Service.workflowRecord`、`Service.refreshRun`、`Service.setRunDecision`、`Service.decisionRecord`、`Service.setDecisionStatus`、`Service.markWorkerDecisionWrite` |
| `internal/steerline/service_actor.go` | #76 | `Service.find`、`Service.findExecution`、`Service.reconcile`、`Service.setError`、`Service.dispatch`、`Service.injectControlFault`、`Service.activeControlRequestID`、`Service.slotReason`、`Service.executionSlotReason`、`Service.refreshSlotOccupancy`、`Service.refreshCandidates`、`Service.finishedForRun`、`Service.pollPRs`、`Service.pollPRsFor`、`Service.eventForRun`、`Service.recordHistoricalWorkerEvent`、`Service.finishWorkerInLoop` |
| `internal/steerline/service_checkpoint.go` | #80 | `Service.publishCheckpoint` |
| `internal/steerline/service_client.go` | #80 | `Service.publishStartupReady` |
| `internal/steerline/service_commit.go` | #76 | `Service.projectState`、`Service.projectActor`、`Service.commitWorker`、`Service.commitWorkerAttached`、`Service.discardWorkerCandidates`、`Service.commitDeclared`、`Service.commitEvents`、`Service.stageWorkerOperation`、`Service.changeWorkerOperation`、`Service.startWorkerOperation` |
| `internal/steerline/service_console.go` | #77 | `Service.note`、`Service.recordActivity`、`Service.emitConsole` |
| `internal/steerline/service_construction.go` | #80 | `Service.Close` |
| `internal/steerline/service_execution_profiles.go` | #76 | `Service.projectExecutionConfig`、`Service.selectDispatchProfile`、`Service.selectDefaultDispatchProfile`、`Service.executionConfigForRun`、`Service.executionConfigForWorkflowID` |
| `internal/steerline/service_loop.go` | #76 | `Service.Run`、`Service.RunWithDispatch`、`Service.run`、`Service.shutdown` |
| `internal/steerline/service_task_snapshot.go` | #76 | `Service.refreshNativeSnapshot` |
| `internal/steerline/service_workers.go` | #76 | `Service.registerWorker`、`Service.forwardWorker`、`Service.drainWorkerMessages`、`Service.handleWorkerMessage` |
| `internal/steerline/worker_attachment_store.go` | #76 | `Service.commitWorkerBinding`、`Service.commitExecutionOperation` |
| `internal/steerline/worker_completion.go` | #76 | `Service.finishWorkerFlowFor` |
| `internal/steerline/worker_environment.go` | #76 | `Service.environmentTurn`、`Service.blockWorkerEnvironment`、`Service.rejectUnsentBlockedAnswer`、`Service.observeWorkerEnvironment`、`Service.advanceWorkerPreparation`、`Service.reconcileEnvironmentBusinessIntent`、`Service.finishWorkerPreparation`、`Service.applyEnvironmentAction` |
| `internal/steerline/worker_environment_host.go` | #76 | `Service.hostEnvironmentForRun`、`Service.currentHostTurn`、`Service.resetHostTurnOutput`、`Service.executeHostAction`、`Service.readHostOutput`、`Service.verifyHostExecutionEvidence` |
| `internal/steerline/worker_events.go` | #76 | `Service.recordWorkerTurnStarted`、`Service.eventWorkerFlowFor`、`Service.routedQuestionSeen` |
| `internal/steerline/worker_execution.go` | #76 | `Service.workerFlowContext`、`Service.deliverResumableWorkerFeedback` |
| `internal/steerline/worker_feedback.go` | #76 | `Service.waitWorkerFlow`、`Service.applyWorkerFlowDecision` |
| `internal/steerline/worker_lifecycle.go` | #76 | `Service.acceptStartedWorkerTurn`、`Service.lifecycleForRun`、`Service.resumeLifecycleWorkerFlow`、`Service.resumeLifecycleObservation`、`Service.reconcilePendingLifecycleSteps`、`Service.lifecycleSnapshot`、`Service.validateLifecycleResultBinding`、`Service.originalLifecycleRequest`、`Service.startWorkerLifecycle`、`Service.interruptOwnedLocalTurn`、`Service.executeLifecycleStep`、`Service.applyLifecycleStepReceipt`、`Service.prepareLifecycleResources`、`Service.cleanupLifecycleResources`、`Service.acknowledgeLifecycleDelivery`、`Service.applyAppServerSessionReceipt`、`Service.applyAppServerTurnReceipt`、`Service.deliverLifecycleActiveFeedback`、`Service.dispatchWorkerTurnEffect` |
| `internal/steerline/worker_lifecycle_feedback.go` | #76 | `Service.deliverLifecycleQuestionAnswer`、`Service.reconcileLifecycleQuestionAnswer`、`Service.reconcileSavedLifecycleQuestionAnswer`、`Service.applyLifecycleQuestionAnswerReceipt`、`Service.confirmLifecycleQuestionAnswer` |
| `internal/steerline/worker_lifecycle_recovery.go` | #76 | `Service.resumeAppServerWorkers`、`Service.applyAppServerRecovery` |
| `internal/steerline/worker_pr.go` | #76 | `Service.offerWorkerPRFeedback`、`Service.pollWorkerFlowPR` |
| `internal/steerline/worker_recovery.go` | #76 | `Service.hasUnknownWorkerOperation`、`Service.beginLocalRecoveryAttempt`、`Service.refreshSavedRecoveryDecision`、`Service.startManuallyRecoveredInitialWorker`、`Service.adoptOrGateServiceExecutions`、`Service.markServiceRecoveryGateForPriorOwner`、`Service.markServiceRecoveryGate`、`Service.markOneServiceRecoveryGate`、`Service.resumeConfiguredWork`、`Service.resumeWorkerFlow` |
| `internal/steerline/worker_resume.go` | #76 | `Service.resumeWorkerWorkspace` |
| `internal/steerline/worker_start.go` | #76 | `Service.prepareWorkerFlow`、`Service.prepareWorkerWorkspace`、`Service.startInitialWorkerFlowTurn`、`Service.startWorkerFlowTurn` |
| `internal/steerline/worker_state.go` | #76 | `Service.stopVerifiedWorkerTurn`、`Service.resolveStoppedWorkerTurn` |
| `internal/steerline/workspace_config.go` | #76 | `Config.prepareWorktreeRoot` |
