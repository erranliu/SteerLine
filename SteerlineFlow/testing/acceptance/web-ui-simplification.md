# WebUI 信息精简与进度展示：验收与审计记录

日期：2026-11-10。用户在文档审计后授权按 Spec/Plan 实施、测试并独立审计。普通开发，不分配 Task ID，不修改 roadmap。关联 [Spec](../../spec/web-ui-simplification.spec.md)、[Plan](../../plans/web-ui-simplification.plan.md)及[测试安排](../../plans/web-ui-simplification.tests.json)。实现与自动验收完成，独立审计发现均已修复并复核；未部署；用户后续授权提交、推送并创建 PR，交付进展见第 9 节；人工体验 WUS-M01 后置。

## 1. 准备、环境与证据

调查代码基线：`d1fe6f771813259c12ebdbf247021ee5b4de67bb`。实际验证基于该 HEAD 的未提交实现，不冒称干净提交通过。最终 38 个前端实现/测试文件和 11 个生产资产的 SHA256 保存于第 7 节 implementation-manifest.json；测试后再次逐字节摘要核对一致。受测版本、命令、报告及截图/请求记录见第 7 节。

实施验证需要 Windows、Go/Git、Node 24+、npm 依赖与 Edge。Plan P5 使用正式四个测试 ID、生产构建和显式文件过滤的专属 Playwright；测试定义引用[正式清单](../test-list.json)，不复制或缩小定义。本机 Windows amd64、Go 1.26.4、Node 24.13.0、锁定 npm 依赖与无头 Edge；B1–B4 中落实 B5 的生命周期/查询频率/键盘/三宽度要求。静态服务仅监听回环随机端口，API 夹具明确标识，afterAll 关闭连接/服务；受控 E2E 清理见原报告。未接入安装环境、真实模型或真实 PR。

证据根规划：`C:\SteerlineDebug\wt-c06b-web-ui-simplification`，每次执行按[路径清单](../../paths.md)创建独立叶目录。正式分级报告以本次 summary/managed 原报告为入口，浏览器/构建/定向诊断单独记录，失败与补测不覆盖。实际目录及原始报告见第 7 节。

## 2. 自动产品验收

以下均为自动验收，已按实际实现/驱动执行，不由人工走查代替；失败会阻塞本功能交付。场景细节及通过条件唯一引用 Spec，代码位置、测试映射和执行时点引用 Plan P2–P5。逐项结果如下，原始证据统一定位第 7 节，不用人工体验或 PR 状态替代。

