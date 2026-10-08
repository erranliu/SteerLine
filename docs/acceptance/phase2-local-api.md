# 任务 #45 验收记录：本机客户端协议与历史查询

唯一归属 #45；范围依据 Spec/Plan `f628161b5a60cefeb75922c15a63b1df9ef9c21b` 的 `docs/spec/phase2-local-api.spec.md` 与 `docs/spec/phase2-local-api.plan.md`。实现候选为 `c18acecc195d294c4b2e7250ad2075224e73bdfa`，tree `c1f9ca9c42bed5ada307b6f6b83b7e55836a98a7`；Plan 仅补充了实现期真实服务集成场景和命令，不扩展验收要求。环境：Windows/amd64、Go 1.26.4；受控临时 Git/SQLite 和 Codex app-server，无 live 模型。记录日期：2026-10-09（Asia/Shanghai）。

## 自动验收

| 项目 | 结果 | 已覆盖行为与证据 |
| --- | --- | --- |
| API1 | 验收失败 | round 1 临时文件 ACL/preflight 已局部修复并定向复验；真实未授权 mutation/副作用观察尚缺，未恢复整体通过。旧通过仅覆盖下列已执行场景： 真实 Windows 用户 ACL、发现/凭证、身份与未授权请求拒绝通过。定向 `TestAPIIdentityAndAuthentication`；真实服务集成同名场景通过。 |
| API2 | 待验收 | round 1 发现现有消费者所需 profiles/presentation 查询遗漏，待原执行者补齐。旧通过覆盖： 任务、图、项目快照、候选、历史、run/execution/attempt 活动和最终回复正文映射通过；受控执行服务上的双候选读取与 `start_candidate` 请求通过。定向和真实服务 `TestAPIQueryParity` 均通过。 |
| API3 | 验收失败 | round 1 同 ID 异内容权威误匹配已局部修复并定向复验；真实故障窗口驱动覆盖尚缺，未恢复整体通过。旧通过覆盖： 原始请求信封、迟到控制回执、相同 ID 重试/并发、不同内容冲突、未登记请求恢复以及读/权限错误不伪装为 `command_not_found` 通过。单项、定向套件和真实服务 `TestAPIOriginalCommandReceipt` 通过；最多一次控制提交。 |
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
