# SteerAgent 分类收敛 Plan

状态：已按用户后续指令实施，本期验收完成；范围及原始失败限制见专属验收。对应 [Spec](../spec/steer-agent-classification.spec.md)、[测试选择](steer-agent-classification.tests.json)和[专属验收](../testing/acceptance/steer-agent-classification.md)。普通开发；用户后续已授权提交、推送及创建 PR，不执行 roadmap、合并 PR 或部署。

## 1. 现有代码与整合选择

调查基线为 `a1451a16b3e7f4f025eff5c923cf7cbe13f63079`，另包含当前工作树长期文档及其审计修订。实施开始前核对源码与文档差异，保留本次既有文档；新增范围冲突先修订本 Spec/Plan，不借实施自行扩展。

| 已读取入口 | 结论与本期处理 |
| --- | --- |
| internal/mainagent/contract、advisory.go | 现有纯协议/核心已满足建议边界；只移动代码归属，保留当前协议及序列化，不增加动作 |
| internal/mainagentbridge、app/main_agent*.go | 原宿主持有 Run/请求/停止信封和共享队列；移动及更新所有组装、恢复和结算消费者 |
| internal/executionconfig、web/server/executionconfig/main_agent.go、Web 设置及共享 API 类型 | main_agent 专用覆盖已存在；只将键/类型按能力命名为 continuation_suggestion，沿原值、校验、compose/native-catalog 和条件保存 |
| internal/roadmap/generation.go、documents.go、product/query/async_roadmaps.go | 解析与文档已独立记录；仅产品查询投影接分类，原业务实现、Purpose、发布及入图所有权保持 |
| internal/taskrunner/pr_conflict_test.go、control.go、runnerbridge/bridge.go | resolve_pr_conflict 已走原 message/outbox；沿现有回归证明保持，不为分类给执行链增加导入或包装 |
| product/httpapi/main_agent.go、httpcontract/routes.go、query/async_advisories.go、storage/sqlite/advisories.go、Web 建议详情 | 内部端口及类型按职责命名，wire 路由/命令/来源/记录原样保留；分类只补活动及 request links 投影 |
| internal/prompts、testkit/boundary、runtime.main-agent-advisory | 固定资产及组装函数保留；移动消费者依赖并补分类反例，实际输入预期 0 B 变化，保持稳定测试 ID |

Runner 契约依据为当前 runnercontract、runnerclient 的 generate、repository/file、配置及 controls 校验；不需要读取私有运行数据或修改 Runner。所选 EXE pin 与公开能力不变，不把源码文档检查当作部署验证。

## 2. 实施顺序

| 阶段 | 工作与所有者 | 退出条件 |
| --- | --- | --- |
| P1 分类定义 | 新建四类及既有用途的纯值标识，仅供产品查询推导三种内容活动分类；不建立目录、组合校验器、handler 或派发器。以真实来源映射反例验证，而非为常量列表建一套校验框架 | SAC-A01/A02；无 I/O、空服务、执行链分类依赖或重复业务状态 |
| P2 建议模块归属 | 按 Spec 移动 contract/core/bridge，更新 app 生命周期及当前消费者的导入/内部命名。保留 wire、持久身份、版本、原请求及资产路径 | SAC-A02/A03/A05/A06；同输入字节及身份不变，同构建恢复覆盖全部停止责任 |
| P3 配置与产品投影 | 重归专用模型设置，更新示例与生成调试包的配置消费者；保留默认值、共享范围、生效时点及本地限制。仅活动及 request_links 派生分类，由现有活动卡片显示名称，保留 phase/action/execution_ref | SAC-A03/A04/A07；Roadmap profile 和原 Run 设置不被覆盖 |
| P4 接线与验证补齐 | 保留提示词原资产/组装函数，调整移动后的依赖守卫和请求断言；补按操作能力隔离、配置冻结与分类反例。建议 L4 保留原用途，配置 UI 复用独立导航专项 | SAC-A04–A08；不删除旧反例，0 B 预期有实际证据并经独立审核 |
| P5 验证与文档交接 | 执行下表全部自动安排，记录实际受测版本及原始证据；独立审核接线/资产/测试，修复后按影响复验，更新长期设计和验收状态 | SAC-A09/A10；交付记录真实，无新能力或部署声明 |

