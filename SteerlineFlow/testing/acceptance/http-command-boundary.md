# 验收：HTTP 服务端命令边界收敛

唯一归属：任务 #92，HTTP 服务端命令边界收敛。2026-10-14经用户确认纳入[execution-map](../../Roadmap/execution-map.md)。[PR #311](https://github.com/erranliu/SteerLine/pull/311)已完成独立复审修复、最小自动验证及真实合并核验，实现完成；HC92-A1–A3通过，无新增人工项。历史失败与每轮源码/证据保留如下。

[Spec](../../spec/http-command-boundary.spec.md) · [Plan / 测试计划](../../plans/http-command-boundary.plan.md) · [草稿](../../Roadmap/drafts/roadmap.md)

## 自动验收计划与结果

| ID | 具体行为/最小通过条件 | 方法、环境和安排 | 状态 |
| --- | --- | --- | --- |
| HC92-A1 | 直接调用登记owner并检查handler构造；小端口stub能替代完整Client/SQL/HTTP对象，原重复实现删除；本项专属规则检查Server/registry依赖与单一记录owner | 新增`TestHTTPCommandBoundary`和AST正/负例；普通Windows Go；提取后、PR前执行 | 本轮通过：1包/7测试，0失败/跳过；守卫只验证同包源码结构，不声称编译隔离 |
| HC92-A2 | 登记并发同ID/异内容/跨动作；派发计数及原intent、正式消费者竞争、错绑定、rejection、读取故障/unknown与原信封补交核对 | 新增registry stub + 原authority/回执反例；私有记录stub；接线后、PR前执行 | 通过：1包/28测试，0失败/跳过 |
| HC92-A3 | 原生查看重复/丢结果、停服请求、边界记录/认证和真实服务接线；查看副作用一次、停服回执、大小/原子性及业务owner保持 | 既有httpapi、`tests/integration`与CLI实际console定向入口；隔离HTTP/SQLite/app，无真实模型；接线后、PR前执行 | 本地候选通过：排队/动态就绪11项、HTTP14项、app2项及CLI/HTTP console 3项均通过，无失败/跳过；原失败证据保留，待独立复审 |

固定L1历史回归：`go run ./cmd/test-feedback -level=L1`在原实现候选`72e1937d`上通过且完整，12组、26包、553测试通过，0失败/跳过。结构守卫补齐轮未重跑L1；原生就绪修复轮复用该历史回归并执行下方最小定向复验，未重跑L1。历史L1不替代新竞态回归。组别及测试数见原L1 summary。

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

### 独立复审后的结构守卫补验与当时未解决项

本轮新增证据基于`006d0ebd`起点、测试时工作树差异最终提交为`100413ea`。专属AST守卫遍历`httpapi`目录全部非测试Go源码，检查Server不导入具体product client、commandRegistry的字段/构造与方法不接受或保存Server、HTTP请求/响应/客户端、product client、SQL连接、net.Conn或os.File等宽能力；同时检查记录格式/owner唯一、commandRegistry只经`requestRecordAccess`访问记录、文件I/O不绕过owner。负例覆盖具体Client、各类宽类型、直接读记录/文件、重复record owner。记录夹具用的`writeRequestRecord`仅留在测试源码。上述是同包AST结构守卫，不表示Go编译器提供了文件级隔离。

