# 用量事实与计费职责划分实施 Plan

日期：2026-11-05。依据 [Spec](../spec/billing-ownership.spec.md)。状态：已按用户授权实施两仓变更，隔离验证和证据收尾中；不纳入 roadmap，不部署安装版。指定安装候选只读诊断与新隔离远端 Run 的验证分开登记。

## 1. 交付顺序与所有权

| 步骤 | 交付内容 | 前置与完成条件 |
| --- | --- | --- |
| P1 Runner 公开契约交接 | 以现有run-usage/v1及run-usage-events为基线，核对Spec §3的事实/裁定/分页语义与必要增量、默认采集已核实远端会话及删除估价职责；固定正式CLI/schema/构建 | 不强制新建v2或重写既有分页；正式目标契约、commit、EXE SHA/version和矩阵交付前，不接目标真实链 |
| P2 SteerLine 计算 owner | 新增 internal/billing 的中性输入模型、匹配、精确十进制计算、状态与原因；不依赖 runnerclient、存储、Web 或 execution 业务状态 | 依 Spec 可先开发；接口与 P1 最终事实模型对齐后定稿；BILL-A04 通过 |
| P3 配置 owner | 建议新增 internal/billingconfig，承接 `<home>/billing.json` schema、版本、摘要、跨进程条件保存/原子替换和轻量 status | 独立于 Runner 和项目，复用既有 privatefs/pathguard；BILL-A03 的存储/并发/隔离通过 |
| P4 Runner 客户端和候选查询 | runnercontract/runnerclient接入既有run-usage-events与正式必要增量，消费decision/metadata_issues；internal/app组合原候选校验、固定价表与完整快照，调用billing；更新API/客户端/HTTP | P1 正式构建与 P2/P3 完成；BILL-A02/A05，通过真实公开调用前核对 pin |
| P5 Web 设置和展示 | 全局计费入口、版本/费率编辑和切换，更新候选用量费用 DTO、明细与价表变更失效机制 | P3/P4 DTO 固定；BILL-A06 自动通过，零项目/离线设置可用 |
| P6 删除旧链与联验 | 删除旧价表路径/Runner估价依赖，更新配置/示例/文档，完成固定Runner联验和回归 | P1–P5及P7/P8集成完成，全部自动项闭合后才可报告实现完成 |
| P7 远端采集设置与诊断 | 原executionconfig owner提供usage_policy编辑与条件保存；usage_policy只保留采集开关；Runner根据原绑定自动核实主机/会话叶目录；应用查询端口投影有效/待重启状态 | P1默认读取和范围校验先交付；不把采集开关放billing.json、不热更新；BILL-A09通过 |
| P8 远端真实联验 | 新隔离home/固定构建验证默认自动采集、正式停启、原绑定/Token展示、重读去重、关闭采集保留历史与回收；形成指定安装Run的恢复条件清单 | P4/P5/P7和新驱动完成；BILL-A10通过；不在安装现场调试，不自动改当前安装策略 |

表格是实施依赖，不创建任务、子聊天或并行 worker。P1 是另一仓库的交付，不由当前文档变更替 Runner 宣布完成。P2/P3 可在等待 P1 时推进，但其夹具不能释放 P4 的真实接口前置。

## 2. 接线落点