P2 将原 contract/core/bridge 分别移至 `internal/steeragent/suggestion/contract`、`internal/steeragent/suggestion`、`internal/steeragentbridge/suggestion`，清理旧 Go 包，不留转发壳；不建立 continuation 的通用契约包或其他类型空目录。按 Spec 保留当前协议、路由、命令、活动来源、payload、去重键及提示资产名；不能用“清理旧名”修改它们或数据库版本。P3 仅将配置键改为 continuation_suggestion 并同步当前消费者，不保留 main_agent 配置别名；version 3、缺省/null 默认行为及原字段校验保持，不增加配置树。安装/旧运行现场不动，使用自有隔离 home 验证原记录恢复。

## 3. 必须补齐的验证能力

1. 在产品查询测试真实来源映射：三种内容活动、未知来源/Purpose、maintenance 无活动、控制通道不误归类；不增加任意 type/operation 组合验证测试平台。依赖矩阵覆盖 suggestion contract/core/bridge 和共享定义的方向，移动原最小 L1 选择并核对实际命中，不为分类把完整 owner 回归塞进 L1。
2. 配置 owner、HTTP、Vitest 核对平铺新键的缺省/null 默认、完整对象及原非法值/未知字段拒绝、旧 main_agent 键拒绝、同摘要条件保存、冲突和保存失败、无关字段不丢失；不扩展原模型值和本地限制。补 app 测试 `TestContinuationSuggestionConfigurationFreezesAtPreparation`：设置保存前后、宿主重启前后分别准备/排队/观察原请求，公开请求证明已准备请求不重建、新请求按对应加载快照固定；Roadmap profile 和原 message 设置不受影响。
3. 现有 `runtime.main-agent-advisory` 保留原路由/取消/来源及全部六状态、真实进度版本取消、原未知 stop 查询、同版未知不重发、源恢复去重和共享槽断言。只补活动与 request links 分类及新产品文案/导航，沿公开 Question 触发，不写内部业务库；不把设置保存/重启流程塞入该场景。
4. 产品 query 测试明确 Purpose 为空→planning.parse、documents→material.generate_documents、未知→不带分类，无关来源不带分类；相同活动在列表和 request_links 中分类一致。发布/入图保留 phase/action/execution_ref、计数和记录数量；冲突通过已有 owner 测试证明原 Run/原短指令不变，不补分类调用。不新增完整 E2E suite、Runner workflow 或测试框架。
5. 修改测试选择/主要断言或名字时，同步正式 test-list、L1/L4 可读列表、E2E manifest、执行器源码/证据要求；保持 `runtime.main-agent-advisory` 的稳定 ID 和原验证目的。缺驱动或浏览器证据视为待补齐，不能以组件测试替代。

Runner 准入补测在各原 owner 完成：suggest 需要本地 generate 的原配置/schema，但缺 repository/file 声明仍可执行；Roadmap 两用途缺其实际必需能力时拒绝，零 Submit；原 conflict message 在 can_message/业务资格有效时不依赖 generate 声明，can_message=false 时零消息。单一能力失败不影响原 reply/观察/停止或无关操作。共享分类不增加 capability 查询；公开请求计数与同输入基线比较，而非把所有观察流量误判为新增执行。

## 4. 测试计划

