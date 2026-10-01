# Plan 执行配置与原生绑定所有权

任务 ID：63；任务名称：执行配置与原生绑定所有权；目标：`CORE-IMPLEMENTATION-DECOUPLING`。唯一执行依据：[Spec](core-decoupling-02-bindings.spec.md)及[共同约束](core-decoupling-constraints.md)。硬前置：#62 整体交付；一个实现 PR。

## 实施顺序

1. 建立 Config/Profile/ExecutionRecord/Decision 的字段到实际消费者清单，区分业务权威与原生状态，约定封套 key/版本/归属/摘要/大小。
2. 从 #62 的路由 codec 扩到配置和绑定；领取前配置绑定 Project/Profile/受理版本，领取后资源/路由绑定 Execution/代次。冻结用途、64 KiB 上限和整份摘要，迁原生定义/默认/规范化/权限解释。
3. 串行更新 Claim、有效发布责任解析（含项目授权 execution_profile）、快照、绑定附件、恢复与查询展示，深复制并更新格式拒绝；持久/冻结/查询消费者当场切换并删除旧字段解释；旧生命周期/采集只通过唯一 codec 的临时解码视图读写新封套，列方法清单交 #64/#65 删除，绝不双写原字段。
4. 审查共享持久记录及纯包依赖，核对全部旧到新 fixture 映射，运行最小冻结与恢复验证。

## 所有者与删除

`config.go`、`execution_profiles.go`、`execution_snapshot.go`、`service_execution_profiles.go`、`execution_capability.go`、`execution_backend.go`、`codex_git_sandbox.go`、`orca.go`/`orca_remote_adapter.go` 的具体配置转换、contracts/execution.go、records.go、runtime.go 及对应适配器绑定。

删除 root 的具体配置别名、旧持久 Profile/Record 字段消费及权限/绑定解码 helper；原生类型和校验归唯一实现，配置/持久/冻结/查询消费者只用通用封套和投影。旧生命周期/采集通过 codec 临时视图消费新封套的有限方法清单明确交 #64/#65 删除。#62 的问题路由 codec 统一接入当前封套规则。

## 验证与命中核对

受影响 codec 纯测试加一组中心冻结/恢复/事务接线。复用 TestProjectPolicyFrozenSnapshotRejectsDamageAndPreservesAuthority、TestClaimRechecksAcceptedConfigFiles、TestWorkerFlowContextsDoNotExchangeExecutionProfiles、TestResolutionAudit 系列中与本轮字段相关的断言及 remote Claim 拒绝，不重新认证全部平台。

实施时先记录所选旧用例的原位置、新位置及断言归属。下列 CD 用例名称是本任务交付的验收入口：可以把现有断言迁入同名用例或用带子用例的入口组合，不能另跑一份相同矩阵。每个命名入口及约定的各实现包必须实际命中，不能仅以进程 exit=0 或其他包有测试判断通过；原有生产缺口才补断言。外层集成包可由生命周期阶段按实际消费者创建，不得导出任意中心私有写入口。

```powershell
go run ./cmd/test-feedback -quick -- ./internal/steerline ./internal/steerline/codexworker ./internal/steerline/orcaworker -count=1 -run '^(TestCD63CodecIdentityAndVersion|TestCD63FrozenConfigAndBinding)$'
go list -deps ./internal/steerline/contracts ./internal/steerline/execution
git diff --check
```

最终验证将定向组与 quick 合在一次现有入口中；execution/contracts/query/sqlite/remote 协议包已由 quick-rules 运行，不再重复单独跑整包。CD63 的 remote codec 用例由 quick-rules 命中；必须核对按包/用例的实际 JSON 测试事件。调试失败可以定向重跑，稳定后不重复矩阵。

Spec 的各项验收对应通用/原生 fixture、固定身份与提交屏障、依赖和删除审查；本轮结果和实际命中按下方实施记录登记。纯规则不跑平台矩阵，集成只跑受影响代表路径；quick 稳定后一次，没有具体失败不扩为全量。

## 本轮实施记录

通用 `ImplementationDataEnvelope` v1 统一承载配置、资源和 answer-purpose 问题路由，包含 codec/version、身份范围、64 KiB 有界 JSON 载荷和完整摘要；配置、资源、问题分别绑定 Project/Profile/受理版本、Execution/Attempt/generation/config digest、WorkflowRun/Execution/Attempt/generation/turn/question。#62 的 `QuestionRoute` 现在是该封套的用途视图，问题 codec 仍由对应实现严格解释。

Codex、Orca 本机和 Orca 远端配置及校验分别归属 `codexworker`、`orcaworker`、`orca_remote_worker`；Codex app-server 绑定和 permission resource payload 归 `codexworker`，Orca execution/turn 状态归 `orcaworker`。contracts 不再持久化 native 配置、app-server endpoint/session/generation、Orca state 或 `worker_session_id`；projection 不再导出 `codex_session_id`。根包 snapshot 只组装通用权限根和 codec 资源封套。