1. **Runner 交接包**：先逐字段核对现有run-usage-events；保留event_id/source_ref、decision、metadata_basis/time_basis、metadata_issues和--after分页，不另建语义相同接口。只对interval/计数等事实缺项安排必要增量。要求正式schema、同revision样例/过期错误、来源去重和修正证据；确认哪些adapter只支持Token、哪些能完整计价。记录Runner删除估价职责的实际差异。
2. **纯计价**：使用固定合成费率验证算术，不引用合成费率作为真实供应商报价。规则精确匹配 provider/model 和有效区间；时间跨界/未知、字段不全和明细契约错误分开。12 位单价精度、整型计数范围、精确金额及响应边界统一定义，不引入浮点路径。
3. **配置**：以 billingconfig 为唯一 schema 与文件访问 owner，Web 和应用 reader 共用它。第一版直接采用 billing.json 新格式。首次保存 create-only，后续 If-Match 条件写；版本保留、active 切换与 generation 在同一文件原子提交。执行配置保存不写 billing.json，也不再保留 pricing_policy 字段。
4. **设置传输**：沿既有全局配置模式，在 Web 侧拟定 `GET /api/web/billing-config`（返回 ETag/当前版本）、`POST /api/web/billing-config/validate`、`PUT /api/web/billing-config`（条件保存新版本）、`POST /api/web/billing-config/activate`（条件切换已有版本）、`GET /api/web/billing-config/status`（轻量摘要/generation）。创建使用 If-None-Match: *，更新/切换使用 If-Match，缺失前置拒绝；写操作沿会话/Origin/CSRF。实际路径/schema 在 P3/P5 接线前固定；这些接口只调用 billingconfig，不启动应用/项目/Runner。应用候选查询从同一 owner 读不可变版本，不依赖 Web 进程在线。
5. **候选查询**：沿现有 `GET /runs/:candidate/usage?execution_id=:runnerRun`，继续核对项目/候选/原 Run/state-dir/实例。开头固定设置版本，再读快照及全页；取得完整用量集合再计算，不边到页边回总额。页级错误保留有效 summary，单列费用查询失败。Runner 不可用时不创建目录或自启服务。
6. **结果与缓存**：SteerLine 费用 DTO 使用完整/部分/未计价 record 数，不延用 priced_responses 计量 interval。携带 policy/version/digest、generation、计算规则版本、usage revision、scope、金额/原因/假设和聚合匹配信息。前后端核对请求身份和版本，价表变更失效后即使 Token 轮询已停止仍重算；跨标签页用 status 检查，固定预算并避免与候选轮询各维护一套用量采集。
7. **展示**：全局设置默认给可填写空表，不预填现价；执行页原摘要和 details 上最小增量展示。仅候选主会话，不增加汇总页、其他 Run 入口或新执行按钮。配置未保存、切换冲突、价格缺失、旧结果刷新失败与真实零各有明确状态；覆盖内容以 Spec §6 为准。
8. **旧链清理**：删除 internal/executionconfig.PricingPolicy、runnerclient.RunCostSupported/EstimateRunCost、runnercontract.CostEstimate 及直接映射；清理绑定命令白名单/catalog 的旧能力、前后端配置字段/fixture/测试/示例；保留 usage_policy 和原服务启动授权。旧配置明确拒绝，不能自动导入旧 Runner 价表。更新 docs/integration/steerrunner.md、源码索引、配置导航、路径清单和相关当前说明；旧验收保持原版本结果。
9. **远端默认采集（2026-11-09 用户修订）**：移除逐目录远端授权配置和授权需求查询；usage_policy仅保留默认开启的enabled。Runner自动按原Run/主机/会话确定最小日志叶目录，保留来源/链接验证、采集许可和临时访问回收。Web只提供采集开关，配置保存/待重启及按实例失效语义保持。两仓更改与回归分别取证，旧字段拒绝，不修改安装现场。详见[本轮验收](../testing/acceptance/remote-usage-default.md)。

不更改 internal/prompts、提示词材料/接线或其黄金样本；本计划的新增测试不通过修改提示词期望来消除失败。也不增加数据库 schema 或原生协议解析器。

## 3. 实施时的测试计划

具体步骤、断言、当前结果及证据登记于[专属验收](../testing/acceptance/billing-ownership.md)。以下是实施必选范围，实际结果以专属验收为准。每行均须实际非零用例、零意外 skip、完整终态/退出码；子集通过不表述为整个登记集通过。

