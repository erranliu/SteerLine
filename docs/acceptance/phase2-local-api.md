# 任务 #45 验收记录：本机客户端协议与历史查询

唯一归属 #45；固定范围依据 Spec/Plan `f628161b5a60cefeb75922c15a63b1df9ef9c21b` 的 `docs/spec/phase2-local-api.spec.md` 与 `docs/spec/phase2-local-api.plan.md`，未改 Spec。Plan 的实现期补充只细化 API1–API5 的真实集成场景、命令与证据位置，不扩大通过条件。当前整合候选 `08341fbc90e975ed512e47740860f2d2e25edb1d`，tree `ba3e2837dd616ebc8789bbc2a56771e281a2216d`；其 API/HTTP 源码与验收 commit `99c98e2bd02b3c0f75fbdea4919ce54e59927745` 相同，额外合入 main `04c62ee5ddb10637a68745baa4fc4f811123ffa9` 的两份 console 文件。环境：Windows/amd64、Go 1.26.4；受控临时 Git/SQLite 和 Codex app-server，无 live 模型。记录日期：2026-10-09（Asia/Shanghai）。

## 自动验收

| 项目 | 结果 | 已覆盖行为与证据 |
| --- | --- | --- |
| API1 | 已验收 | 真实 listener 上缺/错凭证、错 Host/Origin、错路由项目/信封项目/实例及轮换后旧凭证均拒绝；原传输/control/SQLite 请求和 revision/paused facts 不变。轮换后的新凭证成功执行真实 pause/resume，控制 revision 只增加一次。非回环源由 loopback-only 绑定及 `TestAPIIdentityAndAuthentication` 中源地址拒绝断言覆盖。 |
| API2 | 验收失败 | 现有 `ProjectPresentation`/DefaultProfileID 和 unavailable summary 现已经 `GET /presentation` 暴露；真实客户端用发现的 profile ID 完成第二候选动作，任务/图/历史/双候选/活动/final 正文查询通过。旧 `TestAPIQueryParity` 的 SQLite `-shm` `Access is denied` 真实失败尚未能精确复现/归因，故保持失败状态；Windows sidecar 有界重查、持续 ACL 拒绝及真实 WAL open/close 竞争回归通过，当前真实 QueryParity 单次和 count=3 均通过。待新 reviewer 判断这组证据是否足以闭合历史故障，不做环境豁免。 |
| API3 | 已验收 | 真实 HTTP + app `ControlInboxPort` + service actor 覆盖 transport 已登记但发送前取消/同信封补交、正式请求已发布但响应丢失/重连 GET 与 retry、正式消费者迟到竞争、8 个并发 duplicate、异版本同 ID conflict。检查正式 request/receipt、原信封与真实 control revision；每个合法 request 只生效一次，外国 revision receipt 不覆盖原请求。round 2 首次 test 断言过严及调整记录见下。 |
| API4 | 已验收 | 重连回到同一服务实例；unsupported 保留为 rejected 回执；未知、SQLite 读取错误和 control 回执读取错误保持不同事实。定向和真实服务 `TestAPIReconnectAndUnsupported` 通过。 |
| API5 | 已验收 | 真实应用服务恢复后发布发现/ready 身份、正常关闭与端口冲突诊断通过；配置服务路径不启动替代实例。定向 `TestAPIServiceLifecycle` 与真实服务 `TestAPIServiceLifecycle`、`TestAPIConfiguredServicePublishesDiscovery` 通过。 |

无独立人工验收门槛；真实 UI 操作仍由 2.1 节点安排，不计入本任务自动结果。

## round 1 独立复审确认

复审 head `1d99790d485c029180316c8eaceebd1ed5c6a313`，base `f628161b5a60cefeb75922c15a63b1df9ef9c21b`。临时 overlay `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-review-a7b19c70799a414b94ebe42d296c86ae` 复现两项失败：同实例 start_task 的原 target=8/version=3/control_revision=7 被实际 target=9/version=55/control_revision=99 回执掩盖；合法临时文件在保护 DACL 设置前使并发 mutation preflight 失败。局部修复与复验待登记；profiles 查询、真实 API1 mutation/API3 fault coverage 由原执行者续补。旧证据保留，不代表上述遗漏已通过。

## 候选提交验证

所有产品验证针对精确实现候选 `c18acecc195d294c4b2e7250ad2075224e73bdfa`（tree `c1f9ca9c42bed5ada307b6f6b83b7e55836a98a7`）。

