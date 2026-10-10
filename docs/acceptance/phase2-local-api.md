# 任务 #45 验收记录：本机客户端协议与历史查询

唯一归属 #45；启动固定来源为 Spec/Plan `f628161b5a60cefeb75922c15a63b1df9ef9c21b` 的 `docs/spec/phase2-local-api.spec.md` 与 `docs/spec/phase2-local-api.plan.md`。Spec 未改；Plan 原路径保持，已有实现期测试场景补充见后续提交。此前 round 4 精确审阅源码为 `c126560f0315b9e686e68e3cb82cf65359e66894`（tree `95800604c072dc75cc9f44096ddf9f1e3135770f`）；它的 P1 service-owned discard 和 P2 record-boundary 证据继续按原候选映射。本轮已将 #249 分支合入最新 main `21d9735057f990f9fb83a678c5293eca8c8c520c`，其中包含真实合并的 #84 worker-flow-v2。#84 merge、组合映射、版本消费缺口修复和本轮验证见文末。环境：Windows/amd64、Go 1.26.4；受控临时 Git/SQLite 和 Codex app-server，无 live 模型。记录日期：2026-10-10（Asia/Shanghai）。

## 自动验收

| 项目 | 结果 | 已覆盖行为与证据 |
| --- | --- | --- |
| API1 | 已验收 | 真实 listener 上缺/错凭证、错 Host/Origin、错路由项目/信封项目/实例及轮换后旧凭证均拒绝；原传输/control/SQLite 请求和 revision/paused facts 不变。轮换后的新凭证成功执行真实 pause/resume，控制 revision 只增加一次。非回环源由 loopback-only 绑定及 `TestAPIIdentityAndAuthentication` 中源地址拒绝断言覆盖。 |
| API2 | 验收失败 | `GET /presentation` 返回真实非默认 profile；客户端发起 start_candidate 后，受控 backend 实际生成该 profile 的 turn/Decision，正式 request 的查询 digest 与 claim 冻结 digest 分别核对（值不同符合 claim-time remote verification）；正式 provider merge 后，HTTP ShowTask/Snapshot 的 PRURL/workflow/execution/mergedAt 与同库 native Query 一致。旧 `TestAPIQueryParity` 的 SQLite `-shm` `Access is denied` 真实失败仍未精确复现/归因；sidecar 有界重查、持续 ACL 拒绝和真实 WAL 生命周期回归通过，当前 QueryParity 重跑通过不抹除原失败。Round 4 reviewer 核验了当前重试组合，但没有证据证明旧失败命中了可恢复的 CreateFile denial 类。用户现已批准对该有限根因/瞬态可用性风险的条件接受；API2 自动验收仍失败，只有 #84 组合集成与最终门禁通过后才可单独登记 `accepted_exception`，当前不作通过或豁免。 |
| API3 | 已验收 | 真实 HTTP + app `ControlInboxPort` + service actor 覆盖 transport 已登记但发送前取消/同信封补交、正式请求已发布但响应丢失/重连 GET 与 retry、正式消费者迟到竞争、8 个并发 duplicate、异版本同 ID conflict；每个合法 request 只生效一次。复审发现的 configured-service discard 重入锁由 `TestAPIConfiguredServiceDiscardsTaskThroughOwnedWriter` 修复并真实验证：writer 沿同一 service actor 工作，原仓库锁全程保持；重连回执、同 envelope duplicate、异内容同 ID 冲突和无重复业务变更均通过。第三轮原始 lock failure 保留见后文。 |
| API4 | 已验收 | 第三轮 600,243-byte 合法 unknown envelope 曾写出 1,201,014-byte rejection，超过 reader 上限并阻断原 ID GET/preflight。持久聚合边界已在 rebase 后源码 `d2a0bce0` 修复；其后 `internal/product/httpapi` 源码未改变，18 项 HTTP/API 定向、真实原 ID 计数/重连 2 项与 vet 通过，第三轮原始失败保留见后文。 |
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