| ID | 方法与步骤定位 | 最小通过条件 | 当前状态/结果与证据 |
| --- | --- | --- | --- |
| WUS-A01 | Plan P2/P3、web.unit、B1/B4 | 四区与技术字段退出、复制原绑定 | 已验收；Web 全集、B1/B4：四区、技术字段移除及原复制绑定通过；原始证据见第 7 节 |
| WUS-A02 | P2、web.unit、B1 | 阶段真实性、无伪进度、独立状态 | 已验收；Web 映射/状态测试、B1：未知及审查回退到实施、无伪百分比通过；原始证据见第 7 节 |
| WUS-A03 | P2、web.unit、B2 | 等待各阶段/计时/耗尽/陈旧、零执行副作用 | 已验收；Web 等待时钟/资格反例、B2：排队/调用/退避/耗尽/清除及陈旧通过；原始证据见第 7 节 |
| WUS-A04 | P2、web.unit、B1/B2 | 消息/活动边界、展开、源时间及错误保留 | 已验收；Web 消息/活动、B1/B2：六行/五条、展开保持、原时间与错误保留通过；原始证据见第 7 节 |
| WUS-A05 | P3、web.unit、B1/B2 | 真实测试、历史/过期、分轮审查及正文可达 | 已验收；Web 测试/审查/材料反例、B1/B2：真实计数、历史/过期、缺结果不补零通过；原始证据见第 7 节 |
| WUS-A06 | P3、web.unit、B3 | PR/交付/资源与历史 final 分开、完整性准确；终态直接打开/刷新/切换及运行转结束默认展开，尊重主动收起 | 已验收；Web 交付/final、B3：结果维度、冷进入/刷新/切换、历史/完整性及主动收起通过；原始证据见第 7 节 |
| WUS-A07 | P2/P3、web.unit、B2/B3 | 待答/阻塞直显、原动作与最后候选保护 | 已验收；Web 回答/停止/冲突及原资格、B2/B3：问题直显、参与退出与资源区分通过；原始证据见第 7 节 |
| WUS-A08 | P4、web.unit、B4 | DAG/启动/派发/文档/边界与禁用原因 | 已验收；Web DAG/任务/派发回归、B4：依赖/边界/启动资格及文档入口通过；原始证据见第 7 节 |
| WUS-A09 | P4、web.unit、B4、原依赖审阅浏览器 | Roadmap 原正文、依赖/发布审阅和历史/原请求 | 已验收；Web Roadmap 全集、B4 与原依赖审阅浏览器：正文/发布目标/历史与原确认范围通过；原始证据见第 7 节 |
| WUS-A10 | P4、web.unit、B4、原环境总览浏览器 | 全部环境与维护进度/阻塞/原请求定位 | 已验收；Web 环境/维护、B4 与原环境总览浏览器：非空全部环境三宽度、源时间/阻塞/深链接通过；原始证据见第 7 节 |
| WUS-A11 | P4、web.unit、B4、runtime.async-activities | 活动/反馈/未知计数/唯一连接/项目切换 | 已验收；Web 活动/原请求/反馈、B4、正式活动整链：筛选/未知计数/可读目标/跨页发起通过；原始证据见第 7 节 |
| WUS-A12 | P3、web.unit、B1/B3 | 费用估算及部分/旧数据限定、原明细 | 已验收；Web run-usage 全集、B1/B3：估算/覆盖/部分/旧数据与明细通过；原始证据见第 7 节 |
| WUS-A13 | P2–P4、web.unit、B5、runtime.async-activities | 身份隔离、零额外执行、无重复或加频查询 | 已验收；Web 身份/晚到/可见性/原请求、B1/B3 查询窗口、正式活动整链：零额外执行及原绑定通过；原始证据见第 7 节 |
| WUS-A14 | P5 生产构建、B1–B5、键盘/ARIA 测试 | 三宽度/导航收起/各状态 DOM 和截图及焦点/溢出 | 已验收；生产构建、7 项浏览器与 Web 键盘/ARIA：三宽度、导航收起、长正文/异常/历史/结束、DOM及截图通过；原始证据见第 7 节 |
| WUS-R01 | 正式 l1.minimum/smoke.delivery/web.unit/runtime.async-activities | 原定义实际非零、零 skip、终态和清理完整 | 已验收；四个正式 ID 全部通过；Web 406/406、零跳过，L2/L3 cleanup_complete=true；原始证据见第 7 节 |

## 3. 人工体验

WUS-M01：用户观察信息密度、阶段变化的可理解性、长时间运行的动态感及原生查看体验。方法：在自动验收完成的构建中，按 Spec 的运行/等待/待答/交付场景走查。最小通过条件：用户能辨认当前阶段、最近变化、待处理事项和结果，原生查看符合原执行目标。

参与人、窗口待安排；基于本次已验证实现交付，状态“待验收（人工窗口待安排）”。可在自动项全部通过后后置，只影响本次主观体验确认，不阻塞无关开发；自动功能缺失/失败或真实前置不满足不能移入本项。

## 4. 文档检查与独立审计

本区仅记录此前文档阶段，不与 WUS-A/WUS-R 产品结果混写。文档阶段未修改生产实现；后续实施与代码审核见下节。本任务始终没有修改固定提示词、黄金样本或相关 CI。

| 检查 | 实际结果 |
| --- | --- |
| 本地 Markdown 导航 | 2026-11-10 执行 `python scripts/check_doc_links.py`：730 个 Markdown 文件、4518 个本地链接、0 错误 |
| 测试 JSON 格式、正式 ID 存在且有序唯一、Plan 选择一致 | Python 读取本次 JSON 与正式 test-list.json，四个 ID 均登记且按 Plan 顺序唯一，检查通过 |
| Spec/Plan/验收的 A01–A14、M01 映射与索引一致 | Python 核对三文档均含全部 15 个 ID，逐项对照 Plan 映射及六份交付文件，检查通过 |
| `git diff --check` 及新增文件空白检查 | `git diff --check` 退出码 0；另以 Python 核对全部六份交付文件的行尾空白与 EOF，全部通过，包含未跟踪新增文件 |
| 独立只读文档审计 | `audit_spec_plan` 与 `audit_feasibility` 分别完成初审及修订复核；一项 P2 已关闭，一项验证口径建议已澄清，最终无未解决发现 |