| 范围 | 命令/结果 | 原始证据 |
| --- | --- | --- |
| API1/API2/API3/API4/API5 定向协议 | `go test ./internal/product/httpapi -run '^TestAPI(IdentityAndAuthentication\|QueryParity\|OriginalCommandReceipt\|ReconnectAndUnsupported\|StartTaskAndCandidateVersionMapping\|AnswerDecisionReceiptAndUniqueRequest\|ServiceLifecycle)$' -count=1 -v`：7 项通过。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-unit-7b9f9fd323694a1c8f6b1205e8bf3501\api-targeted.log` |
| API3 原信封/回执 | `go test ./internal/product/httpapi -run '^TestAPIOriginalCommandReceipt$' -count=1 -v`：通过。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-command-35073eee7715459d9664ddacd8622f52\api-command.log` |
| 回执 DTO、绑定和版本检查 | query 定向 1 项、app 定向 3 项、execution 定向 3 项均通过；包含答案 Attempt/recovery 绑定、task version 重查及既有 ER3 dispatch 场景。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-query-c08eb7d4846546fa920136d10e36f5aa\query-targeted.log`；`C:\Users\lmas1\AppData\Local\Temp\steerline-api45-app-67b100f21a3e4b619f147ad4ade166db\app-targeted.log`；`C:\Users\lmas1\AppData\Local\Temp\steerline-api45-execution-02e22c66a0094c83ac030220e29634c4\execution-targeted.log` |
| API1–API5 真实服务接线 | `go test ./tests/integration -run '^TestAPI(IdentityAndAuthentication\|QueryParity\|OriginalCommandReceipt\|ReconnectAndUnsupported\|ServiceLifecycle\|ConfiguredServicePublishesDiscovery)$' -count=1 -v`：6 项通过，0 skip；总计 113.984 秒。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-suite-retry-0c22cb1f2d98439780a7c9b6771a4d19\api-integration.log` |
| L1 固定组 | `go run ./cmd/test-feedback -level=L1`：`complete=true`, `passed=true`, 12 组/427 项通过、0 skip，3 workers，Windows/amd64；runner 记录的 source commit 与候选一致。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-l1-181a14ee41924e95a577622980135b92\l1.log`；runner 原始摘要 `C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261008T200738.379733300Z\summary.json` |
| Go 静态 | `go vet ./internal/product/httpapi ./internal/app/...`：通过，无诊断。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-vet-0e035b53f98f474b932a2c352e995063\api-vet.log` |
| 文档/身份静态 | `python scripts/check_doc_links.py`：266 Markdown、1679 本地链接、0 错误；`python scripts/roadmap_task_id.py check`：84 identity files、通过；`git diff --check`：通过。 | 执行时命令结果；交付文档提交后重跑并记录于最终执行摘要。 |

## 保留的失败与范围

- 第一轮完整 API 集成组在 `TestAPIQueryParity` 的最终正文读取处失败：Windows 返回检查 SQLite `steerline.db-shm` 的 `Access is denied`。单独重跑该场景通过，随后完整六项集成组再次运行全部通过。两次原始日志均保留：失败轮 `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-final-9c9c68e27c7c452f9c9429df8a981c97\api-integration.log`；单项重跑 `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-query-retry-2ae16cc55b8b49518764dcd66f8057c3\query-parity.log`；通过轮见上表。失败原因未能稳定复现，根因尚未闭合，不能据重跑通过判为环境豁免；保留原失败并待诊断。
- 早先误跑的宽范围 `go test ./internal/app ./internal/execution` 在既有 `TestWorkerEnvironmentER3RecheckKeepsOriginalDispatch` 等待 `workspace.Git.Output` 时达到 10 分钟工具超时；该次未保留原始日志且不记通过。随后计划内单项该测试通过，L1 也完整通过。
- 未运行 Full、完整 L2/L3、live 模型或完整离线进程；它们不属于本任务的最小验证要求。

PR 与合并核验待登记；本记录只覆盖候选实现和上述自动验收，不宣称任务已合并或已实现完成。


## round 1 局部修复与精确复验