正式测试 ID 的完整有序选择见同名 JSON；它不包含清单未收录的 owner 定向及既有独立配置 UI 专项，后两者仍须按本 Plan 补充执行。通用命令、环境和断言引用[正式清单](../testing/test-list.json)及[测试指南](../testing.md)，不另存第二份定义。下表 Axx 均指 Spec 的 SAC-Axx。源码完成后固定受测版本，执行所选每项并核对实际非零与终态；开发期定向结果不能替代正式集合。

| 验收 | 级别/ID 与原因 | 时点及特殊安排 |
| --- | --- | --- |
| A01/A02/A04/A08 | l1.minimum；原最小协议/依赖方向/提示词/接线守卫及必要来源映射 | 移动包后更新原选择与边界矩阵，检查实际非零；不要求为纯常量包新增镜像测试 |
| A03/A04/A07 | go.http；现行路由/命令、配置目录与条件保存、GET 纯读 | P2/P3 后；只拒绝旧配置键，原协议/路由继续有效；HTTP 压力与 E2E/其他压力串行 |
| A02/A03/A05/A06/A07 | go.storage；原格式/CAS/请求身份、来源分页及 request links | P2/P3 后；无 schema 变化，自有 DB 以原 payload 重启；不打开旧安装库 |
| A03/A04 | go.environment；包含中性 executionconfig owner，避免配置重归影响现有环境/方案边界 | P3 后；原专项定义的全部组/根实际运行 |
| A03/A06 | go.deployment；组装、关闭、启动前停止、namespace 和离线证明 | P2 后；保持原构建/资源身份和未结算责任，非实际部署 |
| A02/A04/A07 | go.roadmap；原解析/文档流程、固定材料、发布及入图的回归保护 | 原图、记录及请求数量不变；分类与本地阶段映射在 query 补充验证，不修改业务 owner 为其添加分类依赖 |
| A03/A07 | web.unit；设置/共享 API 类型、建议查看复制取消、分类与深链 | P3/P4 后；新命名全链与无新操作断言，完整前端集合 |
| A05/A06/A07 | runtime.main-agent-advisory，L4/controlled；公开应用/Git/SQLite/HTTP/Edge 闭环 | P4 驱动补齐后；先构建 Web，保留浏览器、公开请求、原停止与清理证据，串行运行；不以此声称设置流程已验 |
| A02/A07 | runtime.async-activities，L3/controlled；公共活动、request links 与零页面推进回归 | P3/P4 后；按原隔离/并发约束，发布操作不误报模型运行 |
| A02/A04/A09 | smoke.delivery，固定 L2；正式任务和唯一交付不被分类影响 | 最终必跑；真实应用与公开 Runner/PR fixture，无真实外部发布 |
| A03/A07 | 既有配置导航独立 UI 专项；生产构建、受控 HTTP/API/Edge | P3 后；按下述命令补设置页原文案重归、新键保存及宽窄屏，独立保留日志/trace，不混入建议生命周期 |
| A08/A10 | 实际正文审核、文档/清单与范围检查 | P4/P5；见下节及验收，不以测试通过替代独立审核 |

补充定向诊断覆盖正式集合未完整收录的建议/app 与消息 owner：使用 `go run ./cmd/test-feedback -out <独立目录> -- <相关包> -count=1`。新建议 contract/core/bridge 三包全量；app 的五个原根及其全部子项按移动后新名称建立精确清单，保存旧新对应及实际非零证明，不以通配零命中通过。原根为 `TestMainAgentAdvisorySourceIsExplicitAndFreezesOwnerFacts`、`TestMainAgentAdvisoryLegacyActionsRejectedBeforeBusinessAccess`、`TestMainAgentAdvisoryPersistedQuestionAutomaticallyGeneratesWithoutWeb`、`TestMainAgentAdvisoryColdAbsenceAndOriginalStops`、`TestMainAgentAdvisoryStartupReconcilesOriginalStop`。冲突/消息沿下列已有根精确选择：