此前本段只记录 pathguard 对 `-wal/-shm/-journal` 的外层有界重查，现补齐当前 main 组合：`internal/pathguard/transient_windows.go` 的 `retryFileInspection` 对 Windows `CreateFile` 的 `ERROR_ACCESS_DENIED` / `ERROR_SHARING_VIOLATION` 最多 20 次调用，19 次间隔各 5ms（每次进入内层最多计划等待 95ms）。`internal/pathguard/link_windows.go` 对 sidecar 另有 5 个外层 round，间隔为 1+2+3+4ms，总计 10ms。若持续 `CreateFile ERROR_ACCESS_DENIED`，按最坏路径最多 100 次 CreateFile、485ms 计划等待（5×95ms + 10ms；不含 OS 调用开销）。`GetFileInformationByHandle` 在成功 CreateFile 后单独执行，不走这段内层重试；若 denial 出现在该阶段，只有外层最多 5 round / 10ms，不包含 95ms 内层等待。

持续 ACL denial 的回归仍失败，hardlink 检查未放宽。真实 SQLite sidecar open/close stress、API2 全组及 `QueryParity -count=3` 通过，不代表真实合法瞬态 denial 已命中并恢复，也不证明历史失败发生在 CreateFile 阶段。旧日志没有保存具体 syscall、原 handle/path/ACL 与 close-delete 时序，round 4 复核仍无法还原；API2 继续为 `验收失败`，没有按 retry 窗口或重跑结果豁免。

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

## 后续 main 状态/反馈组合核验

main 更新到 `7ffc6a39197366b1177e104eb75cbc453ccee78a` 后，以 merge commit `c70414718ec3593b00d1c911ea1894b76614e1a2`（tree `55d50d0b2f0bdea44a80901cd772903560742aac`）整合；其 parent 1 为 `898183701e76f0ae59b17e3241a48c38982f6c55`，parent 2 为最新 main。相对前一整合 `08341fbc`，差异仅 8 个 `internal/console` 状态/反馈投影文件，不改 API/host/SQLite/HTTP 生产源码。因此 API1–API5 的真实服务集成、profile 与 fault-driver 证据仍映射到 `99c98e2b`；当前整合关系和受影响消费者回归映射到 `c7041471`。

