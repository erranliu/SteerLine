# SteerAgent 分类收敛：长期文档审计与验收

2026-11-10。状态：已按用户后续指令实施分类收敛，本期十项正式回归、补充 owner 与完整配置导航 UI 验收完成（后续修复见第 5 节）；前三轮文档审计保留为历史，当前结果见第 4–6 节。对应 [长期设计](../../../docs/product-design/steer-agent.md)、[Spec](../../spec/steer-agent-classification.spec.md)、[Plan](../../plans/steer-agent-classification.plan.md)及[测试选择](../../plans/steer-agent-classification.tests.json)。普通开发，不进入 roadmap。

## 1. 长期文档审计

审计基于 `a1451a16b3e7f4f025eff5c923cf7cbe13f63079` 与当前未提交文档，检查分类是否扩权、是否复制业务状态、是否把未来能力写成已实现，以及 Runner 映射是否符合调用方代码。此为当前聊天自审，不冒称独立审核。

| 编号 | 发现及依据 | 修订与本期约束 |
| --- | --- | --- |
| D01 | 长期文档将原问题回答与模型操作并列，易误解为已具备回答辅助/自动采用；现有 decisions 与 taskrunner/input.go 只是原 Question 的回答通道，mainagent/advisory.go 只有 prompt 产物 | 明确回答属于既有业务控制，协作表不授予智能操作；本期仅四个既有操作，不增加回答辅助 |
| D02 | “模型、后端、工具及策略分别选择”未写固定时点，易推导续作可切模型；pr-conflict-resolution Spec 与 runnerbridge 的原 message 沿用原绑定 | 明确创建请求时固定，续作沿原 Run；专用建议设置不覆盖 Roadmap profile 或原开发 Run |
| D03 | 文档生成按能力归类，而记录还承载发布/入图，可能把后续阶段误报为模型运行；query/async_roadmaps.go 明确本地 operation 清空 Run/Attempt 并单独记录 action | 明确分类是用途，阶段和 execution_ref 独立；本期投影只派生分类，不新建活动或覆盖原执行事实 |

其他核对结论：generate 的单次/本地/只读及仓库文件材料边界与 runnerclient 一致；冲突处理继续 message，不借 conversation 替代；四类声明不意味着四类可执行；现有 owner、未知请求与停止责任继续保留。Runner conversation 的具体产品接入和所需快照保证仍为后续缺口，本次未测试部署 EXE，也未将源码文档当作能力实测。

审计后由本 Spec 固定命名、四个操作、配置重归及验收，避免长期文档承担字段和实施细节。实施接线、提示词及测试仍须独立审核。

### Spec / Plan 第二轮审计

2026-11-10 按用户要求再次自审，重点检查过度设计、业务解耦和验收可执行性。发现以下问题并直接修订；下列修订替代首轮草案中的对应要求，未实施产品代码。

| 编号 | 确认的问题与依据 | 直接修正 |
| --- | --- | --- |
| R01 | 分类被扩成协议/HTTP/命令/活动来源及数据库版本整体改名；这些字符串参与 mainagentbridge 的去重、序列化和原停止恢复，改动不能改善能力解耦，反而改变模型上下文并扩大数据范围 | 保留当前 wire、持久身份、版本和提示资产；只移模块/内部命名与重归专用配置，不新增兼容壳或迁移 |
| R02 | “注册操作、未知不派发”容易引入并无真实消费者的统一执行/准入层，且 continuation/contract 可能被误作整类通用协议 | 分类限纯值用途映射，没有 handler/API/派发/授权；建议契约仅属 suggest，冲突始终归原 owner |
| R03 | 缺 generate/schema/file 的反例笼统覆盖全部操作，可能让建议依赖文件材料、message 依赖 generate | 明确按操作的必要能力与缺无关能力仍可用的正反例；禁止扩大阻断到原 reply/观察/停止和其他操作 |
| R04 | 新嵌套配置未明确缺省、null、部分对象与未知层级；保存、加载、准备及排队冻结虽有要求但没有明确 app 验证根 | 固定合法/拒绝矩阵、无关配置及条件保存；新增待实施的配置冻结根，覆盖已加载快照和原请求保持 |
| R05 | 建议生命周期 L4 被要求承担设置保存及重启，原驱动并未提供相应流程，会扩展不相干的夹具职责 | 建议 L4 保留原用途；设置页面复用已列明的配置导航独立 UI 专项，给出 build/Edge 命令、证据和清理要求，后端由 HTTP/app 验证 |
| R06 | “公开投影”未限定消费面，Purpose 默认值也不明确；query 不在 go.http 七包范围，投影验收可能缺少执行入口 | 字段只加活动及 request_links，供原活动卡片显示名称；明确空 Purpose/文档/未知/无关来源、列表关联一致性与本地阶段；补 query 全量定向，不新增各详情协议或重复活动 |

