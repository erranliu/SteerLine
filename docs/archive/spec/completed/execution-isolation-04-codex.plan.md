# Plan Codex 执行适配器迁移

任务 ID：57；任务名称：Codex 执行适配器迁移；目标：`EXECUTION-ISOLATION`。状态：已合入 PR #170。唯一执行依据：[Spec](execution-isolation-04-codex.spec.md)。硬前置：#55、#56 合入，一个实现 PR 交付。

## 迁移步骤

1. 接收工作区资源和发布边界，逐项列出中心 Codex I/O、证据解析、业务写入和人工授权判断。保持当前进程承载/Job Object 策略，不顺带修正服务生命周期。
   复用 contracts 的 Command/CodexAskCall/固定记录与 execution 的身份/问题/动作；保持 contracts → execution、不引入反向依赖。远程原生 worker-start 不迁成中心命令，保留独立协议。
2. 在 codexworker 建立固定配置与 backend，先迁参数/命令和证据，再迁执行操作。迁入 LocalSandboxArgs 与 Orca 的 Codex 参数/ValidateManagedArgs，分别保留 local Windows 规则和 Orca 无 model 受管、独立 effort、strict-config/排除临时根；shell 包装不搬入 Codex。复用原客户端和解析。
3. 沿现有 worker_start/worker_mode 启动协调拆同步和异步接线：中心提交意图，平台返回现场句柄与回执，中心通过原子附件保存后登记 observer；失败按原操作记录未知并取消未登记现场。禁止包内直接 commitWorker 或复制另一个协调循环。
4. 将恢复证据读取与执行性恢复拆开，中心先核对用户授权和代次。迁移实时/重放解析到同一入口，删除中心 method、JSON 和 Codex 日志格式解释。
5. 提供固定 CLI 启动命令给 #58，迁移必要测试；删去中心旧原生操作，保留小的请求/回执转换与事务接线。

## 验证

```powershell
go test ./internal/steerline/codexworker ./internal/steerline/codexapp -count=1
go list -deps ./internal/steerline/codexworker
go test ./internal/steerline -count=1 -run '^(TestAppServerServiceTwoTurnsPersistNativeReceipts|TestAppServerAnswerEvidenceRequiresMatchingNativeContent|TestAppServerLateReplyCannotModifyTerminalTask|TestRestartDoesNotSendPreviouslyAcceptedFeedback|TestWorkerIntentIsDurableBeforeDispatchAndFailurePreventsDispatch|TestWorkerReceiptFailureRetainsIntentAndCancelsUnregisteredObserver|TestLocalWindowsSandboxProfileFreezesAndRestores)$'
```

CX1 用传递依赖/调用审查；CX2 用两轮和答案/旧回执；CX3 用人工门槛与意图/回执提交失败；CX4 迁移 LocalSandboxArgs 与 `TestManagedOrcaLaunchFreezesPermissionsAndRejectsArgs`，保留有/无 model、独立 effort、排除隐式根和摘要对照，删原重复测试并记录新包路径。只补缺少接线，不跑真机或全量矩阵。

## 交付

列出每类平台调用、业务提交和原生证据的最终所有者，交接配置、固定命令、事件与未知请求对账给 #58。本轮原生消费者必须全部迁完，不能只靠文件改名或给中心调用追加 helper 宣告完成。

## 实施记录

最终所有者、迁移测试路径、实际最小验证及基线剩余项见[验收记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/execution-isolation-04-codex/README.md)。

## 测试计划（2026-10-07 导航维护）

本次仅将已清理的证据导航改为固定 Git 快照，不改原实现契约或追认历史通过。最小范围为文档静态 `doc-links`、`task-identities`、`whitespace`：当前 Windows/Git/Python 3 环境，交付前执行 `python scripts/check_doc_links.py`、`python scripts/roadmap_task_id.py check`、`git diff --check`；通过条件为无缺失本地链接、任务身份一致及无空白错误。固定历史链接的目标另用 `git cat-file --batch-check` 核对存在；结果见[本次清理验证](../../../acceptance/history.md#测试计划与本次结果)。不选择 L1/L2/L3、Full 或原生模型组；导航变更不需要这些验证，不新增场景或环境能力。