局部实现提交 `18442c4e233c745d42a1de32b286cb23a963b19a`，补齐首次注册窗口提交 `ac3c6411398da955b535cbc87df9fcfff4b8c9a8`（tree `2fff55b07d57707bb46b8688841ad3ec2b190e82`）。下面验证全部针对后者 Windows/amd64、Go 1.26.4 干净工作区；后续仅验收/执行图文档提交复用同一生产与测试源码。原始证据根：`C:/Users/lmas1/AppData/Local/Temp/steerline-api45-localfix-1a11d56aa02/ac3c6411`，`source.json` 与各命令的 `*.result.json` 固定 commit/命令/退出码。

- API3 局部 matcher：既有 transport requestRecord 首次持久唯一注册携带实际映射的 control/decision/discard intent；Decision 规范化通过现有 app/领域函数，无独立规则。回读校验 project/request/instance/action、目标/版本/control revision、profile/digest/graph、候选及完整环境 target、原 Decision/Attempt/recovery 和 choices/text/remarks。缺少可证明映射的权威不按 kind 推定相同；不同内容保持 identity_conflict。提交碰撞后只查询原权威，同内容返回已有请求，异内容拒绝，不重投。
- API1 局部 ACL：Windows CreateFile 的 SecurityAttributes 在创建时设置 current-user owner 和 protected 单用户 full-access DACL；atomic/exclusive 同一真实写入核心保持 write/sync/close/发布及最终严格核验。没有跳过未知/不安全文件，也没有放宽 DACL。
- `go test ./internal/product/httpapi -run '^TestAPI' -count=1 -v`：14 顶层测试通过、0 skip，3.512 秒；`httpapi.log`。正式 host immutable publisher 在 HTTP 已登记、原 submit 前由另一个消费者赢得同 ID：同内容 POST/GET/repeat 成功；合法同实例异 target=9/version=55/control_revision=99 三次都 identity_conflict，原 submit 调用数保持 1。另有 profile/digest/graph/instance 单字段冲突及动作 payload 绑定检查。SQLite answer/discard 通过真实 commands 表读取和正式 DTO mapper 验证同/异内容；answer 使用受控持久回执 fixture，未声明真实服务 fault 驱动完成。
- `TestAPIPrivateWritersConcurrentPreflight` 在两个 writer 分支的真实 temp 创建后、write/sync/close 前用阻塞 reader 保持文件打开，同时调用真实 `ValidateStateDirectory`：均成功，最终文件也通过。Windows `TestAPIPrivateTemporaryACLRejectsUnsafeEntry` 保留继承 DACL、Everyone 额外读取两种拒绝，正向创建时 ACL 通过。
- `go test ./internal/app -run '^Test(LocalAPIDecisionNormalizationUsesOriginalQuestions|BoundAnswerRejectsAttemptOrRecoveryRetarget|DecisionSubmissionRequiresNativeAuthorityBeforeStateAccess)$' -count=1 -v`：3 项通过、0 skip；`app.log`。覆盖正式 mapper 复用原 native question 规范 choice/text/remarks、trim 与原 Attempt/recovery。
- `go test ./internal/product/query -run '^TestCommandReceiptProjectsTyped(RequestIdentity|DiscardResult)$' -count=1 -v`：2 项通过、0 skip；`query.log`。typed 原请求/丢弃回执完整投影且错误 JSON 不伪造可核验内容。
- `go vet ./internal/product/httpapi ./internal/product/client ./internal/product/query ./internal/app`：通过、无诊断；`vet.log`。本轮不重复 L1/Full/L2/L3/live，也未改共享测试驱动；旧 L1 仅证明旧候选的实际范围。临时 fixture 和 listener 随 Go 测试清理，原始证据保留。

文档检查：`python scripts/check_doc_links.py` 266 Markdown/1679 local links/0 errors；`python scripts/roadmap_task_id.py check` 84 identities/通过；`git diff --check` 通过，原始结果见同证据根 `links.log`、`identities.log`、`diff.log`。

局部缺陷已修复且上述复验通过；#45 尚未整体验收或实现完成。原执行者仍需 profiles/presentation 查询、真实 API1 unauthorized mutation/正向基线、API3 要求的全部真实故障窗口、SHM AccessDenied 根因诊断或真实未闭合记录，以及最终必要验证；之后须新的独立复审与真实合并核验。固定 Spec/Plan 未改。

## round 2 真实消费与故障窗口