审计发现 WUS-D01（P2）：Plan P3 原先把结果区默认展开限定为同身份运行转结束，遗漏直接打开深链接、刷新或切换到已结束候选，与 Spec 的结束后优先展示结果不一致。已在 Spec 第 6 节、Plan P3/WUS-A06/B3 和本记录补齐这些情形，并保留用户主动收起后同身份轮询不重开的规则。两位审计者分别独立确认同一问题，按一项登记；修订后两位均复核通过，已关闭。

审计建议 WUS-D02：B5 的请求次数对照补充相同查询启用/展开窗口，单列验证结束态默认展开引起的首次 final 读取，避免把已要求的正常读取误判成重复查询。已补充 Plan B5，`audit_feasibility` 复核通过；这是验证口径澄清，不新增业务范围或豁免重复订阅/提高轮询频率。

文档阶段复核内容的 SHA256（实施状态更新之前的版本，本区不作为自身摘要输入）：

| 文档 | SHA256 |
| --- | --- |
| Spec | `033F32D703EBFD57B9A5306AA964B53FEB2FEF4C9E1FE203224E09BCA8932A20` |
| Plan | `4EAEC53427EF2BCFF92FF691E9987BE818A02BD01067C0BAFDC0D9FE000B39F5` |
| tests.json | `0BB89F24B2A0089C0D74041C1FF6AB8DF69E2A1EB77A38461FFC3E52EE57BC45` |

文档审计不运行 UI 或真实业务，结论仅适用于实际审阅的文档版本；发现功能设计问题先修正规格/Plan 后复核，不伪造实施通过或真实链证据。

## 5. 实施取舍与前后对照

| 原信息/入口 | 实际整理 | 保留的事实/操作 |
| --- | --- | --- |
| `runs.tsx` 标题、原 Output 与页尾 PR | 四个主要区域；原活动与 final 查询各保留一处 | 候选/方案、独立待答与需处理、原生查看、停止参与、冲突解决、复制 |
| Run/Execution/Attempt/Turn、工作区、冻结方案/材料及结果来源 | 删除独立字段/面板；回答区技术目标尾注也删除 | 原查询 key、原信封/版本、草稿、控制资格和复制内容保留 |
| LastMessage、活动列表 | 六行可展开消息、默认五条非内部活动、其余已返回记录按需展开 | 来源时间、读取失败、内容与展开状态保留，内部事件不充当推进 |
| 等待技术详情、审查来源 | 删除操作标识/预算表/来源 ID；当前等待、未核实和失败摘要直显 | 真实等待/重试计数、到期不自行推进，审查按记录计数且不补零 |
| 测试与文档 | 真实计数和测试条，命令/证据/文档差异按需查看 | 候选与测试材料状态保守合并，过期不展示当前通过条 |
| 原交付观察与 PR 重复区 | 合为单一主要 PR 链接/状态，final 独立控制默认展开 | 原生结果/任务交付/资源停止/完整性分开，历史回复不冒充本轮 |
| DAG、任务详情 | 删除图/Task 版本、空依赖区及重复正常说明 | 真实依赖、图边界/分页、方案/派发/启动与禁用原因 |
| Roadmap | 删除生成 JSON 页签、来源哈希；设计/调查依据按需查看 | 原文、依赖审阅、全部文档种类、发布目标和前后正文、历史/问题/原请求 |
| 环境、维护、活动、全局栏 | 删除独立检测 ID、实例 ID、实现页脚；正常维护收为一行，活动去重复类型 | 全部环境、就绪/异常、源核实时间、三筛选与未知计数、精确跳转/原请求核实 |
| 操作回执 | 当前实例未结及最新拒绝直显，其他历史折叠且最多十二条 | 可读动作与可信业务目标；旧请求仍沿原信封核实，原 API 拒绝跨实例补交 |

最初截图夹具的最近消息缺少原查询所需身份字段，初步截图不作为最终同夹具对照。已纠正夹具并从调查 HEAD 单独构建原 Web 资产到 `C:\SteerlineDebug\wt-c06b-web-ui-simplification\diagnostics\baseline-corrected-20261110\dist`，用与改版后相同的 API 事实补拍三宽度。基线构建前逐字节备份本任务的 28 个已修改跟踪源文件，finally 全部恢复并校验；原始备份/摘要、构建日志和浏览器证据均在该目录，不创建额外 Git worktree、不改业务状态。原三宽度截图高为 2489/3087/3349px（1440/800/375px 宽），只对照这份固定夹具，不推断其他真实任务的高度。

## 6. 独立代码审计与修复

