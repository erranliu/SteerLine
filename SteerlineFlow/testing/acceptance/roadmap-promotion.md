# Roadmap 读取、解析与转正验收计划

## 2026-11-06：转正进度与当前流程展示

用户授权首轮界面调整：固定进度区、当前流程与历史分离、集中主操作及左侧草稿状态；不接活动中心。实现仅调整 Web 展示与原命令入口，不修改 Runner 请求、提示词、材料生成、文档发布或 Task/DAG 事务。

- 顶部吸顶区显示六阶段、当前状态、下一步说明和对应主操作；保留原发布/合并与自动派发的效果说明，提供任务/文档内容定位链接。
- 当前流程按最新解析及其 `parent_id` 关联的文档生成记录组织；未核实的内容请求和在途发布优先展示，不能被新记录遮盖。文档失败保留任务/依赖已确认的阶段，重试仍指向原解析记录。
- 历史以独立 URL `record` 参数查看，刷新可恢复；历史只读，顶部仍显示当前流程，返回后恢复当前操作。切换草稿重新限定流程与历史，清除正文编辑状态。
- 左侧按同一规则展示每份草稿状态。来源正文变化保留旧流程并要求显式重新解析；LF/CRLF 的原有等价规则继续适用，不归一化或写入来源。

实现期受测源码为 `01356834e4c791aba9c94064d5dfcb75f0d6b487` 加本轮工作树修改（dirty），当时尚未提交或推送；未部署。本轮原始证据根：`C:/Users/lmas1/AppData/Local/Temp/steerline-roadmap-ui-8306-e75c/`。提 PR 前固定提交的验证另行登记，不将这些 dirty 报告冒充 clean 版本证据。

| 验证 | 实际结果 / 证据 |
| --- | --- |
| 固定 L1 | `go run ./cmd/test-feedback -level=L1 -out <证据根>/L1`；passed/complete=true，准备 0.353s、执行 36.798s；`L1/summary.json` |
| 固定 L2 | `go run ./cmd/test-feedback -level=L2 -out <证据根>/L2`；passed=true，准备 11.692s、执行 130.454s；`L2/summary.json`，受控现场及资源清理按原报告核实 |
| 登记前端全集 | `go run ./cmd/test-feedback -level=L4 -suite=web -scenario web.unit -out <证据根>/web-unit-final`；39 文件 / 343 项通过，0 失败、0 skip，20.100s；`web-unit-final/summary.json` 及 `web-unit-final/web-unit/vitest.json` |
| 构建 | `npm --prefix web run build`；TypeScript 与 Vite 通过 |
| 浏览器界面定向 | 独立 Vite/Edge、模拟 API 数据，直接渲染实际 Roadmaps/AppShell；桌面、吸顶（滚动后 y=12）、历史/当前隔离、刷新恢复历史、返回当前、390×844 窄屏无整页横向溢出、未知结果保持原查询入口、只读导航零提交均通过；`browser/result.json` 与 `desktop.png`、`sticky.png`、`mobile-history.png` |

首次前端全集有一项新增测试在草稿列表返回前查找按钮而失败，修正异步等待后重新完整执行通过；失败保留于 `web-unit/summary.json`。浏览器临时夹具的 TSX 转换/开发连接准备失败未计通过；修正后执行上述界面检查，无页面异常。Roadmap 定向用例共 19 项，已包含在最终前端全集。

本轮未重跑真实模型、文档 PR 合并与入图整链；既有 live 场景只同步“已入图后显示 DAG 入口”的界面断言，不改变原事务与清理检查。浏览器模拟数据验证不代表真实后端链已复验；安装部署和用户人工体验仍未验。

### 提 PR 前固定提交验证

基于 `origin/main=27172ac3` 创建 `codex/roadmap-promotion-progress`，实际固定提交 `57da9c495ac668ddcd67d17b3ebdd49a3af612f8`。以下报告均核实执行前后为同一 clean 提交，证据仍在上方根目录，L1/L2/web 输出子目录分别为 `pr-57da9c49-L1`、`pr-57da9c49-L2`、`pr-57da9c49-web`；调用参数与上表相同，仅替换 `-out`。

- L1：passed/complete=true，准备 5.955s、执行 51.716s。
- L2：passed=true，准备 30.752s、执行 122.871s；受控交付与停止清理通过。
- web.unit：39 文件 / 343 项通过，0 失败、0 skip，34.681s。
- 同提交 TypeScript/Vite 构建通过；711 份 Markdown 的 4400 个本地链接检查 0 错误，`git diff --check` 通过。

三份登记测试报告已用 `test-feedback record` 导出到[固定提交测试记录](../records/57da9c495ac668ddcd67d17b3ebdd49a3af612f8.json)。随后只补充本段证据说明和测试记录，不将这些文档提交冒充另一次实现或浏览器复验。