修订后范围为四类/四个既有用途、建议模块归属、专用配置及现有活动只读分类。Runner/模型输入、现行调用协议与存储结构保持；自动验收仍全部未执行，十个正式 ID 及补充 owner/独立 UI 安排共同覆盖，不以清单出现 ID 代替产品验证。

### Spec 第三轮：过度设计与结构债审计

2026-11-10 按用户要求继续自审。确认并修正三处结构问题；本节替代前轮关于配置树、null 拒绝及强制四个执行消费者的要求，不新增实现工作。

| 编号 | 问题与依据 | 修正 |
| --- | --- | --- |
| R07 | 唯一可配置能力使用 steer_agent.continuation.suggest 三层结构，额外引入空/部分层级、继承解释及校验分支；原 executionconfig 的 MainAgentSettings 已有简单值与 nil 默认规则 | 仅重命名为平铺 continuation_suggestion，沿原两个字段、缺省/null 默认与校验语义；没有类型配置树、空配置或新默认规则 |
| R08 | 只产出 prompt 的核心/宿主以 continuation 整类命名，虽用文字限制仍容易让后续冲突/消息契约进入同一模块 | 核心/contract 与宿主按实际职责命名为 suggestion；产品分类不决定模块树，未来类型不建空包 |
| R09 | 强制四处真实代码消费分类，但冲突命令没有新的分类展示或控制消费者，易形成 taskrunner/runnerbridge 对展示元数据的无效依赖；组合校验也没有外部选择入口 | 只有产品查询层消费建议/解析/文档的活动分类；冲突保留业务归属与原回归，不新增执行链导入、包装、目录或组合校验器 |

本期必要增量仍为模块按职责归属、一个现有配置键重命名、已有活动的只读分类和相应产品文案。测试验证真实来源与行为，不为常量表建立镜像测试；正式回归在最终固定版本执行，不因阶段重复引用而无条件重跑。已确认问题均已同步 Spec/Plan，本轮无新增代码或产品验收结果。

## 2. 自动验收安排

下表按本期范围登记当前结果。执行入口、级别、环境和时点引用 Plan 第 3–5 节及同名 tests.json，原始结果见第 4 节；后续按用户要求修复旧 UI 用例并完整重跑，关项证据见第 5 节，初始失败仍保留。