| 范围 | 命令/结果 | 原始证据 |
| --- | --- | --- |
| merge-wait/optional feedback 状态消费者 | `go test ./internal/console -run '^(TestWaitingPRMerge.*\|TestStartSelectedRestoredRequestDeduplication)$' -count=1 -v`：6 个 merge-feedback 顶层场景及 restored-start 去重通过；覆盖可选/必需问题、同任务兄弟候选、真实环境/恢复/PR 阻断和终止候选。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-latest-main-console-47cf64b8baf347ec929678aedf57561b\console-merge-feedback.log` |
| console vet | `go vet ./internal/console`：通过，无诊断。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-latest-main-vet-f77b847d2cc846818b704324d8cbcebd\console-vet.log` |
| 最新 main 最终组合 L1 | 精确 merge commit `c7041471`：`complete=true`, `passed=true`，12 组/458 项、0 skip、0 fail。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-l1-latest-main-a8fd3ce8676a435183b5afe5f18f4d84\l1.log`；summary `C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261008T222937.820093500Z\summary.json` |

API2 仍保留 `验收失败`；本次 main 合并未改变其诊断结论或授权任何环境豁免。PR #249 未合并，#44 继续等待 #45。

## round 3 latest main、非默认 profile 与持久 Delivery 查询

在 `c70414718ec3593b00d1c911ea1894b76614e1a2` 之后又合入 main `692a784e05479b11e5818fb5a209971949e3064a`，形成 `8cd4199034502642a35576b88226d3ada5ac2583`（tree `9372f76a4d57c3f0be3517d8cf5f722c86ebb62d`）。新增 main 变更覆盖 host Orca worktree-trash metadata admission、统一 Orca native target ID 和对应 app/native tests；HTTP/API 源码无修改。之后提交 `f82c73876e9d1dfae8be380348bd7f5d7db27fc9`（tree `77d296303f5caba8e327e7c21dada626bd0d3c24`）只加测试与 Plan 场景映射。

| 范围 | 命令/结果 | 原始证据 |
| --- | --- | --- |
| 非默认 profile 经 HTTP 到真实 backend | `go test ./tests/integration -run '^TestAPIQueryParity$' -count=1 -v`：通过。fixture 同时提供默认 `fixture` 与非默认 `fixture-alt`；HTTP 展示所选 ID/digest，原生 control request 使用所选 profile，实际 controlled backend 产生 alt 的 turn/Decision；冻结 ImplementationData 为 alt Model/ReasoningEffort。查询 digest `19226c57…` 与 claim 后 frozen digest `464e05a9…` 分别记录且不要求相等；execution config digest 与 WorkflowRun 冻结 digest 一致。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-nondefault-profile-retry2-6a3157a132f047de8deb2dfab0efe9cd\api-query-nondefault-profile.log` |
| 真实持久 PR/Delivery HTTP 查询 | `go test ./tests/integration -run '^TestCD67AdditionalImplementation/neutral_fixed_input_restart_and_fresh_manual_answer$' -count=1 -v`：通过。沿受控正式 provider merge 流程产生持久 TaskDelivery，HTTP ShowTask 与 Snapshot 的 PRURL/workflow/execution/mergedAt 与同一 SQLite native Query 投影一致；未插入 TaskDelivery 或 transport record、未请求外部真实 PR。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-final-delivery-c978654e3be5494aabff70462a980030\api-delivery-query.log` |
| Host native-state/Orca target 组合 | `go test ./internal/app/host -run '^TestNative(StatePreservesWorktreeMetadata|StateMetadataExceptionIsScopedToRealWorktreeDirectory|StateRejectsMetadataJunction|PreflightRejectsSharedSQLiteBeforeValidator)$' -count=1 -v`、`go test ./internal/app -run '^(TestOrcaSelectedTargetMatchesRestoredLifecycle|TestOrcaWorktreeMetadataKeepsNativeQueriesReadOnly)$' -count=1 -v`、`go test ./internal/execution/adapters/orca -run '^TestNativeLifecyclePlansCommittedResourcesAndSeparateInput$' -count=1 -v` 均通过。API configured service discovery 也在新 preflight 组合下通过。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-main-host-2180c84f62eb422f857b623d3d8045d6\native-state.log`；`C:\Users\lmas1\AppData\Local\Temp\steerline-api45-main-app-orca-b4bb4582232940039d85e5299e3cd92d\orca-target-and-metadata.log`；`C:\Users\lmas1\AppData\Local\Temp\steerline-api45-main-orca-lifecycle-06a94f8112e9424092b40f18c744f267\orca-lifecycle.log`；`C:\Users\lmas1\AppData\Local\Temp\steerline-api45-main-api-service-dc8fdeedeafc4241a622280738fd6696\api-configured-service.log` |
| Host/app/Orca static | `go vet ./internal/app/... ./internal/execution/adapters/orca`：通过。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-main-vet-all-621ca00957de42909e9296e101fa6f3a\vet.log` |
| 最新整合候选 L1 | `go run ./cmd/test-feedback -level=L1` 在 `f82c7387` 上：`complete=true`, `passed=true`，12 组/458 项、0 skip、0 fail。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-l1-final-20015b03df9f4ea799db423845499002\l1.log`；summary `C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261009T011201.901274200Z\summary.json` |

### 保留的 profile-driver 调试失败

初次加入 alternate profile 后，测试 fixture 重新 `LoadConfig` 未再次 `ResolveIdentity`，真实 discovery client 按错项目拒绝；保留日志 `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-nondefault-profile-44c4db0155c74dd9a828f40d06c049c3\api-query-nondefault-profile.log`，修复了 fixture identity 初始化。第二次断言在 ExecutionRecord 查 ImplementationData，未找到实际存放于 WorkflowRun 冻结 profile 的载荷；保留日志 `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-nondefault-profile-retry-bb4810d10d494369bf4713bd480dcde2\api-query-nondefault-profile.log`，随后改为核验原 RunView.ExecutionProfile 并通过。二者均为测试夹具/观察点错误，最终真实后端 turn 与选定 profile payload 已验证；不隐藏历史失败。

API2 的 SHM 原始失败及“可能 sidecar delete-pending”仍是推断而非已证实 root cause；新增 profile 与 PR/Delivery 证据不改变 API2 状态。待新 independent reviewer 同时审完整 latest main 候选和历史诊断判定；PR #249 未合并，#44 仍等待 #45。