`audit_ui_implementation` 和 `audit_ui_coverage` 是未参与实现的独立只读审计者；没有修改文件、运行测试或代主进程登记测试通过。合并重复发现后共六项 P2、一项 P3，均已修复并由原审计者复核，无未解决发现。

| ID | 级别 | 发现、修复与反例 |
| --- | --- | --- |
| WUS-C01 | P2 | 候选文档关闭重复提醒后，测试请求失败/旧缓存会掩盖候选过期或未知。PlanTests 直接保守合并 run 与 view 状态，outdated 优先；新增 pending/失败/旧 current 缓存反例，过期不显示当前通过条 |
| WUS-C02 | P2 | 不完整审查空数组会显示零遗留。有效完整结果才显示零；失败修复数带待核实限定，实际遗留条目直显，未核实空列表不补零 |
| WUS-C03 | P2 | 删除技术回执表后独立 reply/message/stop 命令未知/失败可能消失。当前进度从同快照 commands 聚合短提示，不依赖浏览器 journal 或 Run 正常状态；增加独立结果反例 |
| WUS-C04 | P2 | 部分待核实动作都显示“执行操作”，丢失辨识。补齐中文动作名及可信任务/候选上下文；缺源轮次字段不能相互匹配，旧实例不借当前对象；保留原复制/信封 |
| WUS-C05 | P2 | 维护异常缺少最后可信时间。异常/展开时显示 observation.last_success，来源没有时间则明确未知；新增两种反例 |
| WUS-C06 | P2 | 全部历史拒绝/旧实例未知永久铺开。当前未结与最新拒绝直显，其余有界折叠；多条历史与当前未知反例通过，不用后来的成功覆盖原结果 |
| WUS-C07 | P3 | final 读取失败保留正文却没有可信时间。异常时显示 dataUpdatedAt 并标为“最后成功读取”，不当作业务进展；增加缓存后读取失败反例 |

修复验证另外捕获并修正了两点：业务目标匹配须显式提供原 source_run_id/source_attempt_id，防止缺失值相等；历史原请求查询不应被 UI 额外禁用，原 Api.reconcile 已阻止跨实例补交。原 app 级测试改为先展开历史，保留原信封、并发核实及仅失效旧 scope 的完整断言；该最终修正也已由 `audit_ui_coverage` 独立复核。

最终全部测试结束后，`audit_ui_coverage` 又独立只读核对本记录及真实 summary/managed/scenario、406 项 Vitest、7 份浏览器 trace/42 张截图、清理标记和 38 份源码/11 份产物摘要，确认均一致；旧失败证据、未提交状态、受控验证与人工待验边界表述准确，无差异或未解决发现。该取证复核没有重新运行或修改测试。

## 7. 最终实际验证与证据

所有原始路径均位于 `C:\SteerlineDebug\wt-c06b-web-ui-simplification`，每次执行使用独立目录，无自动重试。2026-11-10 北京时间执行；目录时间戳为 UTC。正式入口完整参数按以下实际选择执行，并分别显式传对应 `-out`：

| 内容 | 实际命令/选择 | 原报告相对上述根目录 | 结果 |
| --- | --- | --- | --- |
| 前端生产构建 | `npm --prefix web run build` | `diagnostics/verified-20261110T032748469Z-0bf383729c054414a9a132a15a7e5e27/build.log` | TypeScript 与 Vite 构建通过 |
| Web 全集 | `go run ./cmd/test-feedback -level=L4 -suite=web -scenario web.unit -out <下列目录>` | `test-results/L4/20261110T032751059Z-5249c40e7b534a1c95b9fc25906d8024/summary.json` | 406/406，通过，零失败/跳过；原生 JSON 在同根 web-unit/vitest.json |
| 固定 L1 | `go run ./cmd/test-feedback -level=L1 -out <下列目录>` | `test-results/L1/20261110T032808821Z-9012e0d33de249afaf76ca9ab42dafee/summary.json` | passed=true，执行 16.96 秒 |
| 固定 L2 | `go run ./cmd/test-feedback -level=L2 -out <下列目录>` | `test-results/L2/20261110T032826651Z-0789bc6c93ad411aa6a14928747c7997/summary.json` | passed=true，执行 78.36 秒，cleanup_complete=true |
| 专属/既有浏览器 | `npm --prefix web run test:e2e -- ui-simplification.spec.ts roadmap-dependency-review.spec.ts environment-overview.spec.ts execution-info.spec.ts` | `diagnostics/verified-20261110T032748469Z-0bf383729c054414a9a132a15a7e5e27/browser.log` 及同根 browser/ | 7/7，通过，31.2 秒；有截图、trace、请求记录 |
| 活动受控整链 | `go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled -scenario runtime.async-activities -out <下列目录>` | `test-results/L3/20261110T033022664Z-0334ac8ee740476da94d988b6b3c9e5f/summary.json` | passed=true，执行 241.18 秒，cleanup_complete=true |