| ID | 具体行为与主要反例 | 最小通过条件 | 状态 |
| --- | --- | --- | --- |
| SAC-A01 | 对照四类/四用途归属与三种内容活动的真实来源；未知来源/Purpose、maintenance、answer/stop/observe；扫描新增目录/接口及导入 | 仅实际产品查询消费分类；未知来源不附标签，无目录/组合校验器/handler/通用执行或新准入；执行 owner 不为展示分类增加依赖，未来类型无空包 | 通过 |
| SAC-A02 | 依赖矩阵核对新路径；Roadmap 两类内容记录与发布/入图；原冲突命令绑定 | contract 标准库、核心无执行依赖；原记录/Task/DAG 不重复，原 Run/会话不替换；消息 owner 三个反例及冲突短指令原断言保持 | 通过 |
| SAC-A03 | 平铺 continuation_suggestion 的缺省/null 默认、完整对象、原非法值/未知字段及旧键、保存失败/摘要冲突与无关字段；保存/宿主重启/准备/排队冻结；现行 wire 及原 payload 恢复 | 仅键/类型重命名，值及校验语义保持；缺省/null 使用原默认，非法/旧键零写入拒绝；已加载/准备请求不重建，新请求按快照固定；无关配置及持久身份保持，无配置树或数据库升级 | 通过 |
| SAC-A04 | 同输入捕获四用途的公开请求；suggest 缺必要 generate 配置/schema 与缺无关 repository/file 分别测试；Roadmap 缺必要文件材料；message 缺 generate、can_message=false、错误绑定/版本 | 必要能力失败零提交、缺无关能力仍按原资格可用；原 reply/观察/停止不被新分类阻断；公开计数相对基线无新增现场查询/会话/旁路/权限 | 通过 |
| SAC-A05 | 原 Question 从公开 fixture 进入生产观察，未打开页面生成；重复轮询/源恢复/重启、失败及新 occurrence；查看和复制 | 原问题只生成一次、快照不重建、失败不重开；只有新 occurrence 可新增；答案/材料/PR/任务零新增副作用 | 通过 |
| SAC-A06 | 提交未知、原 stop 回执未知、同版 missing、实际进度后历史正版本取消、源解除、关闭及冷启动恢复；资源迟到结算 | 原请求不重投，原身份核实及 stop 替换条件不放宽；取消不被迟到结果覆盖；未结算持续占共享槽，全部停止证明及清理完整 | 通过 |
| SAC-A07 | query/HTTP/Vitest 与 Edge 检查活动/request links、卡片能力名称、Purpose 空/文档/未知及无关来源、发布/入图；配置 UI 用独立导航专项验证读写、失败/未保存保护、900px/375px 可读；建议 L4 验证复制取消、深链、GET 和隔离 | 分类只在约定活动面出现且列表/关联一致，无字段不显示能力标签；原 phase/action/execution_ref、计数/记录数保持；新配置读写文案正确，页面无新操作，查询零推进；UI 证据与后端保存验证分别登记 | 通过 |
| SAC-A08 | 原路径资产及同固定输入对照；缺资产、未知协议/非法输出按原规则拒绝；独立审核实际请求和接线/测试 | 模板/规则/golden 保持 145/1025/1180 B 且逐字相同，四用途模型正文/材料/schema/digest 预期 0 B 差异；不以样本更新消除变化，审核批准且无阻断项 | 通过 |
| SAC-A09 | 执行 tests.json 中全部十个正式 ID、明确的新旧 app/core/bridge/query/消息 owner 定向及独立配置浏览器；核对实际根/清单、日志和资源 | 所有选中项和补充项非零、完整、无 skip，通过且清理完整；独立 UI 有实际结果/截图/trace，失败补测另存，编译/旧版通过不替代 | 通过 |
| SAC-A10 | 对照实现核对长期设计、当前建议说明、配置/接口导航与源码命名；检查新文件和入口 | 当前/未来清楚，四类无空壳/误称已实现；旧冻结证据不重写，不修改 Runner 协议、roadmap 执行清单或安装现场 | 通过 |

全部自动项及提示词/接线独立审核是本期实现交付的必要条件；不得将真实自动失败转成人工待验。无新增必须由用户参与的人工签收。可后置收集名称理解与使用体验，未观察时不报告人工通过；不替代 Edge 自动行为和实际证据。

## 3. 规划阶段静态检查（历史）

规划阶段仅执行下列文档静态检查，不据此把 SAC-A01–A10 标为通过。尚无产品受测源码、EXE、浏览器证据或实施阶段独立审核结论，沿用旧功能验收链接不等于新分类版本已验。

| 检查 | 2026-11-10 结果 |
| --- | --- |
| `python scripts/check_doc_links.py`（首轮） | 737 份 Markdown、4567 个本地链接，0 错误 |
| 本地 Python 核对 Spec/Plan/acceptance 与同名 JSON（首轮） | 十个正式测试 ID 已登记且有序唯一、全部出现在 Plan；SAC-A01–A10 要求与验收逐项对应；本批新文档为 UTF-8/LF |
| `git diff --check` 与改动范围核对 | 通过；当前仅长期文档、规划、测试选择及导航，无产品实现、提示词资产或 Runner 改动 |
| 第二轮文档与范围复核 | 737 份 Markdown、4568 个本地链接，0 错误；十个正式 ID 与 SAC-A01–A10 映射、全项未执行状态、补充 query/app/独立 UI 入口及改动范围检查通过；三份建议资产与 HEAD 原始字节相同。新增文档检查 UTF-8/LF，存量文件保留原 checkout 换行，不批量格式化 |
| 第三轮结构审计后检查 | 链接检查仍为 737 份 Markdown、4568 个本地链接、0 错误；平铺配置与 suggestion 模块命名在 Spec/Plan 一致，十个测试 ID 和 SAC-A01–A10 映射通过，全部产品项仍未执行；改动限文档，提示资产字节不变 |