## round 3 独立复审失败范围

复审 head `99fd50342dca491a09c25227e7fd197010230467`、base/main `692a784e05479b11e5818fb5a209971949e3064a`。真实 configured service `discard_task` 的仓库锁重入失败属于 P1；服务本身持有原锁，HTTP 原生 mutation 再次取得该锁而失败。P2 是 reader/writer 聚合边界不一致：600,243-byte unknown 请求 rejection 的 Envelope 与 Response.Original 重复存储，使 1,201,014-byte 文件不能正式回读；同时阻断项目 preflight 与服务重启。两者都属于固定通过条件内的真实失败，不以旧定向/L1 通过覆盖。

保留原始证据根 `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-round3-review-a10c27bf69a4460b8f60d8c545dab95f`：`configured-discard.log`、`large-rejection.log`、`httpapi.log`；第三轮 15 个 httpapi 顶层测试通过，而新增独立正式服务 discard 与大 rejection 探针失败。本轮只修 P2 的 httpapi 持久边界，不绕锁、不修改 discard service 接线；P1 仍失败并由原执行者后续补齐。API2 合法 sidecar 生命周期实际命中 AccessDenied 后由有界重查恢复的 syscall/现场证据仍缺失，历史 SHM 根因未闭合，继续保持失败。

profile digest 摘要勘误：上表按原始 `api-query-nondefault-profile.log` 修为查询 `19226c57…`、claim 冻结 `464e05a9…`；仅修正摘要，不改变测试通过事实，也不作为 SHM 根因证明。

## round 3 P2 局部修复与精确源码验证

实现/协议提交 `34e9e0ee8ea83d2d60b568a4525a0fe368ae6795`，source tree `2c0c0d4c7afd7fdc02e2b54aefef5025eff8623d`、httpapi subtree `48c7e689f65c1e57ac5fc7ba06f3bdec76d22908`；后续 acceptance/execution-map 提交仅映射结果，不改变该受验源码或固定 Spec/Plan。所有以下产品命令执行时 HEAD 精确为该实现提交，工作区只有未提交的两份结果文档；没有实现/测试源码差异。

首次 unique 登记和后续 atomic 更新共用聚合编码 guard，保留 reader 原 1 MiB + 32 KiB 上限；完整 envelope、digest、created_at 和实际 mapped control/normalized decision/discard intent 在副作用前核验不含结果的 1,064,960-byte budget。结果只持久有界 transport outcome；Original 从原信封恢复，typed request/receipt 仍由现有正式 authority 完整查询，后续大回执不扩张 arbitration 文件。超限新请求明确 HTTP 413 / `request_record_too_large`，不登记、不 submit，不截断原 payload/映射意图，不增加旧格式迁移/双读/回退。code/diagnostic 的持久摘要有明确截断标记。Windows private writer/DACL、immutable 原信封与完整 mapped intent binding 继续保留；未修改 host/锁/domain/schema。

证据根：`C:\Users\lmas1\AppData\Local\Temp\steerline-api45-record-boundary-34e9e0ee`。

| 范围 | 精确命令与结果 | 原始证据 |
| --- | --- | --- |
| P2 + API 协议/绑定/private preflight 定向回归 | `go test ./internal/product/httpapi -run '^TestAPI' -count=1 -v`：18 顶层测试通过、0 skip，8.073s。新增 `TestAPIRequestRecordSizeBoundary`（7 子场景）、`TestAPIRequestRecordBoundedRejection`、`TestAPIRequestRecordAtomicSizeGuard` 全部通过；正式 HTTP listener、实际文件、正式 reader 和 `ValidateStateDirectory`，不是仅比较 sizeof helper。 | `httpapi.log` |
| API3 真实控制计数、API4 重连/unsupported | `go test ./tests/integration -run '^TestAPI(OriginalCommandReceipt\|ReconnectAndUnsupported)$' -count=1 -v`：2 项通过、0 skip，66.067s；沿临时 Git/SQLite、正式 control request/receipt、服务 actor 核对真实 revision 与原 ID，不声明 configured discard 已修复。 | `integration.log` |
| httpapi 静态 | `go vet ./internal/product/httpapi`：exit 0，无诊断。 | `vet.log` |

