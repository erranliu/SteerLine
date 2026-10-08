# Plan：本机客户端协议与历史查询

任务 ID：45；任务名称：本机客户端协议与历史查询；目标：`PHASE2.1`。执行范围见 [Spec](phase2-local-api.spec.md)，结果记录于[专属验收](../acceptance/phase2-local-api.md)。无未完成硬前置，一个实现 PR。正式实施由 roadmap-runner 核对 execution-map 的范围与前置，启动时固定实际 Spec/Plan commit 和路径。

## 实施与所有权

1. 对照现有 product/api、client、query 和控制台消费者，固定执行 DTO/action 映射；沿当前身份和 CommandService，不复制领域规则。
2. 在 internal/product/httpapi 交付传输适配、身份与发现、认证、查询和命令路由；这是拟新增实现位置，不先创建空包。app/host 只组装和承载，不推进业务状态。
3. 复用持久命令回执验证重复、未登记和未知结果窗口，提供客户端原信封查询/补交样例。用查询刷新闭环，不建设 SSE 或离线 CLI。
4. 交付 #44 可调用的身份/查询样例与 Windows 启动说明。宿主接线由本任务负责；若 #83 先合入，按其 exe/home 入口接线，若后合入则复核双方路径。共享接线串行合入，不虚构 DAG 边。
5. 完成 API1–API5，保留失败和未验项；不运行新的模型交付来验证纯传输。

## 测试计划

以下新测试名和 httpapi 包在实现中一并交付；尚未存在，不宣称已运行。

| 级别 / 场景 ID | 环境与命令 | 时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- |
| 定向 Go / API1、API2、API4、API5 | Windows、真实临时服务/Git/SQLite、受控后端；下方 API 查询/身份定向命令 | 路由接线完成后 | 非零命中；ACL/实例/查询一致及诊断断言通过 | 本次临时目录 api-targeted.log；摘要写专属 acceptance |
| 定向接线 / API3 | 同上；go test ./internal/product/httpapi -run '^TestAPIOriginalCommandReceipt$' -count=1 | 命令接线后 | 注入未登记、丢回执及迟到提交，原 ID 仲裁至多一个副作用 | 临时目录 api-command.log 与身份/回执摘要 |
| L1 固定组 | Windows；go run ./cmd/test-feedback -level=L1 | 定向通过、合入前一次 | passed/complete=true；共享 Query/Command 与宿主边界保持 | runner 默认临时目录，acceptance 记录实际位置 |
| Go 静态 | Windows；go vet ./internal/product/httpapi ./internal/app/... | 实现完成后 | 无新增诊断 | 临时目录 api-vet.log |
| 文档静态 | Python/Git；python scripts/check_doc_links.py；python scripts/roadmap_task_id.py check；git diff --check | 本次规划修订及实现交付前 | 导航、身份及格式通过 | 规划修订见 roadmap-planning；实现见专属 acceptance |

不选择 Full、完整 L2/L3 或 live：本任务复用执行语义，只改变传输。若实际修改原生发送/恢复行为，再选择受影响的最小既有场景并说明原因。协议样例与负向授权测试使用正式路由，不增加测试专用生产写入口。构建/命令/临时证据根执行时登记，本次文档修订不执行产品测试。

API 查询/身份定向命令（随实现交付所列测试）：

```powershell
go test ./internal/product/httpapi -run '^TestAPI(IdentityAndAuthentication|QueryParity|ReconnectAndUnsupported|ServiceLifecycle)$' -count=1
```