实施时按 Plan 的独立 scope 登记每次实际受测 commit/工作树摘要、Runner pin、测试 ID/实际命令、summary 与原始日志、公开请求及 Edge 证据、清理状态；提示词完整 diff 与字节记录单独链接。先前失败保留，复验新增记录；产品发布与安装部署不在本期范围。

## 4. 实施与实际验证

本期按普通开发完成 suggestion 契约/核心/宿主移动、app 停止与恢复消费者、平铺 continuation_suggestion 配置及 Web 文案、产品活动和 request_links 的只读分类。共享分类仅常量，公共 DTO 归 product/api；无通用派发层、类型配置树或未来空服务。Runner、消息/冲突、Roadmap 业务 owner 和存储 schema 没有分类执行依赖；原 wire、持久身份及提示资产保留。

受测基线 HEAD 为 `a1451a16b3e7f4f025eff5c923cf7cbe13f63079` 加本期未提交工作树，不能将结果归为干净 HEAD 通过。原始报告保留源码状态与 E2E 构建摘要；变更路径/删除项及源码 SHA-256 在 `tested-source-manifest.json`，原五个 app 根及全部子项的改名/实际通过映射在 `app-test-root-mapping.json`。独立证据根为 `C:/SteerlineDebug/wt-6521-steer-agent-classification`；下述路径均相对此根。当前未提交，因此按[报告约定](../reports.md)不导出冒指 HEAD 的 Git 历史记录，不为登记记录擅自提交。正式 ID、owner 和浏览器各自证明自己的范围。

### 正式及补充结果

十个正式 ID 均按原选择完整执行，managed 结果为 passed，零测试跳过；E2E 三项（L2、建议、异步活动）的 summary/scenario 均确认 cleanup_complete=true。各目录均有 summary、原始日志及 managed 报告，完整命令以其中记录为准；执行耗时不含准备，E2E EXE SHA-256 见各 summary 的 build_sha256 和 build/frozen-binaries.json。汇总索引为 `verification-index.json`。

| 正式 ID | 结果 / 执行耗时 | 证据目录（相对独立根） |
| --- | --- | --- |
| l1.minimum | 通过 / 18.9 s | `test-results/L1/02` |
| smoke.delivery | 通过 / 82.2 s | `test-results/L2/01` |
| go.http | 通过 / 45.6 s | `test-results/go-http/01` |
| go.storage | 通过 / 81.3 s | `test-results/go-storage/02` |
| go.environment | 通过 / 372.2 s | `test-results/go-environment/01` |
| go.deployment | 通过 / 456.8 s | `test-results/go-deployment/01` |
| go.roadmap | 通过 / 299.3 s | `test-results/go-roadmap/01` |
| web.unit | 通过 / 19.3 s | `test-results/web-unit/02` |
| runtime.main-agent-advisory | 通过 / 148.5 s | `test-results/runtime-advisory/01` |
| runtime.async-activities | 通过 / 257.2 s | `test-results/runtime-activities/01` |

补充定向：`diagnostics/owners-03` 66 项通过（含五个原 app 根全部子项及新配置冻结根）；`capabilities-04` 8 项通过（含必要/无关能力及原消息、冲突反例）；`final-owners-06` 建议核心/宿主、query 和 prompts 全量 159 项通过；`catalogue-07` 31 项通过。均为实际非零、零测试跳过。suggestion/contract 本来没有独立测试文件，只记编译项，契约验证经核心/宿主测试覆盖，不将包级 no-test 事件冒称测试通过。

`web.unit` 全集 47 个 suite、402 项通过，0 skip/todo；生产构建见 `diagnostics/browser-01/build.log`。独立新增设置 UI 见 `diagnostics/browser-03/browser.log`、browser 下截图/trace，精确 1 项通过；已目视检查 900px、375px 截图。建议 L4 的六状态、实际进度版本取消、源恢复/重启去重、共享槽、未知停止及无业务副作用证明均在 `test-results/runtime-advisory/01/scenarios/runtime.main-agent-advisory/advisory-proof.json`，各项 true；公开完整请求在同目录 advisory-public-requests.json，Edge 记录在 browser-ready/browser-cancel。异步活动和 L2 的实际请求、行为与清理事实见各 scenario.json 及同目录原始证据。

### 提示词、接线与独立审核

