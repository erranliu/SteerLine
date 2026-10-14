# 验收：HTTP 服务端命令边界收敛

唯一归属：任务 #92，HTTP 服务端命令边界收敛。2026-10-14经用户确认纳入[execution-map](../roadmap/execution-map.md)。[PR #311](https://github.com/erranliu/SteerLine/pull/311)仍在修复/复审中：本轮补齐HC92-A1结构守卫，但独立复审发现原生查看的就绪竞态仍未修复；本任务不得记为实现完成。

[Spec](../spec/http-command-boundary.spec.md) · [Plan / 测试计划](../spec/http-command-boundary.plan.md) · [草稿](../roadmap/drafts/roadmap.md)

## 自动验收计划与结果

| ID | 具体行为/最小通过条件 | 方法、环境和安排 | 状态 |
| --- | --- | --- | --- |
| HC92-A1 | 直接调用登记owner并检查handler构造；小端口stub能替代完整Client/SQL/HTTP对象，原重复实现删除；本项专属规则检查Server/registry依赖与单一记录owner | 新增`TestHTTPCommandBoundary`和AST正/负例；普通Windows Go；提取后、PR前执行 | 本轮通过：1包/7测试，0失败/跳过；守卫只验证同包源码结构，不声称编译隔离 |
| HC92-A2 | 登记并发同ID/异内容/跨动作；派发计数及原intent、正式消费者竞争、错绑定、rejection、读取故障/unknown与原信封补交核对 | 新增registry stub + 原authority/回执反例；私有记录stub；接线后、PR前执行 | 通过：1包/28测试，0失败/跳过 |
| HC92-A3 | 原生查看重复/丢结果、停服请求、边界记录/认证和真实服务接线；查看副作用一次、停服回执、大小/原子性及业务owner保持 | 既有httpapi及`tests/integration`定向入口；隔离HTTP/SQLite/app，无真实模型；接线后、PR前执行 | 未通过：既有HTTP/app回归通过，但队列等待期间服务降为not-ready后仍可能打开原生会话；修复并定向复验前阻塞 |

固定L1历史回归：`go run ./cmd/test-feedback -level=L1`在原实现候选`72e1937d`上通过且完整，12组、26包、553测试通过，0失败/跳过；本轮只增结构守卫并将测试夹具写入助手移至`_test.go`，未重跑L1。此历史回归不解除HC92-A3的未修复竞态。组别及测试数见原L1 summary。

## 执行环境与命令

- 日期：2026-10-14（日志时间UTC）。本轮固定Spec/Plan为`35d4d65350fa6f6f284e9e91a6bd657ba7abb947`；本轮测试起点commit为`006d0ebdf2eca57bb3d7b56d42c11db32ffd611a`，守卫候选commit为`100413eaa5886990d573b946db159e1383bbe5c6`。Runner记录source_commit为`006d0ebd`，工作树差异正是该候选提交；未改ready采样/开窗生产路径。环境：Windows/amd64，Go 1.26.4；无启用原生验收开关。
- HC92-A1：`go run ./cmd/test-feedback -- ./internal/product/httpapi -run '^TestHTTPCommandBoundary$' -count=1`
- HC92-A2：`go run ./cmd/test-feedback -- ./internal/product/httpapi -run '^(TestHTTPCommandRegistry|TestAPIAuthoritativeControlBinding|TestAPIAuthoritativeSQLiteBinding|TestAPIFirstRegistrationBindsMappedControlIntent|TestAPIFormalConsumerWinsRegistrationWindow|TestAPIFormalControlContentCollisionIsIdentityConflict|TestAPIOriginalCommandReceipt|TestAPIReconnectAndUnsupported)$' -count=1`
- HC92-A3 HTTP：`go run ./cmd/test-feedback -- ./internal/product/httpapi -run '^(TestTUIHTTPRoutes|TestAPIServiceLifecycle|TestAPIIdentityAndAuthentication|TestAPIRequestRecord.*)$' -count=1`
- HC92-A3 app接线：`go run ./cmd/test-feedback -- ./tests/integration -run '^(TestAPIConfiguredServiceDiscardsTaskThroughOwnedWriter|TestAPIOriginalCommandReceipt)$' -count=1`
- 本轮HC92-A1结构守卫及handler：`go run ./cmd/test-feedback -- ./internal/product/httpapi -run '^(TestHTTPCommandBoundary|TestHTTPCommandBoundaryArchitecture|TestHTTPCommandBoundaryArchitectureRejectsPrivateFixtures)$' -count=1`
- 本轮HC92-A2记录助手移至测试代码后的仲裁回归：使用上方HC92-A2命令。
- 既有依赖边界：`go run ./cmd/test-feedback -- ./tests/integration -run '^TestCD67CoreBoundary$' -count=1`
- 固定L1：`go run ./cmd/test-feedback -level=L1`

## 原始证据

Runner保留各`summary.json`、Go JSON事件日志和stderr。以下目录位于本机临时目录：