边界实测：原复现 wire **600,243 bytes** → rejection 文件 **600,540 bytes**，unsupported 后 GET 与 private preflight 正常；wire **1,048,576 bytes**（恰好 1 MiB）→ rejection 文件 **1,048,873 bytes**；mapped control wire **530,283** → 文件 **1,060,849**；normalized decision wire **530,532** → 文件 **1,061,243**。正向保存完整原 envelope/实际映射意图，late duplicate 不改文件；异内容同 ID conflict。超预算 control/decision、规范化额外扩张均 413，transport 文件/正式 control authority/受控 decision authority/实际 submit counter 保持原值，原 ID 权威未登记；每种情况随后合法 pause 仍可发布。

后续 1 MiB typed control receipt 沿正式 query 完整返回，arbitration 不追加重复内容；删除仅此 fixture 所属正式 request 后，GET/duplicate 保留原 envelope 并返回 outcome_unknown、submit 次数仍为 1。近边界 supported rejection 的超长诊断和最坏 JSON 转义也通过实际 writer/reader/preflight。control 正向使用正式 host immutable publisher，profile resolver 和 decision normalizer/receipt 由受控测试 port 提供大型映射；此边界验证不宣称大型业务输入绕过领域约束或已由 service actor 执行业务。真实临时服务/actor 原 ID计数由独立集成组补充。所有新测试资源由各自临时 fixture 清理，无生产测试写口、无用户安装/凭证或他人服务操作。

未重跑 Full/L1/live；现有 `f82c7387` L1 证据原样保留，不作为本次变更的精确源码结果。本轮 P2 局部定向通过不等于独立最终审核，API4 旧失败保留并等待新复审及最终组合结果；P1/Task 丢弃与 API2 历史 SHM/合法瞬态 denial 证据仍未闭合，PR 未合并，#45 未完成，#44 不释放。

结果文档静态：`python scripts/check_doc_links.py` 266 Markdown/1681 本地链接/0 errors；`python scripts/roadmap_task_id.py check` 84 identity files（9 draft/3 formal）通过；`git diff --check` 通过。原始输出分别为同证据根 `doc-links.log`、`task-ids.log`、`diff-check.log`。

## rebase 后 configured-service P1 修复

Task #45 分支基于 `c4bcbec06289c9f5b086c8f1f98859a0f858d7e5` rebase；#45 Spec/Plan 的两个 Git blob 与启动固定提交 `f628161b5a60cefeb75922c15a63b1df9ef9c21b` 完全一致。实现提交 `cab6b6dc5e80fa8021316d0e7a5132ff25cbd478` 将 HTTP 产品客户端的 discard port 绑定到配置服务自己的 `execution.Service`。discard 作为类型化 `Signal` 排入服务已有 actor queue，再由 service-owned `CommandService.DiscardNativeTask` 和已持有的 command store 执行；服务不重新打开 writer、不释放锁、不新增 SQL 写入口或测试专用生产 seam。普通独立客户端仍使用原 TaskCommands 写入端口。

固定 Plan 的 API3 控制故障行保持不变；下面是针对第三轮发现的 configured-service 锁重入缺陷增加的最小补充场景。产品状态及字段仍只由原 native task authority 管理。

| 范围 / 场景 | 命令与结果 | 原始证据 |
| --- | --- | --- |
| API3 P1：configured service 持仓库锁时 discard | `go test ./tests/integration -run '^TestAPIConfiguredServiceDiscardsTaskThroughOwnedWriter$' -count=1 -v`：1 项通过、0 skip；测试耗时 64.76s。正式 `RunConfiguredService` ready 后先验证 repository lock 被占用；HTTP discard 成功并返回原 envelope 与 typed receipt。关闭并重连后 GET 回原回执，同 envelope retry 不重复变更；同 request ID 异 payload 得到 409 `request_id_conflict`，Task version/authority/graph revision 均不变。服务保持 ready，后续 pause control 仍成功，discard/duplicate/conflict/control 后原仓库锁仍被占用；停服后 discovery 移除且锁可取得。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-configured-discard-e0581348aee744dea00b3b6c130fba9d\api-configured-discard.log` |
| 受影响 Go 静态 | `go vet ./internal/app ./internal/execution ./internal/product/client`：通过，无诊断。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-p1-vet-feb634e528024a9c9ce7704fa77a56c7\vet.log` |