业务原因仅为模块按实际产物归属、专用设置命名及活动展示分类，不改变行为规则或模型输入。`prompt-audit/baseline` 为 git archive HEAD 的独立源码；外部 `_capture_test.go` 通过 Go overlay 复用两版原 owner 夹具，实际经过建议 prepared/step、Parse、GenerateDocuments 与 conflictInput。仓库 fixture 路径在调用前固定为同值，随机的外层请求号不当作模型输入；原始 input 不作归一化。每版实际执行 3 个包/4 个用途，原始 Go 事件在 before/after 的 test-wire.jsonl。

| 用途 | 原始 wire input 前→后 | 实际正文前→后 | 完整差异 |
| --- | --- | --- | --- |
| 对话提示词建议 | 1764→1764 B | 1345→1345 B | 0 B |
| Roadmap 解析 | 2786→2786 B | 416→416 B | 0 B |
| Roadmap 文档 | 6180→6180 B | 582→582 B | 0 B |
| 原 Run 冲突消息 | 324→324 B | 288→288 B | 0 B |

完整输入、正文、材料、repository 与 schema 保存于 `prompt-audit/before`、`after`；逐字 diff 是各用途的 `.wire.diff` / `.diff`，字节数和 SHA-256 在 `prompt-audit/comparison.json`。模板/规则/golden 逐字保持 145/1025/1180 B；没有更新 golden。正式停止/未知/来源恢复等行为由原回归验证，overlay 抓取不替代正式测试。

独立审核者 `/root/classification_review` 已只读审查生产接线、原断言迁移、完整请求/字节证据及浏览器范围校准，并独立重算 before/after 原始日志和字节切片；结论为批准、无阻断项。补充的列表非空断言及 ongoing 筛选也已复核通过。原审核记录保存于 `prompt-audit/independent-review.md`。审核不替代下列正式结果。

### 失败与范围限制

- 初始 owner 定向暴露移动后的相对 fixture 路径；修正为 `../../runnerclient/testdata/content-capabilities.json`。新增 app 测试漏建共享队列已修复，原业务逻辑没有新增兜底。原始失败保留在 `diagnostics/core-01`、`owners-02`，修复后 `owners-03` 通过。
- `test-results/L1/01` 检出 API 使用领域分类 DTO 导致 HTTP 消费面间接依赖；改为 API 自有字符串 DTO 后 `L1/02` 通过，原依赖守卫未放宽。
- `diagnostics/final-owners-05` 的新增列表非零断言暴露测试错误使用 recent 筛选进行中活动；修正为显式 ongoing，`final-owners-06` 全量通过。生产筛选规则未改。
- 首次独立 `configuration-navigation.spec.ts` 整个文件未通过（后续已在第 5 节修复关项）：`diagnostics/browser-01` 为 1 passed / 3 failed。依据未变旧用例与当前代码，定位旧保存信封/就绪夹具和已移除环境下拉框的假设为本期范围外问题；没有在干净 HEAD 上重跑证明基线失败。临时修补夹具的 `browser-02` 为 3 passed / 1 failed，修补已撤回，原断言保留。经 Plan 明确本期独立 UI 的精确选择后，`browser-03` 新设置用例 1 passed（2.6 s），900px/375px 截图、trace 和自动服务清理均保留；只能证明本期新增用例，不能声称配置导航全集通过。正式十个测试 ID 不缩减。

本期不执行真实模型/真实 PR、安装部署或 roadmap；没有将受控 fixture 结果称为真实链联验。没有已观察的人工体验签收。

收尾检查：737 份 Markdown、4574 个本地导航链接，0 错误；`git diff --check` 通过；生产 Go 无旧 mainagent 包导入，Runner/message/conflict/Roadmap 执行 owner 无改动。日志在 `doc-links.stdout.txt`、`diff-check.stdout.txt` / stderr；最终未提交工作树文件摘要在 `final-worktree-manifest.json`。提示资产和受测生产文件的 SHA-256 已核对保持。

## 5. 配置导航旧失败修复与完整复验

2026-11-10，用户明确要求处理三项旧失败并完整重跑。结论：这三项失败来自测试夹具及选择器/预期过期，本次没有发现需要修改生产逻辑的产品问题；相对于第 4 节受测生产树，本次只修改 `web/tests/configuration-navigation.spec.ts` 和测试说明/验收文档。没有通过放宽生产解析、添加兼容回退或跳过用例使测试通过。