- A1守卫与handler命令：`go run ./cmd/test-feedback -- ./internal/product/httpapi -run '^(TestHTTPCommandBoundary|TestHTTPCommandBoundaryArchitecture|TestHTTPCommandBoundaryArchitectureRejectsPrivateFixtures)$' -count=1`；1包、7测试通过，complete=true、无失败/跳过。summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T074406.974468400Z\summary.json`；JSON/stderr：同目录`go-test.jsonl`、`go-test.stderr.txt`。
- A2仲裁回归（包含此前authority和回执反例）：`go run ./cmd/test-feedback -- ./internal/product/httpapi -run '^(TestHTTPCommandRegistry|TestAPIAuthoritativeControlBinding|TestAPIAuthoritativeSQLiteBinding|TestAPIFirstRegistrationBindsMappedControlIntent|TestAPIFormalConsumerWinsRegistrationWindow|TestAPIFormalControlContentCollisionIsIdentityConflict|TestAPIOriginalCommandReceipt|TestAPIReconnectAndUnsupported)$' -count=1`；1包、28测试通过，complete=true、无失败/跳过。summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T074214.578444500Z\summary.json`；JSON/stderr：同目录`go-test.jsonl`、`go-test.stderr.txt`。
- 既有依赖边界命令：`go run ./cmd/test-feedback -- ./tests/integration -run '^TestCD67CoreBoundary$' -count=1`；1包、37测试通过，complete=true、无失败/跳过。summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T074030.443987700Z\summary.json`；JSON/stderr：同目录`go-test.jsonl`、`go-test.stderr.txt`。
- 独立只读复审在clean head `006d0ebd`上的43项HTTP定向回归与2项app集成回归通过；HTTP summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T072726.699686200Z\summary.json`；app summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T073016.966730700Z\summary.json`。本轮未重复这两组既有回归。
- 独立复审的原生就绪竞态overlay失败：报告命令为`go run ./cmd/test-feedback -- ./internal/product/httpapi -overlay <overlay.json> -run '^TestReview92QueuedNativeRechecksReadiness$' -count=1`；overlay源码与配置为`C:\Users\lmas1\AppData\Local\Temp\http92-review-64fba3c9316e4ff18edc3d6f0e556f0d\review_native_readiness_test.go`、同目录`overlay.json`；失败summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T072857.882122200Z\summary.json`，Go事件：同目录`go-test.jsonl`。复现时请求先在registry锁外采样`Ready=true`并等待同一仲裁锁，期间`SetReady(false)`，随后仍将过期`true`传给`OpenNative`；实际副作用计数1、返回`applied`/HTTP 200。`commands.go`与registry就绪路径本轮保持不变；需由后续独立修复并重新验收。

### 原生就绪竞态修复与复验

2026-10-14，固定Spec/Plan仍为`35d4d65350fa6f6f284e9e91a6bd657ba7abb947`。修复及测试起点为clean head `4eb626fbc25dfdeed16b20d14c4e7a7ddcb7c52c`；本轮runner记载该head与4个httpapi源码/测试文件的dirty差异，测试期间这些文件未再修改。环境为Windows/amd64、Go1.26.4、隔离fixture，无原生验收开关。

handler不再在共同仲裁锁外传入`Ready`布尔值；registry取得同一仲裁锁后读取现有生命周期状态并查询运行时快照。原实例匹配、连接状态、身份/配置匹配及service.Ready判断与identity路由共用纯值函数。没有引入Server/HTTP对象/具体Client或业务资格规则。私有锁字段使用`sync.Locker`，生产构造仍只创建一个共享`*sync.Mutex`；测试wrapper在Lock尝试时发信号并保留该锁，让请求排队→失去就绪→取得锁的顺序确定，不依靠sleep。原AST守卫及负例未改动。