格式拒绝点：全局 `execution.json` v2、FixedExecutionProfile v5、`builtin-execution-v2`、PolicyResources v3、runtime checkpoint schema 12、SQLite schema 9。旧开发格式、旧资源快照和旧问题路由摘要不读取、不迁移、不回填；历史验收 JSON 保持原始内容供拒绝路径核验。

仍通过唯一 codec 临时读取新封套、待 #64/#65 删除的生命周期与采集消费者：

- 共享读取入口在 `implementation_bindings.go`：`decodeCodexAppServerBinding`、`decodeOrcaExecutionState`、`executionSessionFacts`、`codexSessionID`、`codexBinding`；持久化只调用对应 store helper 写回 envelope。
- #64 Codex lifecycle：`codex_worker.go` 的 `codexBinding`/`startAppServerWorkerTurn`，`worker_events.go` 的 `eventWorkerFlowFor`/`recordWorkerTurnStarted`/`validateQuestionRouteForRun`，`worker_state.go:workerFlowState`，`worker_start.go:startWorkerFlowTurn`，`worker_commit.go:commitWorker`，`worker_execution.go:acceptStartedWorkerTurn`/`deliverResumableWorkerFeedback`，`worker_resume.go:resumeNativeOrcaWorker`，`codex_worker_recovery.go` 与 `codex_worker_reply.go` 的恢复/回答调用，及 `native_cleanup.go`、`native_service.go`、`native_delivery_command.go`、`sqlite_store_write_execution.go`、`sqlite_store_write_native.go` 通过 `executionSessionFacts` 固定交付/写屏障。
- #64 Orca lifecycle：`orca_worker_prepare.go` 的准备/恢复/terminal binding，`orca_worker_dispatch.go` 的 dispatch/反馈/问题路由，`orca_worker_observation.go` 的 inbox/output/settlement，`orca_worker_settlement.go:readNativeOrcaSettlement`，`orca_worker_terminal.go:finishNativeOrcaCleanup`，`orca_worker_workspace.go:prepareRemoteWorkerWorkspace`/`recoverRemoteWorkerWorkspace`，`orca_remote_service.go` 的 prepare/start/resume/cleanup，`orca_execution_operations.go:nativeOrcaOperations`/`readNativeOrcaDelivery`，`worker_environment.go` 的 readiness/feedback/recovery，以及 `state_preflight.go:preflightWorkerFlowState`。
- #65 session collection：`codex_conversation.go:codexReaderCommand`/`recordCodexHistoryCapabilities` 继续通过实现配置 codec 取得 history reader 命令；来源发现、读取和 mapper 尚未搬迁。
- #66 final assembly：`config.go:preflightNative`/`needsCodexExecutable`/`managedWindowsOrca`、`execution_registry.go:init`、`execution_backend.go:localExecutionBackend.Probe`、`codex_worker.go:codexBackend`、`orca.go:nativeOrcaConfig`、`orca_remote_adapter.go:remoteWorkerConfig` 保留实现 codec 的唯一配置组装入口，后续随目录/工厂一起外移。

旧到新断言映射：`TestProjectPolicyFrozenSnapshotRejectsDamageAndPreservesAuthority` → `TestCD63FrozenConfigAndBinding/fixed_profile_integrity`；`TestClaimRechecksAcceptedConfigFiles` → `accepted_config_recheck`；`TestAppServerLaunchMessagePersistsPolicyResources` → `resource_receipt_commit/thread_ready` 与 `turn_result`；`TestWorkerFlowContextsDoNotExchangeExecutionProfiles` → `profile_copy_isolation`；`TestResolutionAuditRequirementsDoNotBlockOriginalObservation` → `original_observation_survives_requirement_drift`；`TestRemotePublicationClaimRequiresPermissionEvidence` → `remote_missing_evidence_still_blocks_claim`；历史权限报告的拒绝断言归 `legacy_permission_evidence_rejected`。#62 route 的既有 `TestQuestionRouteVersionIdentityDigestAndLimit`、Codex/Orca native question-route 用例现验证共享封套的版本、身份和摘要。