| 原用例 | 确认根因与依据 | 修复及保留覆盖 |
| --- | --- | --- |
| 开发者配置 | 当前 `execution-configs.ts` 与后端 `EditRequest` 均使用 saved_digest/config/profile_edits 信封；旧 PUT 夹具把整个信封写为 config，导致断言读不到 developer | 保存 edit.config，并核对摘要、If-Match、空 profile_edits；保留键盘开关、未保存保护、无关方案不丢失、重启生效和发布隐藏 |
| 项目配置导航/并发保存 | 夹具缺少 maintenance/readiness、maintenance.readiness 和 control.blockers；成功 HTTP 响应却返回不符现行 DTO 的空对象，触发页面读取 status/blockers 失败；保存夹具也有同样信封错误 | 用前端 ProjectReadiness 类型约束完整就绪事实，补齐对应端点/字段并正确保存 edit.config；保留返回/历史、草稿保护、共享并发、无关配置、零项目与宽窄屏 |
| 运行环境 | 同样缺就绪 DTO；旧环境下拉框已被 `e79d0b1c` 的全环境概览替代，卡片待核实文字也已更新；操作结果自 `42c7e057` 增加复制执行信息按钮，旧整区纯文本断言已不成立 | 通过概览行的查看/收起详情按钮选择环境，并核对行数、远端名称及展开状态；精确检查成功状态文字和复制按钮可用性；保留六状态、禁用资格、卡片定位/焦点、方案记忆、任务往返、查询失败缓存恢复、键盘、身份不外露、项目隔离和零写入断言 |

这不是对未知生产缺陷的兜底：后端就绪契约明确含 blockers 数组及嵌套 readiness；现行组件单元测试已明确断言无环境下拉框、使用概览，并检查操作结果复制按钮。旧夹具/断言与这些已存在的契约不一致。

复验继续使用本任务独立证据根，失败不覆盖：

- `diagnostics/browser-04/build.log`：当前生产客户端构建通过。首次修复后的完整文件为 3 passed / 1 failed，原始日志/截图/trace 保留；唯一失败是整区文字断言把新增复制按钮也算入状态，已直接修正为状态和按钮分别断言。
- `diagnostics/browser-05/browser.log`：执行 `npm --prefix web run test:e2e -- configuration-navigation.spec.ts`，没有 -g、重试或 skip，**4 passed（11.7 s）**。每项的截图与 trace 均在 browser 目录，最后进程退出码为 0；原 HTTP 服务器 afterAll 关闭逻辑保持。此结果替代前轮“整文件仍未通过”的当前限制，browser-01/02/03/04 继续作为历史证据。

本次没有修改提示词资产、模型接线或对应提示词测试，前轮四用途 0 B 差异和独立审核记录不变；没有声称新增真实后端/模型、PR 或安装部署验证。

后续最小回归已完整补跑：

| ID | 结果 / 执行耗时 | 证据目录（相对独立根） |
| --- | --- | --- |
| l1.minimum | 通过 / 19.0 s | `test-results/L1/03` |
| smoke.delivery | 通过 / 89.4 s | `test-results/L2/02` |
| web.unit | 通过 / 23.5 s | `test-results/web-unit/03` |

L2 清理完整；前端仍为 402 项全部通过、0 skip/todo。完整导航的四份 trace、13 张截图及测试源码 SHA-256 汇总在 `diagnostics/browser-05/verification.json`；已目视检查环境 375px 和配置 900px 截图。临时 HTTP 端口 63217 在测试退出后无监听（Get-NetTCPConnection 全部 Listen 查询成功后筛选），afterAll 及测试进程正常结束。前轮受测生产文件逐项 SHA-256 保持，本次只修测试/文档。

## 6. 实现复审：过度设计、解耦与结构债

2026-11-10，按用户要求审计当前实现，相对基线仍为 HEAD a1451a16b3e7f4f025eff5c923cf7cbe13f63079 加当前未提交工作树；不将前轮已验结果冒称本轮重跑。此次是当前聊天直接审计，未另开独立审核；没有修改提示词资产、组装、Runner 接线或对应提示词断言，前轮相同代码的提示词审核不被本轮 UI 修复改写。