- 新增`TestHTTPNativeQueuedReadiness`：分别在队列等待期间`SetReady(false)`、`Close`、运行时service.Ready变false，均按既有协议返回rejected/HTTP200，副作用为0；恢复后原持久拒绝不重放。
- 新增`TestHTTPNativeRuntimeReadiness`：本地ready保持true，缺快照、旧实例、运行时not-ready、断连、身份/配置不匹配均拒绝/零开窗；恢复后新ID可applied，重复原ID保持原回执且仅开窗一次。
- 新竞态/动态反例及A1守卫/handler：`go run ./cmd/test-feedback -- ./internal/product/httpapi -run '^(TestHTTPNativeQueuedReadiness|TestHTTPNativeRuntimeReadiness|TestHTTPCommandBoundary|TestHTTPCommandBoundaryArchitecture|TestHTTPCommandBoundaryArchitectureRejectsPrivateFixtures)$' -count=1`；1包18测试通过（新就绪11项、原A1 7项），complete=true、exit0、无失败/跳过。summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T080401.987312000Z\summary.json`。
- 同一新增排队回归对旧生产路径的确定性反例：只在overlay适配相同私有锁信号接缝，保留`4eb626fb`的锁外ready采样及旧OpenNative；命令`go run ./cmd/test-feedback -- ./internal/product/httpapi -overlay <overlay.json> -run '^TestHTTPNativeQueuedReadiness$' -count=1`，3子场景均失败为applied/HTTP200（含顶层4失败），complete=true、无skip。overlay：`C:\Users\lmas1\AppData\Local\Temp\http92-readiness-before-621b3ac9695c4dc5b937fd18dc22cb05\overlay.json`；summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T080452.105339400Z\summary.json`。该失败验证新回归确实捕获旧缺陷，不作为修复候选失败。
- A2共同仲裁复验：采用上方固定A2命令；因私有锁字段抽象化而重新确认普通/原生查看共同仲裁，1包28测试通过，complete=true、exit0、无失败/跳过。summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T080612.859215000Z\summary.json`。
- A3 HTTP：采用上方固定HTTP命令；1包14测试通过，complete=true、exit0、无失败/跳过。summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T080513.208375800Z\summary.json`。
- A3真实app接线：采用上方固定app命令；1包2测试通过，complete=true、exit0、无失败/跳过。summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T080513.217024200Z\summary.json`。

上述每个summary同目录保留`go-test.jsonl`、`go-test.stderr.txt`。源码证据另保存`C:\Users\lmas1\AppData\Local\Temp\http92-readiness-evidence-621b3ac9695c4dc5b937fd18dc22cb05\source-fingerprints.json`，包含测试起点、3个生产文件、新测试与未变AST守卫的Git blob及SHA256，交接时核对与实际提交一致。未执行Full、L2/L3/live，未使用安装环境。原失败/历轮证据完整保留；本轮仅解除已复现竞态的自动复验阻塞，仍需新的独立只读复审、同步PR及真实合并。

### 第二轮复审后的CLI Prepare消费者补修

独立复审在clean head `75177ff14753da33c810562ad466b30b83673c01`发现`cmd/steerline/native_task_test.go`仍用旧签名向`httpapi.Prepare`传完整client，导致CLI测试包不能编译。本轮只将该调用改为把同一个client填入`Query`、`Commands`、`Authority`、`NativeViewer`四个端口；原场景、断言和fixture setup语义未改。

已全仓审计所有点号形式的`.Prepare(`调用，区分`workspace.Prepare`和`consoleapp.Prepare`后，全部`httpapi.Prepare`调用均传`httpapi.Ports`：`cmd/steerline/native_task_test.go`、`tests/integration/api_protocol_test.go`三处、`internal/app/execution_service_wiring.go`及`internal/product/httpapi`测试helper/调用，无其他旧签名消费者。

- 独立复审的构建失败：命令`go run ./cmd/test-feedback -- ./cmd/steerline -run '^TestNativeTaskBatchCLIValidatesCreatesAndDiscardsFixedTaskIDs$' -count=1`，summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T082507.826331700Z\summary.json`，Go事件：同目录`go-test.jsonl`。`cmd/steerline`编译失败，0测试，错误为`cannot use client (*client.Client) as httpapi.Ports`。
- 修复后在相同命令上重跑：1包、3项通过，complete=true、exit0、无失败/跳过。source commit为`75177ff14753da33c810562ad466b30b83673c01`，测试时仅`cmd/steerline/native_task_test.go`有dirty差异，该差异完整提交为`01f70460ee644b2e7ff32a7aa2825024932772d8`。summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T084034.169629600Z\summary.json`；Go JSON/stderr：同目录`go-test.jsonl`、`go-test.stderr.txt`。本用例包含实际HTTP-backed console路径。
- 该消费修复不改生产接线或HTTP/console行为，因此复用独立复审对clean head `75177ff`的60项HTTP定向回归、2项app集成和固定L1验证；证据见上节与此前记录，本轮只补跑CLI真实调用场景。

### 失败及修正记录

保留早期失败记录；通过的是后续同命令的完整重跑。

1. HC92-A1第一次运行，summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T065702.425165800Z\summary.json`。构建失败、0个测试；`command_registry.go`缺少`reflect`导入，且旧测试仍调用已移交的`Server.controlInput`。补入导入并将测试调用改为registry owner后重跑。
2. HC92-A1第二次运行，summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T065751.113431900Z\summary.json`。构建通过但1个测试失败；测试authority stub返回未包装的`api.ErrRecordNotFound`，不符合生产查无记录时使用的`api.Error{Kind: api.UnknownRecord}`契约。修正stub后重跑通过。
3. 本轮首次结构守卫运行，summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T074030.428430400Z\summary.json`。7测试中6通过、1失败、无跳过；守卫误以为file owner只在endpoint构造一次，忽略既有`state.go`命令目录验证也构造同一owner。守卫修正为要求唯一owner实现并允许endpoint/state两个既有构造点；随后完整7测试通过。
4. 第二轮复审的CLI构建失败记录见`20261014T082507.826331700Z/summary.json`：旧consumer签名无法传入`Ports`，导致0测试执行。改为四端口组装后，用同一精确命令完整重跑通过，证据为`20261014T084034.169629600Z/summary.json`。

## 人工与阻塞范围

无新增人工项或窗口；#91原人工体验仍归原记录，不阻塞本项。原生就绪竞态、专属守卫遗漏和CLI消费者遗漏已修复，经独立终审与实际合并核验闭合。未触发产品/live原生验收。固定Spec/Plan未修改。

## 最终独立复审与真实合并

2026-10-14，最终reviewer `/root/review92_round3` 独立检查完整22文件diff、固定Spec/Plan、生产路径、全部Prepare消费者和原始验证，未发现新增问题。首轮reviewer `/root/review92_round1` 的就绪竞态由独立fixer `/root/fix92_native_readiness` 在 `75177ff14753da33c810562ad466b30b83673c01` 修复；专属AST守卫遗漏由原执行聊天在 `100413eaa5886990d573b946db159e1383bbe5c6` 补齐；第二轮reviewer `/root/review92_round2` 的CLI消费者遗漏在 `01f70460ee644b2e7ff32a7aa2825024932772d8` 修复。三项P2均已闭合，既有断言/边界未削弱。

最终审核head `b459a86d88f80b34f27140812ccc1755e232611e`、base `80f30b2253386c8e04ea6fa8a7b174037621880a`。reviewer在该clean head复跑上述CLI精确命令，3项通过、complete=true、exit0、无失败/skip，summary：`C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261014T090051.022098500Z\summary.json`；同目录Go JSON/stderr已核。HTTP60项、实际app2项复用clean `75177ff`独立复验，后续生产代码未改变；历史固定L1的12组/553项经逐组原始事件核验后复用，未冒称最终head重跑。原始失败与修复前overlay证据继续保留。

[PR复审记录](https://github.com/erranliu/SteerLine/pull/311#issuecomment-6165070946)记录agent身份、问题、修复提交及实际验证。合并前重新核head/base未变、MERGEABLE/CLEAN、REST main未保护、GraphQL保护规则/rulesets均0、无必需检查；按预期head调用合并。GitHub确认 `mergedAt=2026-10-14T09:20:31Z`（北京时间17:20:31）、merge `e3010edcb4905977e6244f2f896be140fa718a8f`；该merge已进入origin/main，两个父提交正是已核base与head。实现完成与自动验收通过分别登记；本项无人工残留，不扩大为Full/L2/L3/live或桌面体验证明。