当前代码候选 `99c98e2bd02b3c0f75fbdea4919ce54e59927745`（tree `d8b7fad5eff3db07cdb7b5f544c0b68c2509a36c`），在固定 Spec/Plan 之外只补充执行场景。`GET /v1/projects/{project_id}/presentation` 返回现有安全 `ProjectPresentation`、默认 profile、digest 和 unavailable reason；真实查询结果被 API2 集成场景用于 `start_candidate`，不把 implementation payload 或凭证暴露给客户端。

| 范围 | 命令/结果 | 原始证据 |
| --- | --- | --- |
| HTTP/API DTO 与 matcher | `go test ./internal/product/httpapi -run '^TestAPI' -count=1 -v`：15 个顶层测试及子场景通过、0 skip。覆盖 profile presentation、首次 intent 注册、真实正式 request/SQLite receipt 内容绑定和安全临时写入。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-final-unit-3b4d98fa041d48fe925564cfc1d80a89\httpapi.log` |
| API1 真实授权、轮换和正向基线 | `TestAPIIdentityAndAuthentication` 通过。7 类缺陷请求在真实 loopback listener 上被拒绝；请求/receipt/SQLite command 与 control state 未变化。重建同一实例的 listener 后旧 token 被拒，新 token 的合法派发 pause/resume 只产生一个 revision 变化。 | 同上 API 集成组日志；其首轮与失败均在 `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-api1-api3-cabd0889bab64baa8c93fd77373d4920\api1-api3.log` 中保留 |
| API2 profile 与查询 | 真实 `TestAPIQueryParity` 查询 profile 摘要和 DefaultProfileID，再用所选 profile ID 创建第二本机候选；任务、图、历史、run/execution/attempt、活动、final 正文查询通过。最终组内 1 次通过，另独立 `-count=3` 三次通过。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-final-integration-c8b0385e767642fca6c1e82fbae7bfab\api-integration.log`；`C:\Users\lmas1\AppData\Local\Temp\steerline-api45-query-count3-944d238e85fc422389900ad421637853\query-parity-count3.log` |
| API3 真实故障恢复 | `TestAPIControlRequestFaultRecovery` 在真实 HTTP+app `ControlInboxPort`+service actor 上验证取消发送、已发布请求后丢响应、正式 consumer 竞争、8 个 late duplicate 和异 revision request ID。每个合法动作沿原 envelope/request ID 对账且 revision 只增 1；foreign revision receipt 不覆盖原 envelope。 | 上方完整 API 集成组 `api-integration.log`；单项修正前失败另见 `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-fault-retry-ff393d1ae7da4335affd3597740fe8c1\api-fault-recovery.log` |
| API4/API5 真实服务 | `TestAPIOriginalCommandReceipt`、`TestAPIReconnectAndUnsupported`、`TestAPIServiceLifecycle`、`TestAPIConfiguredServicePublishesDiscovery` 全部通过；完整 API1–API5 共 7 项/0 skip/166.931 秒。 | 上方完整 API 集成组 `api-integration.log` |
| SQLite sidecar 检查 | Windows pathguard 4 项通过：DB/WAL/SHM/journal hardlink 仍拒绝；持续 ACL 拒绝仍为 `ERROR_ACCESS_DENIED`；临时 sidecar denial 后路径消失时有界重查为 missing；真实 modernc SQLite WAL 并发开关 800 次、4,800 次 preflight 检查无错误。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-final-shm-a8898f3220e748beb5cb4e918857a261\sqlite-shm-regression.log` |
| 既有应用/DTO 回归 | app 定向 3 项、query 定向 2 项通过，保留原 Decision normalize、Attempt/recovery binding 与 typed receipt projection。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-final-app-0845bd62349547dd9a70447af9d87076\app.log`；`C:\Users\lmas1\AppData\Local\Temp\steerline-api45-final-query-c38e008819da47bda37745caea212d2e\query.log` |
| Go 静态 | `go vet ./internal/product/httpapi ./internal/product/client ./internal/product/query ./internal/app/... ./internal/pathguard`：通过，无诊断。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-final-vet-0b9b4461704b42fe9ada1bf9e545caaf\vet.log` |
| L1 | 精确候选 `99c98e2b`：`complete=true`, `passed=true`，12 组/428 项、0 skip、0 fail。原 L1 的 427 项属于较早源码候选，保留原记录，不用于本候选计数。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-l1-final-86db6d7680594b8a99a31984510701f9\l1.log`；summary `C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261008T215541.153861900Z\summary.json` |

### SQLite `-shm` 历史失败的判定