| 编号 | 确认问题 | 直接修正与证据 |
| --- | --- | --- |
| I01 | 新增 steerAgentLabel 使用普通对象索引并强制断言 Record<string,string>，未知 type 为 constructor/__proto__/toString 时会命中继承属性；无法保证未知分类无标签，且对象值会使 React 渲染失败 | 改为四种已知类型的显式 switch，其余返回 undefined；没有引入注册表、组合校验器或配置。原 helper 测试补四个未知值，活动组件补四个真实渲染反例，要求标题/原 phase 仍可见且无能力标签。修复前 `diagnostics/audit-01-red/web-tests.log` 实际 4 failed / 14 passed，证明不是推测 |
| I02 | 分类 query 测试的 admission 案例实际上仍使用 roadmap_publish，未验证入图动作/phase；本地命令也没有列入关联请求夹具 | 使用真正的 roadmap_admit 与“Roadmap 正在入图”，发布仍使用原动作与阶段；原生成请求及本地命令都加入关联事实，列表/request_links 同时核对分类、phase 和 execution_ref。生产 query 逻辑原本正确，没有为测试改写行为 |

其余审计结论：

- 分类根包只有四类与既有用途常量；只有 product/query 的建议与 Roadmap 两个生产投影导入根包。无运行时目录、统一 action/result、派发器、未来空模块或类型配置树。resolve_conflict 标识仅说明原业务用途，没有向原消息链补无效消费者。
- API 自有可选字符串 DTO，HTTP、Runner、持久事实层没有因展示元数据引入领域核心。原来源/Purpose 决定分类，未知不猜测；分类没有成为准入或状态权威。保留 API 与内部常量的边界，不为复用两个字段增加跨层共享协议。
- 建议 contract 保持值契约，core 只校验/组装提示词，宿主保留来源、冻结请求、共享槽、未知观察和停止责任；归属移动经对照只改变包/内部名，没有旧包转发、迁移、双写或回退。没有为四类能力各建服务。
- 配置沿平铺 continuation_suggestion 及原 model/effort、默认/null、严格未知字段、条件保存与准备冻结；未引入继承、整类开关或跨能力共享设置。原 Run 的模型/权限及 Roadmap profile 不被分类影响。
- contract、generate、service、stop 的全部非 import 字符串与 HEAD 原 owner 逐项相同，协议/payload/source/停止身份保持；Runner、原消息/冲突及 Roadmap 执行 owner 的 Git 差异仍为空。模板/规则/golden 实际仍为 145/1025/1180 B、逐字不变；没有扩大主 Agent 完整文本通道例外。

本轮只修复未知类型呈现和验收缺口，不借审计重构旧 owner 或抽象多个设置表单。相关 Go owner、L1/L2、完整 Web 单元集以及重新构建后的配置导航整文件按影响补跑；正式十项选择保持，其余无生产影响的旧专项不重复登记本轮通过。实际结果追加如下。

| 本轮验证 | 实际结果 | 证据目录（相对独立根） |
| --- | --- | --- |
| 相关 Go owner 全量 | 154 项通过，0 测试跳过；contract 无独立测试，只记编译项 | `diagnostics/audit-02-owners` |
| l1.minimum | 通过 / 18.3 s 执行耗时 | `test-results/L1/04` |
| smoke.delivery | 通过 / 84.6 s 执行耗时 | `test-results/L2/03` |
| web.unit | 通过 / 25.6 s 执行耗时 | `test-results/web-unit/04` |
| 当前 Web 构建 / Edge 配置导航整文件 | 构建通过；4 passed / 11.4 s，无 -g、重试或跳过 | `diagnostics/audit-03-browser` |

完整 Web 集合现为 406 项通过（新增四个未知类型组件反例）、0 fail/skip/todo；L2 清理完整。未重复前轮未受影响的 HTTP 压力、存储、环境、部署、Roadmap 及建议生命周期专项；本轮表格与第 4/5 节的历史结果分别归属。测试及源码核对汇总见 `diagnostics/audit-04-verification.json`，原失败与补测均保留。本轮未修改 Spec 的能力范围或引入新的测试框架。

## 7. 提交与 PR 交付

2026-11-10，用户明确授权提交、推送当前改动并创建 PR。第 4–6 节保留各自实际受测的工作树/基线与失败、修复记录，不改写为干净提交通过。创建 PR 前同步远端 main 的 UI/Roadmap 改动，按实际冲突范围验证集成；只将随后针对干净提交执行的正式报告按原规则导出 Git 历史，文件名指向真实受测 SHA。PR 描述分别说明原分类实施回归和集成后验证，不声称真实模型/PR 业务联验、安装部署或已合并。


### 最新 main 的集成核对