表中 `-out` 均为对应 summary.json 的父目录绝对路径。L2/L3 的 managed 文件及 `scenarios/<id>/scenario.json` 同根保存，场景报告也逐项核对 passed=true、cleanup_complete=true。原命令、构建 SHA、实际测试选择及清理凭据沿这些报告定位；不把定向诊断、截图或受控接口当成真实 Runner/GitHub 结果。

最终诊断根 `diagnostics/verified-20261110T032748469Z-0bf383729c054414a9a132a15a7e5e27` 中另有 implementation-manifest.json 和 visual-comparison.json。浏览器输出由该次 STEERLINE_WEB_EVIDENCE 指向 browser/，finally 恢复原变量；同一源码的构建与各项验证串行。

同夹具默认执行页的高度对照如下，仅说明本夹具的版面变化：

| 视口宽度 | 原页高 | 精简后页高 | 减少 |
| --- | --- | --- | --- |
| 1440px | 2489px | 1762px | 29.2% |
| 800px | 3087px | 2227px | 27.9% |
| 375px | 3349px | 2485px | 25.8% |

基线构建/截图在 `diagnostics/baseline-corrected-20261110`，实际运行采用 STEERLINE_UI_BASELINE=1、STEERLINE_UI_ASSETS_ROOT 指向其中 dist，并显式 `--grep 'baseline corrected fixture'`；该入口只为复核改版前对照，正常七项验收不注册此基线用例。实际视觉走查包含桌面默认页、窄屏待答、全部环境与实际依赖图；产品主观体验仍由 WUS-M01 单独登记。

浏览器 trace 离线检查仅发现故障注入产生的两条 HTTP 503 和三条 ERR_FAILED 控制台资源错误，与环境/测试/消息/快照故障分支对应；没有额外脚本异常。它们是错误状态验收输入，不冒称网络全程无失败。请求记录和 B3 同启用窗口断言证明未增加重复 final/activity 订阅或轮询频率；结束态首次 final 读取按既定要求单列，不当作额外业务输入。

## 8. 中间失败、修复与限制

- 首轮定向/全集诊断用于更新已退出的技术展示断言，保留原业务反例。初次 Vitest 391 项中 22 项失败，原 JSON 位于 `C:\Users\lmas1\AppData\Local\Temp\wus-initial-vitest.json`；后续 `wus-targeted.json`、`wus-audit-fixes.json` 同目录保留当轮结果，不替代最终正式报告。新增测试夹具缺少必需字段导致的类型检查错误也已修正。
- 初轮浏览器 6/7，等待标题同时出现在候选卡片和详情导致严格选择器失败；`diagnostics/browser-first.log` 及 browser-first/ 保留失败。该轮使用此前可构建资产，不作为最终实现验收；限定详情选择器后补测通过。早期 before-browser/ 的消息夹具问题由第 5 节同夹具基线重建取代，未覆盖旧证据。
- 审计修复后的第一次正式 Web 为 405/406：原 app 级手动核实测试发现历史请求查询被额外禁用。失败报告保留于 `test-results/L4/20261110T032400883Z-fbae6095b2e94579a6a41e2945851104`。恢复原信封核实能力、保留 API 跨实例禁止补交和完整原断言后，最终 406/406。
- 审计前的 L1/L2/活动整链及较早浏览器结果留在原独立目录，不作为最终代码的唯一依据；审计修复后上述正式四项与浏览器全部重跑。没有导出干净提交的 Git 测试历史文件，也没有提交、推送、提 PR 或部署。
- 本次只证明前端受控展示/交互及所选受控整链，不证明真实模型、真实外部 PR、安装现场或原生窗口体验；这些均未操作。WUS-M01 人工体验后置，尚未验收，不影响自动实现交付。

## 9. PR 交付

2026-11-10 用户明确要求提 PR，并再次明确不部署。创建 codex/web-ui-simplification 分支提交本任务变更；提交前核对 38 份已测源码与 11 份产物摘要一致。获取远端 main 后发现其新增主 Agent 与依赖预览精简，PR 将保留这些已合入能力，合并后验证另行登记；前述 d1fe6f77 基线的历史结果不冒充新基线验证。
