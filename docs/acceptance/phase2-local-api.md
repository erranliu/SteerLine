# 任务 #45 验收记录：本机客户端协议与历史查询

唯一归属 #45；范围依据 Spec/Plan `f628161b5a60cefeb75922c15a63b1df9ef9c21b` 的 `docs/spec/phase2-local-api.spec.md` 与 `docs/spec/phase2-local-api.plan.md`。实现候选为 `c18acecc195d294c4b2e7250ad2075224e73bdfa`，tree `c1f9ca9c42bed5ada307b6f6b83b7e55836a98a7`；Plan 仅补充了实现期真实服务集成场景和命令，不扩展验收要求。环境：Windows/amd64、Go 1.26.4；受控临时 Git/SQLite 和 Codex app-server，无 live 模型。记录日期：2026-10-09（Asia/Shanghai）。

## 自动验收

| 项目 | 结果 | 已覆盖行为与证据 |
| --- | --- | --- |
| API1 | 验收失败 | round 1 已确认合法临时文件 ACL/preflight 竞态；真实未授权 mutation/副作用观察尚缺。旧通过仅覆盖下列已执行场景： 真实 Windows 用户 ACL、发现/凭证、身份与未授权请求拒绝通过。定向 `TestAPIIdentityAndAuthentication`；真实服务集成同名场景通过。 |
| API2 | 待验收 | round 1 发现现有消费者所需 profiles/presentation 查询遗漏，待原执行者补齐。旧通过覆盖： 任务、图、项目快照、候选、历史、run/execution/attempt 活动和最终回复正文映射通过；受控执行服务上的双候选读取与 `start_candidate` 请求通过。定向和真实服务 `TestAPIQueryParity` 均通过。 |
| API3 | 验收失败 | round 1 同 ID 异内容正式权威请求被错误复用；真实故障窗口驱动覆盖尚缺。旧通过覆盖： 原始请求信封、迟到控制回执、相同 ID 重试/并发、不同内容冲突、未登记请求恢复以及读/权限错误不伪装为 `command_not_found` 通过。单项、定向套件和真实服务 `TestAPIOriginalCommandReceipt` 通过；最多一次控制提交。 |
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