```text
./internal/taskrunner: TestPRConflictInputUsesOriginalBindingAndShortPrompt
./internal/runnerbridge: TestRunMessageUsesOriginalOutbox
                       TestUnknownRunMessageNeverResends
                       TestRunMessageBarrierSaveFailureSendsNothing
```

另执行第 3 节的 app 配置冻结根；`./internal/product/query` 全量定向覆盖新增投影及其原状态反例，该包不在 go.http 的七包集合内，不能遗漏。合同/core/bridge/app/query 的补充命令均使用上述 test-feedback 入口及独立 out；新增根在实施时建立并确认非零，旧新根映射随验收记录保存。

上述正式集合在最终固定版本完整执行；开发阶段按影响定向迭代。没有新修改、失败或未解决疑点时，不因阶段表再次出现相同 ID 而重复执行同一正式全集。

配置浏览器复用 [L4 已列明](../testing/l4.md)的独立 `configuration-navigation.spec.ts`，新增当前模型目录 fixture、新键保存及失败/未保存保护、900px/375px 设置区域可读性用例，不增加业务服务。先 `npm --prefix web run build`，再设 `STEERLINE_WEB_EVIDENCE` 为独占绝对目录，完整运行 `npm --prefix web run test:e2e -- configuration-navigation.spec.ts`，覆盖新增建议设置及三个原导航/环境用例。保存 build/browser 日志、实际命中结果、截图、trace 和服务器/浏览器清理事实；不以 web.unit 或编译替代。该入口已是独立 UI 专项，不伪造正式 JSON ID；实施时同步其 L4 说明。它仅证明本期客户端行为，后端保存与加载快照由 go.http 和新增 app 根验证。

首次实施曾将独立 UI 验收限定为新增设置用例，并保留三个旧用例失败。用户后续明确要求修复三项失败、判断夹具与产品责任，并重跑完整文件；现按当前保存信封、就绪 DTO、环境概览及操作结果呈现维护旧夹具/断言，保留状态、任务往返、故障恢复、项目隔离、键盘与宽窄屏覆盖。完整文件复验与根因登记在专属验收第 5 节，旧失败日志不覆盖；十个正式测试 ID 不变。仅 UI 测试/文档修复时补跑 L1、L2、web.unit，不因没有生产改动重复其余已通过专项。

无需真实模型或真实 PR 联验：后端与 Runner 执行契约、实际模型输入未改，本期不声称重新验证模型质量；controlled 也不冒充真实执行证明。若实施发现必须改变该边界，先修订范围及测试选择。

## 5. 提示词审核、证据与交付

当前建议资产基线：模板 145 B、规则 1025 B、golden 1180 B，路径及三者字节都保持。P4 用相同固定业务事实和身份夹具对照建议、Roadmap、冲突的实际模型正文、材料/schema/digest，预期每项 0 B 变化，不把新分类放进正文。实际随机请求号不冒充提示词差异；任何超出预期的正文变化先修复或修订本期范围，不能只更新样本。

按[提示词治理](../../docs/prompt-governance.md)提交完整正文 diff、前后 UTF-8 字节数、每项变化的业务原因及独立审核结论，覆盖资产、接线、守卫与测试安排；本次文档自审不替代实施阶段独立审核。无批准或有未解决阻断项，不能报告实现交付完成。

证据使用[路径清单](../paths.md)的独立 scope `wt-6521-steer-agent-classification`；每次执行新叶目录，保留源码/构建身份、summary、原始日志和清理结果。失败与补测分别保留。正式集合及补充诊断/独立 UI 专项完成后，按报告约定仅为干净且未变的实际受测 SHA 导出测试历史；未提交工作树只保留本地原始报告和文件摘要，不误归到 HEAD，也不为导出记录擅自提交。另登记不能导出的原始专项证据；更新长期设计、配置导航和现行文档对保留 wire/资产名的说明，不改写历史固定 Spec。文本通道例外仍仅适用于建议用途，不扩大到其他类型。不改 execution-map，不承诺部署或 PR 已创建。