## 2026-11-02：草稿换行比较修复

用户授权仅放宽草稿 LF/CRLF 内容比较；原始正文/摘要、固定文件 manifest、JSON 和原请求身份保持严格核验。原因为 Windows `core.autocrlf=true` 可让 Git 干净工作树出现 CRLF，而 blob 为 LF；不要求项目固定 LF。依据与命令见 [Spec](../../spec/roadmap-promotion.spec.md#41-来源) 和 [Plan](../../plans/roadmap-promotion.plan.md#2026-11-02草稿-lfcrlf-比较修复)。

| ID | 验收行为及通过条件 | 状态 / 证据 |
| --- | --- | --- |
| RP-NL-A01 | Git/工作树两个换行方向可解析，确认容许 LF/CRLF 变化；原始 Body/Digest 与实际 Git 材料字节保留 | 已验收；`TestDocumentDrivenRoadmapMaterials`、`TestRoadmapConfirmationToleratesOnlyCheckoutLineEndings` 通过，原始日志见下文 diagnostics |
| RP-NL-A02 | 正文、空格、空行、末尾换行、BOM、单独 CR、伪造摘要及 JSON 格式变化仍拒绝 | 已验收；同上及既有 JSON/manifest 反例通过 |
| RP-NL-A03 | 真实 Git 的 LF blob/CRLF checkout 经公开 fixture 解析、确认、生成、发布/合并及 SQLite 入图；发布前正文变化拒绝，原请求不重复 | 已验收；`TestRoadmapPublicationAndAdmissionWithCRLFCheckout` 通过，109.120s；仅受控验证，无真实模型或外部 PR |
| RP-NL-A04 | 固定 L1/L2、提示词原正文测试、链接/空白检查及独立审核通过 | 已验收；固定 L1/L2 和提示词测试通过，链接 0 错误、空白检查通过；独立只读审核无发现；未部署安装版 |

接线差异仅为草稿比较时将 CRLF 视为 LF；返回与存储仍使用各自原始字节，不归一化用户文件、不改写原请求或输出 Schema。固定 Markdown 缺失、图/映射/确认 JSON 缺失或不符仍在 submit 前阻断，没有补材料或内联兜底。

| 提示词/规则 | UTF-8 字节前 → 后 | 实际正文差异 |
| --- | --- | --- |
| `internal/prompts/roadmap-parse.md` | 253 → 253 | 无 |
| `internal/prompts/roadmap-documents.md` | 398 → 398 | 无 |
| `internal/prompts/materials/roadmap-parse.md` | 766 → 766 | 无 |
| `internal/prompts/materials/roadmap-documents.md` | 1363 → 1363 | 无 |

测试范围：新增内容比较反例、原固定材料两方向及受控 Git 整链；不更新黄金样本，不把源码测试通过写成安装版或真实模型已验。

实际受测为基线 `7d85a9e908102a4f12210bbba91408f39362e6a7` 加本次未提交工作树实现；报告保留 dirty 来源，不声称固定 clean commit 已验。原始证据根为 `C:/SteerlineDebug/wt-762b-roadmap-newlines/`：

- `diagnostics/20261102T065016447Z-0a5914e8/summary.json`：定向 Roadmap 共 44 项（含子项）通过，0 失败/测试 skip，271.085s；`go-test.jsonl` 保留原始断言。纯 model 包无独立测试文件，由 roadmap owner 直接覆盖，不作为测试通过数。
- `test-results/L1/20261102T065047916Z-2556fe9f/summary.json`：passed/complete=true，准备 0.440s、执行 31.039s；`test-results/L2/20261102T065047916Z-2556fe9f/summary.json`：passed=true，受控交付冒烟通过，准备 12.813s、执行 183.452s。
- 提示词原正文测试 `go test ./internal/prompts -count=1 -v` 通过，四份材料按 Git blob 对比均逐字相同，字节数见上表。独立审查者只读核对生产/测试/文档差异，确认原请求冻结、实际 Git 材料及 manifest 边界保持，无新增发现。

首次直接在普通沙箱运行的隔离 Git 测试因临时 `.git/config` 写入权限被拒绝，不计通过；正式定向及 L1/L2 在允许创建自有隔离 Git/进程的上下文执行，无安装环境或已有任务操作。静态检查只证明文档和差异格式，不代替实际整链结果。

日期：2026-10-26。归属：[Spec](../../spec/roadmap-promotion.spec.md) / [Plan](../../plans/roadmap-promotion.plan.md)。状态：用户已授权实施。本仓读取、解析、确认、发布/入图和正式 Web 已接入并进行隔离受控验证；2026-10-27 已接入合入的 Runner 固定仓库能力，真实 Codex 解析/生成与隔离 Git/SQLite 发布入图通过；正式 Edge 浏览器整链、真实 GitHub PR、入图及正常停服/清理最终已通过，最新证据见本文末尾；人工体验、Gitea 外部写入和安装部署未验。

原型页面模拟及静态检查不属于产品通过证据。原始产物按[路径清单](../../paths.md)保存在仓库外唯一目录；此文档只记录最小结论、源码/构建/环境和证据定位。执行级别、命令与前置映射见 Plan V1–V7 与后续授权第 7 节。

## 自动验收

| ID | 自动行为与反例 | 最小通过条件 | 方式 / 前置 | 当前结果 |
| --- | --- | --- | --- | --- |
| RP-A01 | 读取已纳管项目 drafts Markdown，无配套JSON；未提交来源、外部刷新、空文件/编码/超限/越界/重定向 | 原文字节与来源可追溯，错误具体；只读/刷新零模型、编号、文件写入或Task副作用 | V1/V2，临时Git与文件fixture | 本仓基础已实现；定向部分覆盖，整项未验 |
| RP-A02 | Runner generate同时识别任务与已有DAG依赖，保留作者代号/关系，未写代号给建议，区分原文与生成建议；模型返回正式号或错误来源 | 确实沿公开只读调用，输入完整；新正式编号不被采纳，结果来源/Schema正确，零Task/Candidate | V1/V2；V4另验真实Runner/Codex | 本仓基础已实现；定向部分覆盖，整项未验 |
| RP-A03 | 两份Roadmap均有T1、重复代号、跨草稿未入图引用、未知正式任务、环、范围重叠、源/图/规范变化及旧结果迟到 | 唯一身份和图判断正确；原关系/建议可审阅；关键问题不能确认；不按名称猜对应或静默替换 | V1/V2/V3，公开图fixture | 本仓基础已实现；定向部分覆盖，整项未验 |
| RP-A04 | 能力缺失，固定仓库/commit调查，整批设计与多份Spec/Plan，缺文件/截断/越界/路径冲突，正文修改与拆分/依赖变化 | 缺能力零生成；完整真实调查与文档依据；稳定语义路径；修改撤销确认，结构变化退回重确认 | V1/V2；V4及独立浏览器live专项使用实际Runner固定构建 | 能力门槛/文档集校验与真实固定版本生成通过；大仓库最大规模未验 |
| RP-A05 | 完整diff/目标、创建PR与合并的授权范围、文档分支/发布/合并失败和未知、错误仓库/base/head、未合入文档 | 原意图/原PR对账；只核实真实合并，未入图时零新正式编号；Markdown不改，无重复发布 | V1/V2/V3；V4真实专属文档PR | 受控发布反例及真实 GitHub 主链通过；真实 Gitea 外部写入未验 |
| RP-A06 | 合并后入图才编号；T2内部依赖T1转换；并发入图/重放/同号换正文/异号重复来源/取消历史号/任意写入点故障 | 一个批次编号/Task/Contract/DAG/来源映射/回执全提交或全回滚；不回收、不补洞、不重复；正式依赖正确 | V1 SQLite事务/故障注入，V3整链，V5回归 | SQLite事务、原请求重放及真实入图通过；其他反例按V1定向记录 |
| RP-A07 | 解析/生成停止与迟到接受、断线/重启、旧输入晚到、Git已合并未入图、SQLite已提交回执丢失、相关图变化 | 原请求/构建/输入/映射一致；未知只查原事实，无新模型/PR/编号；全部提交不能显示成已证实部分创建；停止/资源义务保留 | V1/V3/V6，原请求及公开fixture计数 | 原号受控回归与真实SQLite停止竞态通过；强杀原生现场未验 |
| RP-A08 | 正式API/Web从读取到入图、能力提示、只读/确认门槛、Task与来源双向跳转、项目/home/实例切换、自动派发提示 | 正式公开链消费同一事实，映射正确；迟到响应不串项目，现有测试生成/候选/派发/唯一交付规则不回退 | V2/V3/V5/V6；隔离Go/Web/Edge | 正式浏览器整链、双向导航、刷新与窄屏通过；项目隔离沿V2回归 |
| RP-A09 | 受控整链及真实Runner/Codex/文档PR整链，失败保留/独立复验，Run/提交/PR/最终图与资源清理 | V3/V4各自完整非零覆盖、实际终态、真实来源和必要清理；fixture与历史结果不顶替真实能力 | V3/V4 owner 链及独立 L4 live 脚本（用户已授权） | 原生内容、正式浏览器、真实 GitHub 文档 PR、入图与清理均通过；见最终表 |

## 人工体验

| ID | 场景与准备 | 参与人/窗口 | 截止及阻塞范围 | 当前结果 |
| --- | --- | --- | --- | --- |
| RP-M01 | 在新隔离包阅读带T1/T2的Markdown，区分原依赖/建议，确认后查看未编号文档，检查发布权限范围，入图后核对映射与双向导航及窄屏可读性 | 用户或指定体验人，窗口待安排；需自动核心链可用及已准备包/示例 | 产品体验签收前；可后置，不阻塞无关开发，不能豁免自动失败或缺能力 | 未安排 / 未验 |

## Runner 交接与入口准备

- Runner 已合入 aa8c392f9798d05184788d83947dd7bf4afc75a6，扩展 generate/1 的 repository 与公开来源；已按真实 capability/schema 接入，固定构建见下文。
- TestRoadmap* / Web 测试已实际运行。为避免重复驱动框架，V3/V4 使用同一个生产 owner 链测试（live 加 e2e tag）；没有登记两个虚构 L4 场景，不报告其通过。
- 本轮已按用户对接授权运行本机 Codex，实际 Git/SQLite 与 loopback provider 隔离，自动派发未启动；没有真实外部 PR/安装部署操作。
- 来源文件、已有正式任务材料、规范与模型结果须形成同一次固定输入证据。真实调查必须证明固定仓库/提交及实际查阅文件，多文档结果不得用截断正文取代。

## 结果登记

产品实现与验证均基于 HEAD 2c1cd9682b3e421f21b5935ccd73a06624015e42 加本轮未提交工作树，Windows/amd64、Go 1.26.4。真实生成/整链没有结果，不把 owner 或原型结果提升为 RP-A01–A09 整项通过。

| 日期 | 范围 | 源码/构建 | 命令或方式 | 结果与证据 |
| --- | --- | --- | --- | --- |
| 2026-10-26 | 文档静态 | 2c1cd9682b3e421f21b5935ccd73a06624015e42 上本轮未提交文档 | python scripts/check_doc_links.py；git diff --check | 两项退出 0；586 份 Markdown、3674 个本地链接、0 错误。日志：C:\SteerlineDebug\wt-1285-planning-ui\diagnostics\documents-20261026T095957986Z。不作为产品验收 |
| 2026-10-26 | 单文件原型检查 | 同一工作树的 planning-ui/index.html，无产品构建 | JavaScript语法检查；独立无头Chrome交互与桌面/窄屏截图 | 通过：内部代号/原依赖/建议、确认到合并零新编号、稳定文档路径、整批入图回执丢失后原映射恢复、两份T1隔离和已有任务关联。证据：C:\SteerlineDebug\wt-1285-planning-ui\diagnostics\local-ids-2026-10-26T09-58-37-762Z。真实Runner/数据库/PR均未调用，不计RP-A通过 |

实施后每次执行追加实际时间、固定SHA/工作树状态、Runner EXE/version/SHA、home/测试仓库、命令、范围、通过/失败/未完成及独立证据路径；失败和后续复验分别保留。生成文档、PR合并或有Task编号均不能单独证明本功能通过。

## 本轮实施与验证（2026-10-26）

已实施只读来源、Runner generate 解析/来源验证/确认、共享额度和原请求停止接管、Spec/Plan 文档集校验、固定文档 PR/实际合并核验、单一 Task 批次内自动编号/依赖/来源/回执事务，以及 Web 导航、历史、能力门槛、恢复和双向来源跳转。schema 32，不迁移旧开发库。产品设计及动作见 [Roadmap 转正](../../../docs/product-design/roadmap-promotion.md)。

### 定向覆盖范围

- RP-A01/A02/A03：任意 Markdown、未提交来源、边界/编码/大小、内部代号、引用/环/证据、原文或已有材料变化、原请求先保存、原结果重放与来源校验。公开 Runner fixture，没有真实 Codex 调用。重复解析不能把已绑定内部代号重新分配给新正式 Task。
- RP-A04：缺能力入口、确认摘要、完整文档集、固定 commit、缺失/重复/越界/编码/空正文/非 Markdown 拒绝；没有真实仓库调查或模型文档生成结果。
- RP-A05/A06/A07：显式未来能力文档 fixture + 真实临时 Git/bare remote/SQLite + loopback provider。创建 PR 已生效但回执丢失，查询原 PR、显式继续原合并授权且只创建/合并一次；源 Markdown 和主工作树不变，文档合并前后入图前均零新 Task。入图同事务分配编号、转换边、写来源与回执，重放、同号改正文、异号重复来源、回滚和并发两份 T1 已验；未做所有写入点故障注入或真实外部 PR。
- RP-A08：正式 Web 读取、原有/建议关系、显式解析信封、能力提示、断线禁写和映射跳转；共享请求号拒绝跨动作/正文替换、重复提交与登记后未知不重派。未运行正式 HTTP/Web→Runner 生成整链或桌面人工体验。
- RP-A09 / RP-M01：未完成，保持待前置/未安排。没有真实 Runner EXE 构建、usage 或外部 PR 清理证据；受控临时 Git/SQLite/provider 均由测试 cleanup 管理。

### 原始证据

证据根为 C:/SteerlineDebug/wt-1285-roadmap-promotion/。未提交源码状态、实际选择和原输出以各 summary.json / go-test.jsonl 为准；下表不会代替原始报告。

| 范围 | 实际入口与结果 | 证据（相对上述根） |
| --- | --- | --- |
| 早期 owner 定向 | roadmap/app/sqlite/testmanagement 的 TestRoadmap*；通过，48.352s | diagnostics/owner-20261026-a/summary.json |
| Git 发布/入图 | TestRoadmapPublication*；通过，48.446s，未使用真实外部 provider | diagnostics/publication-20261026-a/summary.json |
| 恢复与事务补测 | TestRoadmap* / 批次；通过，81.057s | diagnostics/owners-20261026-b/summary.json |
| 正式 Web | scripts/test-web.ps1，构建成功、31 文件 221 测试通过；Web server/httpapi/httpclient 定向通过。未传 DebugConfig，未运行浏览器整链 | diagnostics/web-20261026-b/build.log、client-tests.log、server-tests.log；后者指向受管 Go 原始报告 |
| 最终 owner / 边界 | 定向 11 包、241 项（含子用例）通过，零 skip，95.672s；含 Roadmap、批次、共享额度、HTTP 仲裁、Windows/Linux 全标签边界与等待状态回归 | diagnostics/final-owners-20261026-f/summary.json |
| 固定 L2 | 受控核心交付冒烟 passed=true；独立 Git/SQLite/公开 fixture，不使用真实模型 | test-results/L2/final-20261026-e/summary.json |
| 固定 L1 | 本轮最终实现回归通过，准备 0.3s / 执行 6.4s | test-results/L1/final-20261026-e/summary.json |

实现期间保留的失败：首次 boundary 发现 SQLite 依赖内容调用用例，已拆到 roadmap/model 并加入守卫正反例；并发现既有 execution_wait 测试把 testkit/protocol 带入纯契约/调用方依赖图，改为本地公开值 fixture / 从既有 schema 文件只读加载。旧 app HTTP consumer 守卫与已交付 Web 配置纯值边界不一致，已仅对 Web 两个入口允许 executionconfig/runnercontract，与现有全标签守卫一致。未放开 SQL、存储或业务实现依赖。失败原始记录在 diagnostics/boundary-20261026-b、final-owners-20261026-c（拆包漏引 Token 的编译失败）、final-owners-20261026-d（旧 app 守卫），复验另用新目录。

最终文档检查：587 份 Markdown、3684 个本地链接、0 错误；git diff --check 退出 0。文档结果日志见 diagnostics/static-final-20261026-g/；不作为真实产品整链证据。

## 2026-10-27 Runner 能力对接

本轮用户授权“Runner 能力已做好合入，对接完成完整功能”。复用 generate/1，未新增 workflow、运行时、发布平台或恢复框架。文档生成通过独立内容记录的 purpose=documents / parent_id 固定原确认；其 input、完整代码 commit/tree、三份必需规范和文档 schema 先持久化，再提交。两种内容操作共用现有原请求/停止/接管/额度。

生成结果核对原生 verified 来源、completed/stopped、Run 与 result 的 repository_source、预期 commit/tree，以及调查路径和 SHA-256。manifest 是可用固定版本证明，不当作模型已逐文件阅读的证明。完整 JSON 限 1 MiB；Schema 固定每个新任务的内部代号和 Spec/Plan/验收路径，缺文件/空正文/错误身份/改路径/越界/缺调查拒绝，关键问题只保存问题，不采用文档。

Web 已开放生成、独立请求历史/停止、代码提交/整体设计/调查引用、Spec/Plan/验收正文编辑和完整发布目标及所有文件前后内容预览。正文修改后重新形成审阅摘要。发布前重检原文/图/代码基线，同一来源的另一在途发布不能重复创建。仅同来源的生成索引可更新，其他现存文档不覆盖。正式入图仍在单一 Task 事务分配编号、转换依赖、写来源和原回执。

### 固定实际构建与环境

- Runner 源码：aa8c392f9798d05184788d83947dd7bf4afc75a6，clean；通过其 scripts/build.ps1 构建到仓库外目录，不改 Runner 源码或安装环境。
- Runner EXE：C:/SteerlineDebug/wt-1285-roadmap-promotion/runner-20261027/steerrunner.exe；version=0.2.0-dev.3-r3；SHA-256=882931d802971d8481c5c38fc28cb8c1e93edfa01ce38ac40cb235f562e0c0ae。
- 相邻 .build.json 保留源码 digest/commit/clean 和构建身份。capabilities.json 来自该 EXE 的公开 CLI；入仓仅保留 generate capability / generate_input 的公开 fixture。
- 本机 Codex：0.164.0-alpha.11.1，gpt-6.1-sol / low / read_only。用户登录/认证未复制。SteerLine 基于 2c1cd9682b3e421f21b5935ccd73a06624015e42 加本工作树改动，未提交、未部署。

### 本轮实际结果

证据均位于 C:/SteerlineDebug/wt-1285-roadmap-promotion/：

| 范围 | 结果与边界 | 原始证据 |
| --- | --- | --- |
| 初次受控 owner 链 | 通过，97.100s；不再植入“未来已生成结果”，从 parse → confirm → generate → observe → publish/merge → admit/reconcile 使用生产用例与公开协议 fixture | diagnostics/repository-20261027-a/summary.json |
| 真实本机内容 + 隔离发布/入图 | 通过，206.719s；两个任务、六份完整文档；原文不变、dirty 工作树不进入固定快照、零提前编号、内部边转换、原 PR 未知恢复与唯一合并、原入图回执恢复、资源停止 | diagnostics/live-20261027-b/summary.json、evidence/ |
| 完整自含项目规范的真实复验 | 通过；同样走实际 Runner/Codex 和隔离 Git/SQLite/provider。补存两类原公开 Runner observation，包含 native command/session/turn/verified 来源、仓库 manifest、完整输出与 stopped 事实；测试最后正常 service stop 和 resources_settled=true | diagnostics/live-20261027-complete/summary.json、evidence/review-runner-observation.json、documents-review-runner-observation.json、admitted.json、runner-settlement.json |
| owner/边界回归 | 243 项（含子用例）通过、零 skip，124.700s；含原队列/批次/调用方与 Windows/Linux 全标签守卫 | diagnostics/repository-final-20261027-c/summary.json |
| Web | 构建通过、31 文件 223 项通过；server/httpapi/httpclient 通过；未启动浏览器 E2E | diagnostics/web-repository-20261027-c/build.log、client-tests.log、server-tests.log |
| 固定 L1 | passed=true | test-results/L1/repository-final-20261027-e/summary.json |
| 固定 L2 | passed=true | test-results/L2/repository-final-20261027-d/summary.json |

最后新增的来源归属与防重用例以及最终回归另记下表，保留较早报告的真实范围；不会把先前执行包装成最终源码重跑。

### 保留的失败与修正

- live-20261027-a：测试启动时未先设置 EXE SHA，调用方身份校验拒绝，尚未 submit。已修复测试初始化并提前登记清理。该失败现场被 Go TempDir 清理移除 discovery，原空闲服务无法通过公开 CLI 停止；只在核实本轮 EXE、PID、精确 state-dir 和 startup-id 后结束自有空闲进程，没有原生执行或其他环境受影响。此记录不算正常停止证明。临时 capability 查询服务另以原 instance/request 正常停止，公开 last_shutdown.resources_settled=true。
- live-20261027-final：模型改写英文 evidence，原文逐字校验拒绝。未创建 Task；独立正常结算。现已把 evidence 的 Schema 收紧为 Markdown 原文行枚举，并明确禁止翻译/缩写，不放宽业务校验、不自动重发。
- live-20261027-schema：原生正确返回 questions 并阻止采用文档，因为测试把 SteerLine 自身的规范拷入算术项目，却未提供其引用的框架/规范。已将 fixture 改为自含的目标项目规范和已有 go.mod；未削弱生产缺规范/关键问题门槛。该失败的 failed-runner-observation.json 和正常停止证明保留。

### 前次验证时的未验边界

正式 HTTP/Web 浏览器完整交互、真实 GitHub/Gitea 外部 PR 写入、最大规模仓库/结果边界的原生现场及人工 RP-M01 未验。本轮公开 Runner 调用经生产 action/use-case 串联；HTTP 原信封仲裁与正式 Web 消费分别已有自动测试，不将它们合称浏览器 E2E。原生运行中强杀/未知发送故障由 Runner 上游及本仓受控原请求用例覆盖，本轮未重复真实故障矩阵。安装环境未更新。


### 最终受测状态（2026-10-27）

| 范围 | 最终结果 | 相对证据根 |
| --- | --- | --- |
| 原生内容与隔离转正 | passed，180.304s；两次真实 generate（解析、固定提交文档）、两个任务/六份文档、实际 Git 合并、Task/DAG/原映射、正常停止及资源结算。公开 observation 已保存，文档 Run=8b374b4adf880bc0645781f531f4bcc3、status=completed、stop_status=stopped、source.verified=true | diagnostics/live-20261027-complete/summary.json 与 evidence/ |
| 最终 owner/边界 | 248 项通过，零 skip，129.637s；含缺能力/缺规范零提交、原文行枚举、完整文档、问题阻断、原停止、在途发布防重及生成索引归属等 | diagnostics/repository-final-20261027-f/summary.json |
| 固定 L1 | passed=true，准备 0.7s / 执行 9.5s | test-results/L1/repository-final-20261027-f/summary.json |
| 固定 L2 | passed=true，受控核心交付回归 | test-results/L2/repository-final-20261027-f/summary.json |
| 最终 Web | 构建、223 项前端测试及 server/httpapi/httpclient 定向均通过 | diagnostics/web-repository-final-20261027-f/ |

最后静态检查日志归 diagnostics/static-repository-final-20261027-f/，仅证明文档链接、原型脚本语法和差异格式。无本轮 Runner 构建的残留服务进程。正式浏览器/外部 provider、人工体验及部署仍按上述边界保留未验，不改写历史失败。

## 2026-10-27 后续授权：正式浏览器与真实 GitHub PR

独立入口：`scripts/test-roadmap-browser-live.ps1`，消费 `web/tests/roadmap-live.spec.ts`。固定同一 Runner EXE（SHA-256 882931d802971d8481c5c38fc28cb8c1e93edfa01ce38ac40cb235f562e0c0ae）与本机 Codex，正式 Edge、正式 Go Web/HTTP/SQLite，未拦截或替换 Runner、API、GitHub provider。所有分支仅在 `erranliu/SteerlineTest`，专属随机 base/head；自动派发关闭。安装环境未变。

### 实际修复与原始失败

- `browser-pr-20261027-a`：测试 workflow 版本填错（1，而实际固定 Runner 发布 development/2），项目启动失败；零模型/Task/PR。原 verify-stop 因未成功启动的项目闭合事实不全而失败，保留原结果；之后独立公开 Runner 资源核验通过。
- `browser-pr-20261027-b`：Roadmap 点击后退回项目选择。修复 `use-application-projects` 漏登记的页面类型，补直接进入、切换及返回用例；正常停服及清理。
- `browser-pr-20261027-c`：Roadmap 刷新返回 asset_not_found。修复 Go Web SPA 路由漏登记，并补原页面、读取及原命令信封转发测试；正常停服及清理。
- `browser-pr-20261027-d`：真实 [PR #682](https://github.com/erranliu/SteerlineTest/pull/682) 已由产品合并并入图；末尾断言误把零 Run 的合法 null 当成数组，已修正。截图另发现浅色正文底色导致文字难读，改用现有深色主题并增加浏览器对比度断言。停服同时暴露入图后的测试安排生成可能碰到 stale_revision：两类内容 owner 现仅在确定拒绝后重新观察并准备新内部停止信封，对未知结果保留原号，Close 等待真实 stopped。新增确定拒绝/未知结果的四个 owner 分支。原失败现场经公开原 stop 重放确认拒绝、离线 takeover 修订值、pending_stops、service stop、verify-stopped 完成独立结算，未修改 Runner 私有数据、未强杀进程；不改写原失败报告。
- `browser-pr-20261027-e`：测试在文档请求尚未发出前主动刷新，HTTP 取消使本次请求明确 rejected（runner_unavailable）；零新 Task/PR，正常清理。刷新检查现等待原 Run 已受理后执行，核对不新增请求。
- `browser-pr-20261027-f`：浏览器全流程通过（5.2m），真实 [PR #683](https://github.com/erranliu/SteerlineTest/pull/683) 合并，六份正文与两份索引逐文件核对一致，T1→#0、T2→#1、依赖 #0→#1，原发布和入图重放不重复，DAG/来源跳转、刷新、窄屏及可读性全部通过。原 stop 已 clean=true、resources_settled=true；其后的离线 verify-stop 发现受管 roadmap-publications 未在状态目录白名单登记，原入口总结果保持失败。修复守卫并增加普通目录、文件替代及未知目录测试后，新构建在 `verification-20261027-g/stop-verification.json` 对原现场完成离线核验，两个原 Run 均 settled=true。

失败报告、trace、截图、原 JSON/回执、提交和 PR 均保留在证据根对应目录。A–E 的 `evidence/final-runner-resource-verification.json` 全部为 resources_settled=true；各轮自有远端分支均已清理，未改默认分支。F 后续核验单独保存，不覆盖其原 result.json。

最终完整入口复验与回归结果在下表追加；人工 RP-M01、真实 Gitea、最大规模原生边界及安装部署仍未验，不从本次两任务 GitHub 场景推断通过。

- `browser-pr-20261027-h`：完整浏览器再次通过（5.0m），产品合并真实 [PR #684](https://github.com/erranliu/SteerlineTest/pull/684)。停服出现 SQLite 原信封不可变校验与“已明确拒绝后新停止”转换冲突，内存 store 未能暴露此问题。现保留被拒信封并单独持久化 rejected；仅旧状态已持久化 rejected→unknown 且新内部 Request ID 不同才允许替换，unknown/accepted 不允许。补入真实 SQLite 的两类 owner Close 联测与持久化转换拒绝用例，停启 pending_stops 同步沿确定拒绝规则。H 原失败保持，资源通过公开原停止重放/确定拒绝/离线 revision/pending_stops/正常停服/verify-stopped 独立结算，证据为其 evidence/recovery-*.json。

### 最终完整入口与回归（2026-10-27）

证据根仍为 `C:/SteerlineDebug/wt-1285-roadmap-promotion/`，本表更新当前结论，上文各轮原始失败与当时边界保留。

| 范围 | 实际结果 | 相对证据路径 |
| --- | --- | --- |
| 正式浏览器及外部 PR 完整入口 | **passed=true，exit=0，cleanup_complete=true**；浏览器 5.7m。真实固定 Runner/Codex 解析及调查，两个任务六份完整文档、正文编辑/预览、真实 [PR #685](https://github.com/erranliu/SteerlineTest/pull/685) 创建合并、八份合并文件逐字核对、T1→#0/T2→#1、#0→#1；合并前/入图前零正式任务，原发布及入图信封重放不重复。直接刷新、DAG及来源导航、窄屏无整页水平滚动与正文对比度均通过 | browser-pr-20261027-i/result.json、evidence/browser-facts.json、evidence/browser.log、evidence/browser/（trace 与三张截图） |
| 正常停服与资源结算 | 同一入口原 stop 成功；公开 verify-stop 中两个原 Run 均 resources_settled=true，无独立补救步骤；根目录所有服务进程及监听为0；测试 base/head 删除、main 提交未变 | browser-pr-20261027-i/evidence/stop.log、stop-verification.json、external-pr.json、remote-before.json、remote-after.json |
| Owner/真实 SQLite/恢复 | 56 项通过，0 用例 skip；含停止确定拒绝与未知分支、真实 SQLite Close 联测及信封转换守卫、发布状态目录重开守卫 | diagnostics/final-owner-20261027-i/summary.json |
| 固定 L1 / L2 | 均 passed=true | test-results/L1/browser-pr-20261027-i/summary.json、test-results/L2/browser-pr-20261027-i/summary.json |
| Web / HTTP | 路由修复后前端31文件224项与 Go Web server/httpapi/httpclient通过；最终样式由本次实际浏览器、构建和对比度断言验证 | diagnostics/browser-pr-final-20261027/web-tests.log、http-tests.log；browser-pr-20261027-i/evidence/build-assets.log、build-web.log |

最终 PR merge commit=`b78746c19bc9d9a64d6b50bd911394ce638f8603`，文档 head=`944a3c3b174059fe9e5c2e02a0e1fda9d5d15f8b`；实际冻结源码与三个 EXE 摘要保留在该轮 evidence/build.json。测试生成的任务留在隔离 SQLite，自动派发关闭、无正式执行候选；测试 PR 保留，只有本轮专属远端 refs 被删除。

本次不覆盖人工 RP-M01、真实 Gitea、最大规模仓库/文档原生现场、强杀故障矩阵或安装部署，不把两任务 GitHub 场景扩写为这些范围通过。

### 提 PR 前同步主分支验证（2026-10-27）

功能提交 `227e23ee` 后同步 `origin/main=3ca70df63c677657cc617ddf4557a79844e4fc7b`。冲突处理保留主分支项目环境适配的命令、只读路由、辅助观察、takeover/正常停服与 result-file 能力，并保留 Roadmap 对应分支；两类业务共用原请求空间及内容队列。主分支已使用 schema 32，本功能的新增表组合改用 schema 33，按未发布项目规则拒绝旧开发库，不添加迁移或兼容读取。

同步后在相同工作树执行：

| 范围 | 结果 | 相对证据路径 |
| --- | --- | --- |
| 固定 L1 / L2 | passed=true | test-results/L1/pr-main-20261027-a/summary.json、test-results/L2/pr-main-20261027-a/summary.json |
| Roadmap / SQLite / Task / Runner / HTTP 定向 | 37项通过、0 skip | diagnostics/pr-main-20261027-a/summary.json |
| 主分支环境适配准入、命令及恢复定向 | 9项通过、0 skip | diagnostics/pr-environment-20261027-a/summary.json |
| 前端与构建 | 32文件233项通过；tsc及Vite构建通过 | 本聊天实际 npm test / npm run build 输出 |
| 文档 | 594文件3755个本地链接，0错误；git diff --check通过 | 本聊天实际静态检查输出 |

前述真实 Runner/Codex/Edge/GitHub #685 整链属于同步主分支之前的固定源码现场。本次同步后重跑上述受控回归与 Web 构建，未把旧真实链标记为同步后的重新执行；原证据及未验范围保留。