曾启动 `go test ./internal/execution ./internal/product/client ./internal/app`，在无输出且未完成时为避免扩大为 package-wide rerun 而中断；该命令不记通过或失败。没有重跑 L1/Full/L2/L3/live，既有 `f82c7387` L1 仅保留其历史源码范围。最终 P1 结论只依据上面的真实 configured-service 集成和当前 vet；P2 的 `internal/product/httpapi` 源码自精确 18 项验证后未改变。新候选待独立复审；API2 历史 `-shm AccessDenied` 的系统阶段仍未知，保持验收失败、不作环境豁免；PR #249 未合并，#44 继续等待 #45。

文档静态：`python scripts/check_doc_links.py` 271 Markdown/1740 本地链接/0 errors；`python scripts/roadmap_task_id.py check` 85 identity files（10 draft/4 formal）通过；`git diff --check` 通过。原始输出在 `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-final-docs-72f22ca79349478d908e740bfffc05f6`。

## Round 4 独立复审与当前候选证据

Independent reviewer `/root/review45_round4` reviewed exact source candidate `c126560f0315b9e686e68e3cb82cf65359e66894` (tree `95800604c072dc75cc9f44096ddf9f1e3135770f`; base `c4bcbec06289c9f5b086c8f1f98859a0f858d7e5`; PR #249 OPEN) and found no newly confirmed local code defect or fixed-requirement omission. The reviewer traced the configured-service lifetime, writer/actor signal, HTTP arbitration, Windows ACL/preflight and Query/Delivery paths; confirmed the P1/P2 fixes against source and test evidence; and explicitly did not treat API2 as closed or this review as #84 combination proof. The review report is `C:\Users\lmas1\.codex\task-state\roadmap-runner\roadmap-steerline-20261008-01a11a74\review45-round4.md`; the PR record is [round 4 review comment](https://github.com/erranliu/SteerLine/pull/249#issuecomment-6073460856).

| 范围 | 精确命令与结果 | 当前证据位置 |
| --- | --- | --- |
| HTTP/API 与 P2 边界 | `go test ./internal/product/httpapi -run '^TestAPI' -count=1 -v`：18 顶层测试、0 fail/0 skip，6.739s。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-review4-targeted-653dd425c05944ab9d0e882b68dbf865\httpapi.log` |
| P1 configured-service 持锁 discard | `go test ./tests/integration -run '^TestAPIConfiguredServiceDiscardsTaskThroughOwnedWriter$' -count=1 -v`：1 test、0 skip，测试 73.51s / package 78.676s。原仓库锁保留、typed receipt/重连/重复/冲突/无重复 revision/ready/control/停服释放均通过。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-review4-targeted-653dd425c05944ab9d0e882b68dbf865\configured-discard.log` |
| 当前源码 L1 | `go run ./cmd/test-feedback -level=L1`：exact source `c126560f`，12 groups / 458 passed / 0 fail / 0 skip，`complete=true`, `passed=true`，3 workers，111.775s execution。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-review4-bc29830a550841ba968719ddcec654c7\summary.json`；该根目录下 `quick-*/test.jsonl` 与 `quick-*/stderr.txt` |
| 当前源码 Go vet | `go vet ./internal/product/httpapi ./internal/app/... ./internal/execution ./internal/product/client`：exit 0。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-review4-targeted-653dd425c05944ab9d0e882b68dbf865\vet.log` |
| 差异格式 | `git diff --check`：通过；review tree clean。 | round 4 review record and current branch state |

Round 4 L1 的初始 output path 参数是字面量 `$reviewOut`。runner 停止后，reviewer 核验原始绝对路径并将对应 artifacts 放入上列临时 evidence root；`summary.json` 原始 path 字段未改写，当前实际文件按其相对后缀对应到 `quick-*/...`。原 JSON 与移动后文件均保留，不把 `$reviewOut` 当作可直接打开的绝对路径。

Review 复核当前重试实现时补正边界：`retryFileInspection` 对 `CreateFile` 的 access/sharing denial 最多 20 calls、19×5ms；#45 sidecar 外层最多 5 rounds，另有 1+2+3+4ms。持续 `CreateFile ERROR_ACCESS_DENIED` 的组合上限为 100 calls / 485ms planned wait，不含 OS overhead；`GetFileInformationByHandle` 阶段没有内层重试，只有外层 round 间隔。以上是当前 main 与 #45 的组合实现，不是历史 SHM failure 的 syscall 归因或恢复证明。Review 保留 API2 `验收失败`：原 QueryParity failure 的具体阶段/ACL/close-delete 时序未知，当前有界 fail-closed 行为和 L1 不能将其转成通过。用户随后明确批准了有限、带条件的剩余风险，原话及生效门槛见下节；在门槛满足前不记 `accepted_exception`。

当前 source review 已完成，PR #249 仍因 API2 未闭合而 OPEN、未合并。#84 的实际 worker 状态/格式组合尚未发生；本轮 review 不授权提前集成，也不改变 #44 对 #45 真实合并的等待条件。相对已审源码 `c126560f`，本轮仅修改本验收记录和 `docs/roadmap/execution-map.md`，未改代码。

Round 4 文档同步静态：`python scripts/check_doc_links.py` 271 Markdown/1741 本地链接/0 errors；`python scripts/roadmap_task_id.py check` 85 identity files（10 draft/4 formal），next ID 87；`git diff --check` 通过。原始输出在 `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-latest-main-docs-19fa0c5869d043b0ae9ffe993f658edd`。本轮未分配或修改任何 Task ID。

## 用户对 API2 有限剩余风险的条件接受

2026-10-09，用户对如下完整问题明确回复“接受”：“是否接受 #45 的 SHM 根因未证实、可能暂时误拒绝查询或启动的风险，保留未验记录，并在 #84 集成复核及最终门禁通过后合并？”协调已将原话、范围和条件记录在原 run 授权及 [PR #249 评论](https://github.com/erranliu/SteerLine/pull/249#issuecomment-6073688681)。

该接受范围仅限历史 API2 合法 SQLite SHM `Access is denied` 的根因/syscall 证据缺口，以及合法瞬态 denial 可能在有界窗口外暂时拒绝查询或启动的可用性风险。原自动验收失败、syscall 阶段未知和现场不可复现仍保留；不改写为自动通过，不改成待人工验收，不扩大到其他缺陷或风险，也不作为当前 `c126560f` 候选的合并许可。

只有待 #84 实现进入主干后，#45 按真实状态/格式/接口完成必要共享集成，组合验证、独立复核及最终平台门禁全部通过，才按用户已给授权将该 API2 事项单独记为 `accepted_exception` 并进入合并流程。在此之前 API2 仍显示“验收失败”，#45 等待 #84 共享集成，PR 不合并；#44 继续等待 #45 真实合入。

## 2026-10-10 #84 合并后的共享接线

GitHub 已核实 #84 PR #262 的 head `be4488aeedfb2a1a4f01623c2514e764742fa291` 于 `2026-10-09T11:21:48Z` 合并，merge commit 为 `267858f017a0393c217c5ce003bc7bedf5d6a561`。该交付进入最新 main `21d9735057f990f9fb83a678c5293eca8c8c520c`；post-merge 独立审计仍在进行，WFS-M1 仍未验。本段不将 #84 标为实现完成。

#45 已将该 main 合入原 PR 分支，并按实际 `worker-flow-v2` 结构核对 stage 查询映射：`WorkflowRunRecord` 的 `FlowRef`、`StepID`、`Status`、`StageDiagnostic`、`StageResults` 和报告策略，经 `RunView`、product query、HTTP `CandidateSummary`、HTTP client 与页面快照消费。公开结果逐项保留步骤、原 Command/Turn、输入版本、成功/最终核验、最终正文/来源、失败事实和结构化 outcome；不从旧 summary 推导 stage 状态。

集成时发现 `start_task` 的 HTTP envelope 已映射 `expected_task_version`，但 main 新 actor admission 忽略该字段并一律取当前 graph 版本。#45 已修复为：显式旧版本在 admission 时返回 `stale_version`，省略版本的既有内部调用仍使用当前版本；新增真实 service control inbox regression 覆盖旧版本不会创建 worker claim。另新增受控三阶段本地 worker 通过真实 HTTP run 查询与 task page 对比全部公开 stage 字段的 producer-to-consumer 场景。

本轮精确接线源码为 `8f95b1f7084631c5d64c9532c694386dc5080117`（tree `6b9a1ac4a5ea7bc5e5ecbca11fbbe12c8ba2d492`），基于 merge commit `78355a5a8a7fa6396d1dccede45939f8fadded5d`。执行回归、HTTP/API、服务集成和静态检查通过；精确命令与日志如下。后续仅两处测试 fixture keyed-literal 清理提交 `968c26afbb5697b0517e3d75109801a73b55debf` 后完成 vet；该提交不改产品代码或测试行为。

| 范围 | 命令 / 结果 | 原始证据 |
| --- | --- | --- |
| `start_task` 版本消费与 actor 生命周期 | `go test ./internal/execution -run '^Test(TaskStartControlRejectsStaleDisplayedTaskVersion\|TaskStartControlDoesNotSerializeIndependentTasks\|TaskStartControlRecoversReservationsWithoutReplay\|TaskStartControlRejectsAuthorityChangedDuringPreflight)$' -count=1 -v`：4 顶层测试、0 skip；5 个独立 worker 阶段、stale version 拒绝、reservation recovery 和 authority recheck 全部通过，150.918 秒。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-84-combo-20261010\execution-targeted.log`（源码内容已提交为 `a4a0d32f`） |
| HTTP/API 协议、intent 和持久边界 | `go test ./internal/product/httpapi -run '^TestAPI' -count=1 -v`：18 顶层测试、0 skip，包括 expected version/payload mapping、request intent 绑定、SQLite 回执和 size-boundary。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-84-combo-20261010\httpapi.log`（源码 `8f95b1f7`） |
| API stage producer-to-consumer | `go test ./tests/integration -run '^Test(APIConfiguredServiceDiscardsTaskThroughOwnedWriter\|APIQueryParity\|APIWorkerStageQueryProjection)$' -count=1 -v`：3 项、0 skip、206.293 秒。Configured-service discard 和既有 QueryParity 通过；新增受控本地 worker 三阶段真实持久结果经 HTTP `/runs/{id}` 与 `/snapshot` task page 对照 native `RunView`，FlowRef、StepID、status、StageDiagnostic、report policy 及每条 StageResult 字段相符。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-84-combo-20261010\api-integration-retry.log`（源码 `8f95b1f7`）。首次测试编译错误已修正，原日志保留在同目录 `api-integration.log`；不计通过。 |
| API1/API3/API5 服务路径 | `go test ./tests/integration -run '^Test(APIServiceLifecycle\|APIConfiguredServicePublishesDiscovery\|APIIdentityAndAuthentication\|APIOriginalCommandReceipt\|APIReconnectAndUnsupported)$' -count=1 -v`：5 项、0 skip、142.567 秒。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-84-combo-20261010\api-service-integration.log`（源码 `8f95b1f7`） |
| Vet | `go vet ./internal/product/httpapi ./internal/product/client ./internal/product/query ./internal/app/... ./internal/execution`：提交 `968c26af` 后通过、无诊断。首次 vet 发现两个主干合入的 fixture unkeyed literal，按字段名修复；失败与通过日志分别保留。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-84-combo-20261010\vet.log`、`vet-retry.log` |
| 文档/身份静态 | `python scripts/check_doc_links.py`：294 Markdown、1832 本地链接、0 errors；`python scripts/roadmap_task_id.py check`：88 identity files、10 draft、5 formal rows、next ID 91；`git diff --check` 通过。 | `C:\Users\lmas1\AppData\Local\Temp\steerline-api45-84-combo-20261010\doc-links-final-pre-l1.log`、`task-ids-final-pre-l1.log`、`diff-check-final-pre-l1.log` |
| L1 | 最新合入/最终实现候选的固定 L1 尚待本节记录；当前不据此宣称本轮 L1 通过。 | 待运行 |

API2 的历史 SHM failure 与 syscall 阶段未知仍保留为 `验收失败`；用户授权的限定 `accepted_exception` 门槛尚未满足。PR #249 仍 OPEN，不合并；#44 不启动。