最终 quick JSON 报告：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261001T110142.429138300Z\summary.json`，`passed=true`、complete。实际 JSON 事件为 quick-core 1/1 包、6/6 tests；quick-rules 9/9 包、75/75 tests；quick-targeted 3/3 包、37/37 tests（根包 25、Codex 8、Orca 本机 4）。

命中明细：root `TestCD63FrozenConfigAndBinding` 实际包含 question route identity 与 receipt barrier、固定配置/资源/恢复/事务子例；`codexworker.TestCD63CodecIdentityAndVersion/shared_question_envelope` 与 `orcaworker.TestCD63CodecIdentityAndVersion/shared_question_envelope` 各自实际调用既有 #62 原生路由回归；quick-rules 实际命中 `execution.TestCD63CodecIdentityAndVersion`、`execution.TestQuestionRouteVersionIdentityDigestAndLimit` 及 `orca_remote_worker.TestCD63CodecIdentityAndVersion`。因此三个实现配置 codec、共同 envelope、question route、中心冻结接线和 remote profile codec 均有包/用例事件证据；没有依靠其他包的通过数推断命中。

`go test ./internal/steerline/console -run '^$' -count=1` 编译通过（0 tests 执行）；`go list -deps ./internal/steerline/contracts ./internal/steerline/execution` 通过，108 个依赖中没有具体实现包；`git diff --check` exit 0，只有工作树 LF→CRLF 规范化提示、无 whitespace finding。remote 缺权限证据的领取拒绝子例通过；未进行真实远端权限认证，也未触发 business dispatch。

## 质量审计补修

对实现基线 `4ba7d4db783c1a6f18ef12c999488e79f22e9d27` 的审计修复如下，均保持单写者与原身份约束：

- 封套摘要绑定规范化 JSON 字节，兼容同一当前格式的 compact/indent 输出及 HTML escaping；构造和读取都保存独立的规范化载荷。64 KiB 验证改用有效 JSON，分别确认精确上限与超限拒绝。
- 人工确认后原 thread 的新 Attempt seed 可在 process-free、pending/resuming 阶段完成资源绑定并通过重启预检；已绑定进程或非恢复状态丢失原 receipt 仍拒绝。Execution 的配置摘要须匹配 WorkflowRun 的完整固定快照。
- 重复绑定不再被临时 decoder 当作“缺失”；codec 必须匹配原协议，已绑定 app-server 不能只保存恢复 seed。原生 coordinator 须匹配持久 target/runtime，app-server endpoint 在连接前使用同一 loopback/port 校验。
- Orca 环境重检查找回 Dispatch 或清空 output cursor 后写回新封套，重新轮询后重新读取临时 turn view；避免确认操作只落盘 receipt、丢失原绑定身份。
- native profile 的规范化、支持校验和非秘密展示投影全部归属实现包；根包 codec 目录只组装通用封套。remote Codex 同样执行 model 校验。
- 修正迁移后 fixture 的 native UUID、固定快照/受理摘要及实际 Git remote 接线；通用 workerflow 的 opaque session 断言仍保留。

新增回归实际命中 `execution.TestCD63CodecIdentityAndVersion/formatted_JSON_round_trip`、root `TestCD63FrozenConfigAndBinding/recovery_seed_resources`、`binding_codec_isolation`（含 duplicate/protocol/seed/native-target 子例）、`environment_recheck_bindings` 和 Codex `binding_endpoint_validated_before_connection`。原生传输的两个 `TestCodexSocket*`、配置副本的两个 worker-context 用例，以及受影响 Task CAS/前置领取/活跃丢弃/精确 PR 丢弃和 worker delivery gate 用例定向通过；未扩展为全量测试或原生认证矩阵。

## 验收映射与 PR 交付清单

| Spec 验收 | 入口/产物 | 必须核对的实际结果 |
| --- | --- | --- |
| CD63-1 | TestCD63CodecIdentityAndVersion | 三个实现 codec 用同一组必要身份/版本/摘要断言；损坏数据拒绝，不能重算自身摘要逃过整份快照。 |
| CD63-2 | TestCD63FrozenConfigAndBinding | 真实 Claim/绑定/恢复链固定受理版本、有效发布责任与资源；公开对象复制、当前配置变更及缺证 remote 拒绝。 |
| CD63-3 | 源码与依赖审查 | 根包和 contracts 无原生配置/绑定定义或 JSON 字段解释，原持久字段与校验桥删除；只对尚未迁移的生命周期/采集列有限临时消费者清单，不要求提前完成 #64/#65。 |

PR 附本阶段消费者/所有者和删除清单、版本变化、旧到新测试路径映射及实际命中；未运行的原生证据单独列限制。下游只能接收已实际接通的契约，不以空包、双路或仅声明接口替代交付。

## 交接

#64 获得可恢复的规范化配置、原绑定、通用会话/轮次和小型 codec 目录；具体构造仍留唯一组装入口供 #66 接管。

格式改变明确更新版本并拒绝旧开发格式；不加迁移/双写/fallback。保留原请求、资源身份、深复制及单写者事务。PR 和验证记录使用实际行为与支持限制，不能把结构迁移算作 remote 权限证据补齐。
