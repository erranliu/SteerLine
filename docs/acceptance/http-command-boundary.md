# 验收：HTTP 服务端命令边界收敛

唯一归属：任务 #92，HTTP 服务端命令边界收敛。2026-10-14经用户确认纳入[execution-map](../roadmap/execution-map.md)。实现及本地自动验收已完成；[PR #311](https://github.com/erranliu/SteerLine/pull/311)待独立复审和合并核验，本任务仍未记为实现完成。

[Spec](../spec/http-command-boundary.spec.md) · [Plan / 测试计划](../spec/http-command-boundary.plan.md) · [草稿](../roadmap/drafts/roadmap.md)

## 自动验收计划与结果

| ID | 具体行为/最小通过条件 | 方法、环境和安排 | 状态 |
| --- | --- | --- | --- |
| HC92-A1 | 直接调用登记owner并检查handler构造；小端口stub能替代完整Client/SQL/HTTP对象，原重复实现删除 | 新增`TestHTTPCommandBoundary`；普通Windows Go；提取后、PR前执行 | 通过：1包/1测试，0失败/跳过。结构检查见本次PR源码diff |
| HC92-A2 | 登记并发同ID/异内容/跨动作；派发计数及原intent、正式消费者竞争、错绑定、rejection、读取故障/unknown与原信封补交核对 | 新增registry stub + 原authority/回执反例；私有记录stub；接线后、PR前执行 | 通过：1包/28测试，0失败/跳过 |
| HC92-A3 | 原生查看重复/丢结果、停服请求、边界记录/认证和真实服务接线；查看副作用一次、停服回执、大小/原子性及业务owner保持 | 既有httpapi及`tests/integration`定向入口；隔离HTTP/SQLite/app，无真实模型；接线后、PR前执行 | 通过：HTTP 1包/14测试，app集成1包/2测试；均无失败/跳过 |

固定L1回归：`go run ./cmd/test-feedback -level=L1`，通过且完整。12个组、26个包、553个测试全部通过，0失败/跳过；选择范围为runner的固定L1清单，不含Full/L2/L3或live验收。组别及测试数：`quick-core` 4、`quick-rules` 384、`quick-execution` 7、`quick-execution-boundary` 61、`quick-owner-orca-terminal` 6、`quick-owner-codec` 14、`quick-owner-identity` 10、`quick-owner-codexapp` 4、`quick-owner-workspace` 2、`quick-owner-publication` 9、`quick-owner-app` 12、`quick-boundary` 40。

## 执行环境与命令

- 日期：2026-10-14（日志时间UTC）。固定Spec/Plan与测试源码起点：`35d4d65350fa6f6f284e9e91a6bd657ba7abb947`；验收实现候选提交：`72e1937d`（测试时工作树中的实现内容与该提交一致）。运行环境：Windows/amd64，Go 1.26.4；无启用原生验收开关。
- HC92-A1：`go run ./cmd/test-feedback -- ./internal/product/httpapi -run '^TestHTTPCommandBoundary$' -count=1`
- HC92-A2：`go run ./cmd/test-feedback -- ./internal/product/httpapi -run '^(TestHTTPCommandRegistry|TestAPIAuthoritativeControlBinding|TestAPIAuthoritativeSQLiteBinding|TestAPIFirstRegistrationBindsMappedControlIntent|TestAPIFormalConsumerWinsRegistrationWindow|TestAPIFormalControlContentCollisionIsIdentityConflict|TestAPIOriginalCommandReceipt|TestAPIReconnectAndUnsupported)$' -count=1`
- HC92-A3 HTTP：`go run ./cmd/test-feedback -- ./internal/product/httpapi -run '^(TestTUIHTTPRoutes|TestAPIServiceLifecycle|TestAPIIdentityAndAuthentication|TestAPIRequestRecord.*)$' -count=1`
- HC92-A3 app接线：`go run ./cmd/test-feedback -- ./tests/integration -run '^(TestAPIConfiguredServiceDiscardsTaskThroughOwnedWriter|TestAPIOriginalCommandReceipt)$' -count=1`
- 固定L1：`go run ./cmd/test-feedback -level=L1`

## 原始证据

Runner保留各`summary.json`、Go JSON事件日志和stderr。以下目录位于本机临时目录：

- HC92-A1：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T070551.999402800Z\`，summary：`summary.json`。
- HC92-A2：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T070605.427383900Z\`，summary：`summary.json`。
- HC92-A3 HTTP：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T070624.440413700Z\`，summary：`summary.json`。
- HC92-A3 app接线：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T070642.563530700Z\`，summary：`summary.json`。
- 固定L1：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T070921.437919100Z\`，summary：`summary.json`；组级Go JSON/stderr记录在同目录各组子目录。

### 失败及修正记录

保留早期失败记录；通过的是后续同命令的完整重跑。

1. HC92-A1第一次运行，summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T065702.425165800Z\summary.json`。构建失败、0个测试；`command_registry.go`缺少`reflect`导入，且旧测试仍调用已移交的`Server.controlInput`。补入导入并将测试调用改为registry owner后重跑。
2. HC92-A1第二次运行，summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T065751.113431900Z\summary.json`。构建通过但1个测试失败；测试authority stub返回未包装的`api.ErrRecordNotFound`，不符合生产查无记录时使用的`api.Error{Kind: api.UnknownRecord}`契约。修正stub后重跑通过。

## 人工与阻塞范围

无新增人工项或窗口；#91原人工体验仍归原记录，不阻塞本项。自动失败、共享契约未经确定或关键前置环境不可用阻止本项交付，不阻碍已独立验证的无关任务。未触发产品/原生验收。
