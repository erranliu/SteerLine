# Plan：本机客户端协议与历史查询

> #45 已实现并合入，本文件保留原交付规格/计划；实际状态/限制见专属验收。新 TUI 接入和必要扩展另由[任务 #91](tui-http-client.spec.md)交付，不将新要求回写成旧任务成果。原固定 commit 的契约仍按其原版本读取。

任务 ID：45；任务名称：本机客户端协议与历史查询；目标：`PHASE2.1`。执行范围见 [Spec](phase2-local-api.spec.md)，结果记录于[专属验收](../acceptance/phase2-local-api.md)。无未完成硬前置，一个实现 PR。正式实施由 roadmap-runner 核对 execution-map 的范围与前置，启动时固定实际 Spec/Plan commit 和路径。

## 实施与所有权

1. 对照现有 product/api、client、query 和控制台消费者，固定执行 DTO/action 映射；沿当前身份和 CommandService，不复制领域规则。
2. 在 internal/product/httpapi 交付传输适配、身份与发现、认证、查询和命令路由；这是拟新增实现位置，不先创建空包。app/host 只组装和承载，不推进业务状态。
3. 复用持久命令回执验证重复、未登记和未知结果窗口，提供客户端原信封查询/补交样例。用查询刷新闭环，不建设 SSE 或离线 CLI。
4. 交付 #44 可调用的身份/查询样例与 Windows 启动说明。宿主接线由本任务负责；若 #83 先合入，按其 exe/home 入口接线，若后合入则复核双方路径。共享接线串行合入，不虚构 DAG 边。
5. 完成 API1–API5，保留失败和未验项；不运行新的模型交付来验证纯传输。

## 测试计划

固定 Plan（`f628161b5a60cefeb75922c15a63b1df9ef9c21b`）中的预期测试入口已在实现候选交付。本实现期补充行细化 API1–API3 的真实 Windows 服务驱动和 SQLite sidecar 竞争诊断，不改变 Spec 的成果或通过条件。

| 级别 / 场景 ID | 环境与命令 | 时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- |
| 定向 Go / API1、API2、API4、API5 | Windows、真实临时服务/Git/SQLite、受控后端；下方 API 查询/身份定向命令 | 路由接线完成后 | 非零命中；ACL/实例/查询一致及诊断断言通过 | 本次临时目录 api-targeted.log；摘要写专属 acceptance |
| 定向接线 / API3 | 同上；go test ./internal/product/httpapi -run '^TestAPIOriginalCommandReceipt$' -count=1 | 命令接线后 | 注入未登记、丢回执及迟到提交，原 ID 仲裁至多一个副作用 | 临时目录 api-command.log 与身份/回执摘要 |
| 定向真实授权 / API1 | Windows 临时 Git/SQLite 项目和真实 service actor；`go test ./tests/integration -run '^TestAPIIdentityAndAuthentication$' -count=1` | auth/发现接线后 | 缺/错凭证、错误 Host/Origin/project/instance 和轮换后的旧凭证均在原信封/控制/SQLite 事实不变时拒绝；新凭证的合法 pause/resume 有一个真实状态变更 | 临时目录 `api1-auth.log`；逐项摘要写本 acceptance |
| 定向真实故障 / API3 | 同一真实 HTTP server、真实 app `ControlInboxPort` 与 service actor；`go test ./tests/integration -run '^TestAPIControlRequestFaultRecovery$' -count=1` | 请求登记/恢复接线后 | 取消发送前与发送后响应、同内容正式消费者竞争及异版本竞争；沿同一 request_id/envelope 对账；真实 revision 至多变化一次，异版本 receipt 不得覆盖原 envelope | 临时目录 `api3-faults.log`；请求/receipt/状态断言及摘要写本 acceptance |
| 定向 sidecar 竞争回归 / API2 | Windows；真实 `modernc.org/sqlite` WAL 建库、并发开关 SQLite 连接与 link-count preflight；`go test ./internal/pathguard -run '^Test(RejectSharedSQLiteFiles|RejectSharedSQLiteFilesDuringConcurrentSidecarLifecycle|PersistentSQLiteSidecarACLIsNotIgnored)$' -count=1` | SHM 访问失败诊断后 | 正常 SHM 生命周期检查不误报，硬链接仍拒绝，持续 ACL 拒绝不被重试掩盖 | 临时目录 `sqlite-shm-regression.log`；原 QueryParity 失败独立保留 |
| 定向真实接线 / API1–API5 | Windows 临时 Git/SQLite 项目、真实 app host/service actor、受控 Codex app-server/Git；`go test ./tests/integration -run '^TestAPI(IdentityAndAuthentication|QueryParity|OriginalCommandReceipt|ControlRequestFaultRecovery|ReconnectAndUnsupported|ServiceLifecycle|ConfiguredServicePublishesDiscovery)$' -count=1` | httpapi 命令/查询组装后及提交前 | 七个测试全通过且零 skip；真实身份/ACL、presentation 的非默认 profile 选择、双候选查询、所选 profile 的实际 backend turn/Decision、final 正文、原信封故障恢复及应用服务 lifecycle/discovery 通过。记录 query digest 与 claim 后冻结 digest；claim-time remote verification 可使二者不同，不用错误相等断言。不启动 live 模型 | 临时目录 `api-integration.log`；逐项摘要写本 acceptance |
| 定向真实 Delivery 查询 / API2 | Windows 临时 Git/SQLite、受控正式 publication/provider/service；`go test ./tests/integration -run '^TestCD67AdditionalImplementation/neutral_fixed_input_restart_and_fresh_manual_answer$' -count=1` | API presentation/查询路由接线后 | 正式 provider merge 生成的持久 TaskDelivery，经 HTTP ShowTask 与 Snapshot 与既有 native Query 的 PRURL/Workflow/Execution/MergedAt 一致；不手写 Delivery/transport record，不访问真实外部 PR | 临时目录 `api-delivery-query.log`；原 Query/HTTP 输出和摘要写本 acceptance |
| L1 固定组 | Windows；go run ./cmd/test-feedback -level=L1 | 定向通过、合入前一次 | passed/complete=true；共享 Query/Command 与宿主边界保持 | runner 默认临时目录，acceptance 记录实际位置 |
| Go 静态 | Windows；go vet ./internal/product/httpapi ./internal/app/... | 实现完成后 | 无新增诊断 | 临时目录 api-vet.log |
| 文档静态 | Python/Git；python scripts/check_doc_links.py；python scripts/roadmap_task_id.py check；git diff --check | 本次规划修订及实现交付前 | 导航、身份及格式通过 | 规划修订见 roadmap-planning；实现见专属 acceptance |

不选择 Full、完整 L2/L3 或 live：本任务复用执行语义，只改变传输。若实际修改原生发送/恢复行为，再选择受影响的最小既有场景并说明原因。协议样例与负向授权测试使用正式路由，不增加测试专用生产写入口。构建/命令/临时证据根执行时登记，本次文档修订不执行产品测试。

API 查询/身份定向命令（随实现交付所列测试）：

```powershell
go test ./internal/product/httpapi -run '^TestAPI(IdentityAndAuthentication|QueryParity|ReconnectAndUnsupported|ServiceLifecycle)$' -count=1
```