round 0 的最终正文读取曾由 `pathguard.RejectSharedSQLiteFiles` 返回 `inspect ... steerline.db-shm: Access is denied`，旧实现未标明错误来自 Windows `CreateFile` 还是 `GetFileInformationByHandle`，原始日志保留在前述失败轮证据目录。检查了实际依赖 `modernc.org/sqlite v1.38.2` 的 Windows `_winShmPurge`：最后引用释放后先关闭 SHM handle，再调用 `_winDelete`；Windows `CreateFile` 文档说明 delete-pending 文件可能返回 `ERROR_ACCESS_DENIED`。这是与观测相符的可能竞态，但旧失败现场未保存系统调用阶段，也未能在复验中直接重现，故不能把根因写成已证实。

当前实现只对 `-wal/-shm/-journal` 的 `ERROR_ACCESS_DENIED` 做最多 5 次、总计 10ms 的短重查；稳定 ACL denial 在回归中仍失败，hardlink 检查未放宽。真实 SQLite sidecar open/close stress、API2 全组及 `QueryParity -count=3` 均通过。由于原始错误的精确系统调用与现场状态不可恢复，API2 仍保留 `验收失败`；新 reviewer 需决定上述有界处理与当前回归是否足以关闭该历史故障，不能按环境波动或重跑通过自动销项。

### 保留的 round 2 失败

round 2 首次 `TestAPIControlRequestFaultRecovery` 曾因测试只允许 late consumer 竞争返回 `identity_conflict` 而失败；真实 same-intent consumer 在竞态中也可能直接返回匹配的原 request。测试现同时核验两种合法返回，并要求后续 GET 按原 envelope 对账、业务 revision 仅增 1；修正后的单项与完整集成组通过。原失败日志位于上表单项 evidence，未删除或改写。

当前 API1/API3/API4/API5 自动项通过；API2 的历史 SHM 故障仍失败/待独立裁定。没有人工后置项，也没有 Full/L2/L3/live-model 验收。PR #249 仍 OPEN、未合并；#44 继续等待 #45。

## 最新 main 合入后的组合核验

main 更新为 `04c62ee5ddb10637a68745baa4fc4f811123ffa9` 后，保留本分支原提交，并以 merge commit `08341fbc90e975ed512e47740860f2d2e25edb1d`（tree `ba3e2837dd616ebc8789bbc2a56771e281a2216d`）整合。`08341fbc^1` 是本 PR 原 head `4bb0c8704339dcf3db87d0c6ad2f7843ba21212b`，`08341fbc^2` 是上述最新 main。合入差异仅 `internal/console/model_control.go` 和 `internal/console/start_request_test.go`：restored request deduplication 只阻塞同 task 的未结算 `start_task`/`start_candidate`；pending decision/environment request 不阻止 profile picker，已完成的 start 也不阻止重选。API/host/SQLite/HTTP 生产文件无差异，因此 round 2 API1–API5 证据仍精确映射到上节的 `99c98e2b` 源码候选。

| 范围 | 命令/结果 | 原始证据 |
| --- | --- | --- |
| 受 main 影响的 console start/profile 消费者 | `go test ./internal/console -run '^(TestStartSelectedRestoredRequestDeduplication\|TestEnterFocusesWorkspaceAndStartKeepsOneRequestID\|TestStartIsNotPublishedWhenRequestIdentityCannotBePersisted\|TestStartDisablesStaleCandidatesAndAllowsManualParallel\|TestReadyStartChoosesConfiguredProfileAndBindsRequest)$' -count=1 -v`：5 个顶层测试及恢复状态子场景通过。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-merge-console-f00084d4478e48439d695e3b2f6629d7\console-start-request.log` |
| Go 静态 | `go vet ./internal/console`：通过，无诊断。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-merge-vet-4d0dca3c40924911a32b0a6246ffa648\console-vet.log` |
| 最新 main 组合 L1 | 精确 merge commit `08341fbc`：`complete=true`, `passed=true`，12 组/441 项、0 skip、0 fail。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-l1-merged-49811ead77e24acab51b2893dc1c0b6c\l1.log`；summary `C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261008T221034.224737800Z\summary.json` |

最新 main 组合核验不更改 API2 判定：历史 `-shm Access is denied` 现场的 syscall 阶段未知，API2 仍为验收失败，等待新 reviewer 审查有界重查与回归证据。PR #249 head 会记录 docs-only evidence commit；未合并，#44 仍等待 #45。