- HC92-A1：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T070551.999402800Z\`，summary：`summary.json`。
- HC92-A2：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T070605.427383900Z\`，summary：`summary.json`。
- HC92-A3 HTTP：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T070624.440413700Z\`，summary：`summary.json`。
- HC92-A3 app接线：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T070642.563530700Z\`，summary：`summary.json`。
- 固定L1：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T070921.437919100Z\`，summary：`summary.json`；组级Go JSON/stderr记录在同目录各组子目录。

### 独立复审后的补验与未解决项

本轮新增证据基于`006d0ebd`起点、测试时工作树差异最终提交为`100413ea`。专属AST守卫遍历`httpapi`目录全部非测试Go源码，检查Server不导入具体product client、commandRegistry的字段/构造与方法不接受或保存Server、HTTP请求/响应/客户端、product client、SQL连接、net.Conn或os.File等宽能力；同时检查记录格式/owner唯一、commandRegistry只经`requestRecordAccess`访问记录、文件I/O不绕过owner。负例覆盖具体Client、各类宽类型、直接读记录/文件、重复record owner。记录夹具用的`writeRequestRecord`仅留在测试源码。上述是同包AST结构守卫，不表示Go编译器提供了文件级隔离。

- A1守卫与handler命令：`go run ./cmd/test-feedback -- ./internal/product/httpapi -run '^(TestHTTPCommandBoundary|TestHTTPCommandBoundaryArchitecture|TestHTTPCommandBoundaryArchitectureRejectsPrivateFixtures)$' -count=1`；1包、7测试通过，complete=true、无失败/跳过。summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T074406.974468400Z\summary.json`；JSON/stderr：同目录`go-test.jsonl`、`go-test.stderr.txt`。
- A2仲裁回归（包含此前authority和回执反例）：`go run ./cmd/test-feedback -- ./internal/product/httpapi -run '^(TestHTTPCommandRegistry|TestAPIAuthoritativeControlBinding|TestAPIAuthoritativeSQLiteBinding|TestAPIFirstRegistrationBindsMappedControlIntent|TestAPIFormalConsumerWinsRegistrationWindow|TestAPIFormalControlContentCollisionIsIdentityConflict|TestAPIOriginalCommandReceipt|TestAPIReconnectAndUnsupported)$' -count=1`；1包、28测试通过，complete=true、无失败/跳过。summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T074214.578444500Z\summary.json`；JSON/stderr：同目录`go-test.jsonl`、`go-test.stderr.txt`。
- 既有依赖边界命令：`go run ./cmd/test-feedback -- ./tests/integration -run '^TestCD67CoreBoundary$' -count=1`；1包、37测试通过，complete=true、无失败/跳过。summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T074030.443987700Z\summary.json`；JSON/stderr：同目录`go-test.jsonl`、`go-test.stderr.txt`。
- 独立只读复审在clean head `006d0ebd`上的43项HTTP定向回归与2项app集成回归通过；HTTP summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T072726.699686200Z\summary.json`；app summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T073016.966730700Z\summary.json`。本轮未重复这两组既有回归。
- 独立复审的原生就绪竞态overlay失败：报告命令为`go run ./cmd/test-feedback -- ./internal/product/httpapi -overlay <overlay.json> -run '^TestReview92QueuedNativeRechecksReadiness$' -count=1`；overlay源码与配置为`C:\Users\lmas1\AppData\Local\Temp\http92-review-64fba3c9316e4ff18edc3d6f0e556f0d\review_native_readiness_test.go`、同目录`overlay.json`；失败summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T072857.882122200Z\summary.json`，Go事件：同目录`go-test.jsonl`。复现时请求先在registry锁外采样`Ready=true`并等待同一仲裁锁，期间`SetReady(false)`，随后仍将过期`true`传给`OpenNative`；实际副作用计数1、返回`applied`/HTTP 200。`commands.go`与registry就绪路径本轮保持不变；需由后续独立修复并重新验收。

### 失败及修正记录

保留早期失败记录；通过的是后续同命令的完整重跑。

1. HC92-A1第一次运行，summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T065702.425165800Z\summary.json`。构建失败、0个测试；`command_registry.go`缺少`reflect`导入，且旧测试仍调用已移交的`Server.controlInput`。补入导入并将测试调用改为registry owner后重跑。
2. HC92-A1第二次运行，summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T065751.113431900Z\summary.json`。构建通过但1个测试失败；测试authority stub返回未包装的`api.ErrRecordNotFound`，不符合生产查无记录时使用的`api.Error{Kind: api.UnknownRecord}`契约。修正stub后重跑通过。
3. 本轮首次结构守卫运行，summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T074030.428430400Z\summary.json`。7测试中6通过、1失败、无跳过；守卫误以为file owner只在endpoint构造一次，忽略既有`state.go`命令目录验证也构造同一owner。守卫修正为要求唯一owner实现并允许endpoint/state两个既有构造点；随后完整7测试通过。

## 人工与阻塞范围

无新增人工项或窗口；#91原人工体验仍归原记录，不阻塞本项。当前阻塞为独立复审确认的原生就绪竞态，尚无修复或复验；交付前必须修复并按HC92-A3复验。未触发产品/live原生验收。固定Spec/Plan未修改。
