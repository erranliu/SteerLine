# Plan Codex 执行适配器迁移

任务 ID：57；任务名称：Codex 执行适配器迁移；目标：`EXECUTION-ISOLATION`。状态：待实施。唯一执行依据：[Spec](execution-isolation-04-codex.spec.md)。硬前置：#55、#56 合入，一个实现 PR 交付。

## 迁移步骤

1. 接收工作区资源和发布边界，逐项列出中心 Codex I/O、证据解析、业务写入和人工授权判断。保持当前进程承载/Job Object 策略，不顺带修正服务生命周期。
2. 在 codexworker 建立固定配置与 backend 实现，先迁参数/命令和证据，再迁 prepare/start/answer/observe/stop。将已有 LocalSandboxArgs 的设置决策移到权限编译层，保持 local elevated 默认及 unelevated 分散根拒绝。复用 codexapp 原生客户端和原生事件解析，必要 native payload 不暴露到业务消费者。
3. 沿现有 worker_start/worker_mode 启动协调拆同步和异步接线：中心提交意图，平台返回现场句柄与回执，中心通过原子附件保存后登记 observer；失败按原操作记录未知并取消未登记现场。禁止包内直接 commitWorker 或复制另一个协调循环。
4. 将恢复证据读取与执行性恢复拆开，中心先核对用户授权和代次。迁移实时/重放解析到同一入口，删除中心 method、JSON 和 Codex 日志格式解释。
5. 提供固定 CLI 启动命令给 #58，迁移必要测试；删去中心旧原生操作，保留小的请求/回执转换与事务接线。

## 验证

```powershell
go test ./internal/steerline/codexworker ./internal/steerline/codexapp -count=1
go list -deps ./internal/steerline/codexworker
go test ./internal/steerline -count=1 -run '^(TestAppServerServiceTwoTurnsPersistNativeReceipts|TestAppServerAnswerEvidenceRequiresMatchingNativeContent|TestAppServerLateReplyCannotModifyTerminalTask|TestRestartDoesNotSendPreviouslyAcceptedFeedback|TestWorkerIntentIsDurableBeforeDispatchAndFailurePreventsDispatch|TestWorkerReceiptFailureRetainsIntentAndCancelsUnregisteredObserver|TestLocalWindowsSandboxProfileFreezesAndRestores)$'
```

CX1 用传递依赖/调用审查；CX2 用两轮和答案/旧回执；CX3 用人工门槛与意图/回执提交失败；CX4 用 argv/参数 fixture 和 Windows 默认/拒绝/冻结回归。LocalSandboxArgs 纯测试迁移后从原 codexapp 包筛选移除，记录其新路径。只补缺少的恢复输入授权或命令接线断言，不跑真实 agent 或全量矩阵。

## 交付

列出每类平台调用、业务提交和原生证据的最终所有者，交接配置、固定命令、事件与未知请求对账给 #58。本轮原生消费者必须全部迁完，不能只靠文件改名或给中心调用追加 helper 宣告完成。