PR 目标 main 已前进到 `3ce94c753f4b610aa3f6c7d70b98d194ad4ecf2e`。初始实现保存后 rebase，集成提交为 `3954901935eaae56d757e5f7e795441465e68087`：两个导航索引同时保留两边条目；活动卡片保留 main 简化布局，只增加分类标签；候选页保留 main 新分区，只替换建议组件导入与名称。相对该 main，Runner/冲突/Roadmap 执行 owner 和提示词资产无 PR 差异。

最新 main 同夹具实际抓取再验证：建议/解析/文档/冲突原始 input 分别 1764/2786/6387/324 B，正文 1345/416/599/288 B，前后逐字相同、diff 全为 0 B。文档用途相对旧 a145 基线的长度增长来自 main 已合入的 provides_tests 内容，不能沿用旧 6180/582 B 声称当前基线。完整请求与 before/after 测试日志在 `pr-prompt-audit`；独立审核者 `/root/classification_review` 已核对并批准集成后的提示词/接线，未冒称 GitHub 人工 review。

干净集成提交已跑 L1/L2/Web 全集（422 项）并导出对应 Git 历史。首轮 `go.roadmap` 的 draft-dependencies 子例出现进度观察断言失败：业务仍为 awaiting-merge，但旧模拟错误的观察诊断尚未清除；生产代码与原断言均未改，此次失败与原始日志保留在 `test-results/pr-roadmap/01` 和同 SHA 历史中，后续串行复验另存。不能用其他分支通过或分类查询测试替代该失败。

最新 main 的 UI 隐藏检测 ID、统一从“复制执行信息”读取，首轮两文件浏览器验证为 7 passed / 1 failed（`diagnostics/pr-browser-01`）。将旧的 ID 直接可见断言改为核实完整 ID 被复制、正文不外露后，配置导航及 UI 简化两文件完整 8 passed / 31.8 s（`diagnostics/pr-browser-02`）；未放宽产品行为或删除身份检查。长期 SteerAgent 文档同步为分类已落地，明确用途标签、独立建议设置与仍未实现的能力缺口；运行日志继续只记在本验收中。


### 最终干净提交验证与测试历史

最终源码/测试/长期文档受测提交为 `76cadaeb539c0835b91bef2eeba83e13c69b7424`。下列七项正式执行的 managed 报告均确认 before/after SHA 相同、before_clean/after_clean=true，完整且零测试跳过；L2、建议生命周期、异步活动的 summary/scenario 均确认清理完成。正式历史已导出到同名 records 文件，后续只提交记录与本验收补记，记录仍指向真实受测 SHA。

| 正式 ID | 结果 / 执行耗时 | 原始目录（相对独立根） |
| --- | --- | --- |
| l1.minimum | 通过 / 17.6 s | `test-results/pr-L1/02` |
| smoke.delivery | 通过 / 86.4 s | `test-results/pr-L2/02` |
| web.unit | 通过 / 22.7 s | `test-results/pr-web/02` |
| go.http | 通过 / 48.9 s | `test-results/pr-http/02` |
| go.roadmap | 通过 / 361.0 s | `test-results/pr-roadmap/02` |
| runtime.main-agent-advisory | 通过 / 151.0 s | `test-results/pr-advisory/02` |
| runtime.async-activities | 通过 / 264.4 s | `test-results/pr-activities/02` |

相关建议/core/bridge/query/配置 owner 全量 154 项通过（`diagnostics/pr-integration-01/owners`）；App 建议来源、配置冻结、重启、停止的 17 项通过（`diagnostics/pr-app-02`）。当前 Web 全集 422 项通过，0 fail/skip/todo。配置导航和 UI 简化两文件在固定提交上完整 8 passed / 30.9 s（`diagnostics/pr-browser-03`），不以定向子集冒称整文件。Roadmap 首轮失败对应 `3954901`，后续完整串行复验通过对应 `76cadaeb`；两者之间 Go 生产文件没有变化，也没有放宽原观察断言，不把可能的时间/负载因素写成已确认根因。

本轮未重复原基线已有的 go.storage/go.environment/go.deployment 专项，不把第 4 节十项通过套用为最新提交全部重跑。此前脏工作树报告仍只保留本地；只有两份真实干净提交记录进入 Git 历史。最新 main 实际输入 0 B 差异与独立审核批准已在上节明确。原始证据索引为 `pr-integration-summary.json`。代码 PR 的创建是本次开发交付，不等于 SteerLine 产品真实 PR/模型业务链的验收，也不代表安装部署或合并完成。