| 验收项 | 测试 ID / 环境 | 命令或待补入口、时点 | 最小通过条件 |
| --- | --- | --- | --- |
| BILL-A01 | Runner 自有公开契约/原生 owner；独立环境 | P1在Runner仓库执行；命令、构建和证据随交接固定 | 既有公开能力可复用，必要增量/矩阵/归一化/裁定/分页证明齐全；删除其计价职责 |
| BILL-A02 | 定向诊断 / isolated | P4 实现后 `go run ./cmd/test-feedback -out <新diagnostics目录> -- ./internal/runnerclient -count=1` | 原身份、逐页实例绑定、同快照、重复/乱页/summary不符、过期/预算/取消拒绝；零估价/补采/执行调用 |
| BILL-A03 | 定向诊断 / isolated | P3 实现后 `go run ./cmd/test-feedback -out <新diagnostics目录> -- ./internal/billingconfig -count=1` | 初次保存/并发条件/原子失败/版本/区间/路径/私有权限/无项目/离线/错误隔离通过 |
| BILL-A04 | 定向诊断 / isolated | P2 实现后 `go run ./cmd/test-feedback -out <新diagnostics目录> -- ./internal/billing -count=1` | 专属验收的纯函数矩阵及独立人工可算期望值正确，确定性和溢出/精度边界成立 |
| BILL-A05 | 定向诊断 / isolated | P4/P6 后 `go run ./cmd/test-feedback -out <新diagnostics目录> -- ./internal/app ./internal/product/httpapi ./internal/product/applicationhttp ./internal/executionconfig -count=1` | 候选绑定/协议、价表原子选择、故障保 Token、配置变化不改执行、删除旧链与 usage_policy 保留 |
| BILL-A06 | `web.unit` / isolated；Web HTTP owner | P5 后 `go run ./cmd/test-feedback -level=L4 -suite=web -scenario web.unit -out <新L4目录>`，并执行下一行 go.http；浏览器扩展既有 `web/tests/smoke.spec.ts`，在新 simulate 包设置独立 home/config/evidence 后执行 `npm --prefix web run test:e2e -- smoke.spec.ts -g web.smoke.billing`，finally 停止并核实清理 | 设置保存/切换、零项目、显示/部分/未知/零/精度、缓存/取消/跨标签页、宽窄屏与键盘可达性自动断言；构建成功不能替代浏览器 |
| BILL-A07 | `go.runner-contract` / isolated，固定独立 EXE | P6 扩展既有 runneracceptance 公开契约测试后执行 `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.runner-contract -out <新L4目录>` | 明确 STEERRUNNER_EXE/VERSION/SHA256；用量契约与受控账本 fixture 分层验证，经真实 CLI 固定快照/明细，调用方 fixture 得到预期费用；真实原生样本缺请求时间须保持未知；不支持/未知/停止清理有证据 |
| BILL-A08 | `l1.minimum`、`smoke.delivery`、`go.http`、`web.unit` / 清单原环境 | 集成后与提交前：固定 `go run ./cmd/test-feedback -level=L1 -out <新L1目录>`、`go run ./cmd/test-feedback -level=L2 -out <新L2目录>`；`go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.http -out <新L4目录>`；web.unit 复用 A06 同源码结果 | 完整回归非零/无skip，旧费用链消失、采集默认启用且执行控制不变；生产前端/Go 构建和文档检查通过 |
| BILL-A09 | 定向诊断 + go.http/web.unit / isolated | P7后 `go run ./cmd/test-feedback -out <新diagnostics目录> -- ./web/server/executionconfig ./internal/app ./internal/product/applicationhttp -count=1`；复用A06/A08同源码Web/HTTP报告 | 默认开启且无需目录授权、已核实最小根/主机校验、跨进程条件保存、待重启/有效策略、实例/策略变化/迟到缓存、访问回收状态均按Spec §3.4验证；读取查询零自动停服/执行输入 |
| BILL-A10 | `usage.remote-codex` / L4 live，已登记 | `go run ./cmd/test-feedback -level=L4 -suite=usage -env=live -scenario usage.remote-codex -out <新目录>`；显式 STEERRUNNER_EXE/VERSION/SHA256、STEERLINE_USAGE_LIVE_INPUT（冻结 execute/1 原文）和 STEERLINE_USAGE_LIVE_ROOT（不得存在）；新 home/固定 EXE/明确远端环境 | 默认自动采集同原绑定Token并正确展示，正式停启保留绑定，关闭采集保留历史，明细/summary同revision且重读不重复；回收/资源清理核实。现场缺日志等事实失败不能改称通过 |

新 billing/billingconfig owner 首先以定向诊断验证，再接入既有普通 Go 测试归属。新增 usage.remote-codex 属于 localSuites 的 runneracceptance live 组，已同步执行器、test-list.json 和 L4 列表，不伪装成 tests/e2e/suites.json 场景。公开三测试沿 go.runner-contract；可计价账本夹具联验为新增 TestBillingUsagePublishedFixture，另以定向 runneracceptance 命令运行（需 STEERRUNNER_BILLING_FIXTURE），由 Runner 的 billingacceptance owner 在新目录创建，不能当作原生取源证明。浏览器扩展已有 smoke.spec.ts 的 web.smoke.billing，使用零项目 home 显式维持 serve 和 Web 进程，finally 停止；不依赖退出即回收子进程的短启动器。

本次设计不改SQLite，故不选择go.storage；若实现改变该边界须补go.storage。不增加真实PR交付、完整原生采集/远端权限恢复矩阵或默认Go Full。Runner原生事实来源验证由P1交接，A07只验公开边界/消费闭环，A10为本次新增远端配置修复的最小真实链，不用fixture替代；真实远端请求/环境/模型已在隔离执行前固定，详见专属验收；不影响安装现场。

人工 BILL-M01 只检查主观信息清晰度和体验，窗口待安排，可后置；核心计算、字段缺失及自动化失败不能改列人工待验。

## 4. 输出与证据

所有执行使用 [paths.md](../paths.md) 的新外部 scope，例如 `C:\SteerlineDebug\wt-1148-billing\test-results\<级别>\<唯一运行ID>`，定向调用放 diagnostics；每次调用新目录，Web/构建/压力与共享源码写入按规则串行。固定源码 SHA、Runner commit/EXE SHA/version、配置版本摘要、命令、scope 和清理事实写专属验收；失败和补验分开保留，不改安装目录。

2026-11-05 初始文档阶段对安装候选只读诊断；随后用户已授权普通开发实施。实现与验证在当前 worktree、独立 Runner 副本及新外部 home 完成，不部署安装版。实际代码/契约、构建、失败与补验、命令和清理见专属验收。原安装 Run 的恢复条件单列，不能用新隔离 Run 的通过冒充原现场已回填。
