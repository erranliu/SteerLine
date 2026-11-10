# Plan：WebUI 信息精简与进度展示

日期：2026-11-10。状态：已按用户后续授权实施，自动验证通过，独立代码审计发现已修复并复核。依据 [Spec](../spec/web-ui-simplification.spec.md)，测试选择见同名 [tests.json](web-ui-simplification.tests.json)，实际结果与审计记录归[专属验收](../testing/acceptance/web-ui-simplification.md)。实施步骤及通过条件保留，结果只在专属记录维护；用户后续已授权提交、推送并创建 PR，仍不合并或部署，不进入 roadmap。

遵循[编写规范](../../SteerlineProjectGuideDoc/spec-plan-guidelines.md)、[测试指南](../testing.md)、[验收流程](../testing/acceptance/README.md)及[路径清单](../paths.md)。实施步骤按依赖顺序推进，普通开发由当前聊天执行，不因步骤划分创建独立工作树或调度任务。

## 1. 调查结论与实施范围

调查基线与 Spec 相同：`d1fe6f771813259c12ebdbf247021ee5b4de67bb`；编写时另查看了实际候选/最终回复类型、原查询、组件测试、Playwright 配置、环境/执行信息/依赖审阅浏览器测试及正式测试清单。实施前再次核对实际基线，验收使用真正受测版本，不能沿用调查 SHA 声称通过。

| 已有能力 | 实施决定 |
| --- | --- |
| `features/runs/state.ts` 已区分执行、待答、需处理、候选参与与任务交付 | 复用原函数与筛选计数，不另写统一执行状态机，不改变资格规则 |
| `Candidate.runner_delivery.step/phase/status`、`workflow_stage` 仅提供当前事实，没有完整工作流拓扑 | 以当前阶段文字和既有活动变化为本次交付；不实施固定阶段条。只对已核对的阶段名做纯显示映射，未知保守显示，不推断通过前序阶段 |
| 最终回复按 Execution/Attempt 读取，正文无逐条 Turn 标识 | 当前执行结束后展示最近已取得回复，仍明确包含历史；不按排序补造本轮归属，不扩展接口 |
| 等待、消息、测试、用量各有独立查询和时间/可见性逻辑 | 调整渲染位置及 props，保留原 owner、查询 key 和缓存；不为整合再拉同一数据 |
| 现有多个 Playwright 文件能服务 `web/server/dist` 并拦截 API | 按已有测试方式补本任务的展示场景，使用真实生产资产与明确的受控事实，不搭建新的产品测试服务或模型模拟器 |
| 当前 `runs.test.tsx` 明确断言冻结模型/审计来源可见，浏览器文件也包含既有文案选择器 | 按 Spec 修改已退出的展示断言，同时保留来源真实性、未核实不当成功、原身份和零额外写入断言；不能直接删测试来消除失败 |

生产修改限于 `web/src/features/` 相关展示组件及局部样式、`web/src/app/app.tsx`/`app-shell.tsx`/`operations.tsx`、必要的显示辅助函数和 `web/src/styles.css`。不改生产 API、journal、原请求 hooks、业务 owner 或权限门槛；展示组件仍消费其结果。新增显示函数留在所属 feature，只有实际重复时共享小型展示能力，不建立通用描述器、注册表、数据协调 hook 或缓存层。

## 2. P1：固定展示映射与改版前对照

1. 为 Spec 第 3 节的信息逐项标记原组件、保留位置、删除位置和默认/展开状态。原生技术 ID 的删除只作用于独立字段，不清洗错误正文或业务文档；原复制格式和原请求绑定保持。
2. 固定当前阶段映射：优先使用可核实的 `runner_delivery.step`，否则使用同候选的 `workflow_stage`。已知准备/实施/测试/审查/修复/报告/发布/待合并阶段转中文；状态、等待和参与资格分别渲染。没有数据时显示阶段待核实，不用 Agent 文本推断阶段。
3. 固定 PR、TaskDelivery、原生结果、最终回复和资源停止的展示来源及冲突表达。`run.pr`、`runner_delivery` 仅表达各自观察，TaskDelivery 从已有 Task 查询取得；明确待合并与已合并待交付，不能凭阶段或 `closed` 推断任务交付。
4. 使用与后续相同的受控夹具、相同视口为旧布局保存对照截图和主要区域顺序/尺寸。若实现时旧页面已变化，记录真实基线，不复制本任务之外的旧截图冒充前态。

P1 的表和截图定位写入专属验收；P2–P5 以该表核对完整内容和去重，防止删掉副作用说明、发布目标或重要错误。

## 3. P2：重组执行详情与最近动态

涉及 `runs.tsx`、`delivery-summary.tsx`、`runner-workflow.tsx`、`last-message.tsx`、`execution-wait.tsx` 及所属样式。原“输出”包含活动与最终回复两个 tab，需拆开渲染职责：活动并入动态，最终回复进入交付；不保留原输出面板再叠加新四区。

1. 将标题、阶段/状态、必要动作和等待摘要组成“当前进度”。继续使用 `candidateStatuses` 等原判断；`Answer`、恢复/环境阻塞及当前错误置于进度后、动态前。合并同源重复提示，不吞掉同时运行和非阻塞待答。
2. 移除 `run-meta`、工作区路径块、冻结参数/材料及技术审计来源、命令回执与可用动作说明长列表。保留现有 stop/冲突解决/原生查看/复制组件，不以隐藏按钮代替资格原因。
3. 将 LastMessage 的数据获取和时间逻辑保留在原职责内，动态区域默认六行预览、最多五条非 `runner_event` 活动；用户展开全文、单项详情和其余已返回记录。用活动 seq 维持原事件身份，不以文本相似度合并，不新增日志读取。
4. 等待摘要沿 `preparationWait` 和 `ExecutionWaitSummary` 的时钟/可见性逻辑，删除等待技术详情；保留排队、查询、退避、耗尽及实际计数。到期、陈旧、无基准分别保持原保守语义；只改变显示，不调 Runner。
5. 本地展开状态以项目/实例、候选、Execution/Attempt 绑定，普通快照 revision 变化不重挂整个详情；活动按 seq、回复按 item_key/revision 定位。切换真正执行身份重置，重查同身份保留选择/展开/焦点；沿原草稿 key 绑定回答。

依赖：P1。先扩展受影响 Vitest 行为测试再替换旧区域，确保动态、问题和等待均只有一个主要位置，技术字段退出不会导致原请求/版本校验被删。

## 4. P3：整合测试、审查、文档和交付

涉及 `plan-tests.tsx`、`runner-workflow.tsx`、`candidate-documents.tsx`、`delivery-summary.tsx`、原 final 渲染和 `run-usage.tsx`。复用已有 API 类型，不能将展示摘要回写候选。

- “测试与审查”默认保留真实测试计数、分步骤审查摘要及直接可见的遗留/失败提示；逐项测试命令、检查要求、修复说明和证据折叠。unknown/missing/invalid/零项/历史关联/过期分开处理；不将 false 解释为失败，多轮修复不相加成去重问题数。
- 任务文档入口收为一处，保留按需请求、执行内容/最新内容/差异及材料当前性提示；删除仅展示提交哈希的版本行，不丢失文档对照语义。材料提醒可在共同父区去重，测试关联失效的具体影响仍可见。
- 交付区承接唯一主要 PR 链接、缓存时间/错误、实际交付与资源摘要和最终回复。停止参与和资源停止分开；PR 来源冲突显示待核实。已有历史 final 的完整性、采集缺口、读取错误仍保留。
- final 仅在用户查看或原执行结束而结果区按 Spec 默认展开时启用原查询，不新增第二份 final 查询。以最近已取得回复作展示优先项，始终保留“包含先前轮次”提示，未核实提示不被旧回复覆盖；无法确定回复顺序时保持接口顺序并展示历史列表，不猜“最新”或“本轮”。直接打开深链接、刷新页面或切换到已结束候选时，在取得该身份的首个有效终态快照后默认展开结果；同身份从运行进入结束且用户尚未主动选择过展开状态时也自动展开一次。用户主动收起后，同身份普通轮询不得重开或抢焦点；切换真正身份按新对象初始化，不能带入旧对象的展开选择或正文。
- 用量保留单行费用估算/Token 与原折叠明细；无价表、部分计价、旧估算和读取错误保留真实限定。不改估价、采集或查询身份，不额外查询配置目录。

依赖：P2。删除已无消费者的旧展示组件/样式/导入，不保留隐藏 DOM 或 CSS 仅遮住技术字段，不改仍承担业务判断和复制的字段/类型。

## 5. P4：精简其他页面与全局摘要

| 位置 | 修改及核对重点 |
| --- | --- |
| `tasks/dag.tsx`、`task-detail.tsx`、`dispatch.tsx` | 删除图/Task 版本与装饰说明，无依赖无阻塞时省略空区；保留截断/分页、实际依赖、派发/启动/方案选择、当前阻断和处理入口，正常就绪压缩 |
| `roadmaps/roadmaps.tsx`、`roadmaps.css` | 移除生成 JSON tab 及其仅用于展示的状态，保留源 Markdown、任务依赖审阅、所有文档种类与编辑/发布前后预览；来源哈希/固定提交退出展示，调查理由与重叠结论按需展开。保留历史标识、问题、来源变化、主动作、发布副作用与原请求查询/深链接 |
| `environment-checks/project-environment.tsx`、`environment-card.tsx`、`environment-overview.tsx` | 沿已有全部环境总览精简，复制替代独立检测 ID 展示；检测阶段、原因、计数若原数据提供则直显，保留所选方案/资格、停止和历史。不得删除重新检测会失效当前 ready 的提示 |
| `project-maintenance/project-maintenance.tsx` | 收拢正常摘要，准备/同步差异或未知原因靠前；保留检查更新、更新本地、提交日志、材料及原请求操作。`?maintenance=...` 与 `request_id` 仍打开原区，不因收起状态失效 |
| `activities/activities.tsx`、`activities/operation-result.tsx`、`app/operations.tsx` | 精简重复标签和技术字段，保留三筛选及未知计数、时间、进度、复制和精确定位；当前未知/拒绝/未发送错误在折叠历史外显示。复用原即时反馈/在途事实，不把 accepted/applied 一律换成成功 |
| `app/app.tsx`、`app-shell.tsx`、`styles.css` | 删除实例及实现页脚，保留一个连接标记、项目切换和活动；正常环境/维护紧凑，异常仍有短原因和入口。不得同时在顶部和各页再加一份同源长告警 |

依赖：P1；与 P2/P3 在文件级顺序整合，避免多次改写 `runs.tsx`/全局样式。保留既有设置与测试管理能力，长正文/路径只在自己的容器换行或滚动。配置页有效参数不适用执行页技术字段移除规则。

## 6. P5：行为测试、浏览器能力补齐与验证

### 6.1 测试选择和验收映射

正式选择四个有序唯一 ID：`l1.minimum`、`smoke.delivery`、`web.unit`、`runtime.async-activities`，见同名 JSON。完整命令、环境和集合断言引用同提交[正式清单](../testing/test-list.json)，不在此重写。L1/L2 是项目必跑；web.unit 覆盖真实交互与观察反例；活动整链验证改版没有破坏跨页观察、发起接线及清理。后端未改，不选 go.http/go.storage/go.environment/go.roadmap、全量 Full 或真实模型；准备查询/测试进度的上游状态持久化不因本次展示改版重新实现，相关 UI 反例由组件和专属浏览器覆盖。

| Spec 验收项 | 实施后测试安排 | 时点与覆盖 |
| --- | --- | --- |
| WUS-A01 | `web.unit` 的 runs、copy-execution-info、execution-info 测试；浏览器 B1/B4 | P2/P3 后；四区/技术字段退出、原复制内容及手动复制绑定 |
| WUS-A02 | `web.unit` 的候选状态及新增 feature 内纯显示映射测试；B1 | P2 后；当前/未知/回退、无拓扑、独立状态、无伪百分比 |
| WUS-A03 | `web.unit` 的 execution-wait、delivery-summary；B2 | P2 后；所有等待分支、计数和无额外操作，fake timers 控制可信/陈旧计时 |
| WUS-A04 | `web.unit` 的 last-message、runs；B1/B2 | P2 后；六行/五条边界、剩余记录、全文、空态、读取失败和同身份轮询保持展开 |
| WUS-A05 | `web.unit` 的 plan-tests、candidate-documents、审查展示；B1/B2 | P3 后；真实计数、重跑/非法/历史/过期、分轮与缺结果，不改写业务结论 |
| WUS-A06 | `web.unit` 的 delivery-summary、runs final；B3 | P3 后；各结果维度、缓存时间/错误、历史与缺口；直接打开/刷新/切换已结束候选及运行转结束均默认展开，主动收起后普通轮询不重开 |
| WUS-A07 | `web.unit` 的 answer、stop-candidate、resolve-pr-conflict、run-task/candidate-selection；B2/B3 | P2/P3 后；问题直显、原资格/草稿/信封/最后候选保护、停止参与不冒充资源停止 |
| WUS-A08 | `web.unit` 的 dag、task-detail、dispatch；B4 | P4 后；依赖、截断/分页、启动/方案、派发、文档与禁用原因 |
| WUS-A09 | `web.unit` 的 roadmaps/dependency-review；B4，加既有 roadmap-dependency-review 浏览器回归 | P4 后；当前/历史、全部文档、编辑与发布预览、原请求确认/发布/入图参数，不能仅检查按钮存在 |
| WUS-A10 | `web.unit` 的 project-environment/use-readiness/project-maintenance；B4，加既有 environment-overview 浏览器回归 | P4 后；全部环境、准备进度、维护就绪/阻塞、历史及精确深链接 |
| WUS-A11 | `web.unit` 的 activities/operation-result/operations/app/project-switch/feedback；B4；`runtime.async-activities` | P4 后及最终回归；原请求未知/拒绝直显、真实与未知计数、连接唯一、项目切换、发起/跨页/清理 |
| WUS-A12 | `web.unit` 的 run-usage；B1/B3 | P3 后；估算/部分/不可用/旧数据限定及明细可达 |
| WUS-A13 | `web.unit` 的 use-original-requests/use-candidate-starts/project-switch/query-visibility 及上述组件延迟响应测试；B5；`runtime.async-activities` | P2–P4 及最终回归；按原目标计数 GET/POST，轮询保持/身份切换、晚到响应及折叠零额外执行 |
| WUS-A14 | 专属生产构建/Playwright B1–B5；`web.unit` 补键盘及 ARIA 语义 | 全部页面完成后；375/800/1440px、导航收起、长正文与各状态截图、键盘/焦点/无页面溢出 |
| 必跑回归 | `l1.minimum`、`smoke.delivery`、`web.unit`、`runtime.async-activities` | 代码完成及提交前，四项按正式清单实际执行，非零/零 skip/完整终态及要求的清理通过 |
| WUS-M01 | 用户体验走查 | 自动验收通过后后置，参与人及窗口待安排，只判断主观体验，不代替自动失败 |

对受影响组件用可控 Promise、隔离 QueryClient、fake-indexeddb 及必要的 fake timers 断言行为和实际请求，不复制实现逻辑作测试。不机械新增每个样式或静态文案测试；同一行为通过最小反例覆盖，旧来源/身份/未知语义断言保留。Spec/Plan、测试 ID 清单及文档链接检查只证明文档一致性，不算上述产品验收通过。

### 6.2 专属浏览器补齐

新增 `web/tests/ui-simplification.spec.ts`，复用 `roadmap-dependency-review.spec.ts`、`environment-overview.spec.ts`、`execution-info.spec.ts` 已有生产资产服务和 `page.route` 方式；只在测试目录内按需复用有限夹具，不提取产品通用服务。运行前 `npm --prefix web run build`，之后显式执行：

```powershell
npm --prefix web run test:e2e -- ui-simplification.spec.ts roadmap-dependency-review.spec.ts environment-overview.spec.ts execution-info.spec.ts
```

这是现有 Playwright 入口的专属补充验收，不属于 web.unit，不新增分级 ID/长期测试流程；同名 JSON 仅列正式 ID，不塞入虚构浏览器 ID。若实际扩展分级场景目的/驱动或断言清单，须同步清单、可读列表、manifest 及受管证据校验，不能只改测试文本。

浏览器夹具预先覆盖 `/auth/status`、应用目录、项目连接/快照、业务列表/详情、final/活动/测试/用量等实际请求，保留原前端启动链。静态服务只绑定回环随机端口，未声明 API 请求使测试失败；静态 HTML 不能被当成成功 API。拦截受控命令时记录完整原信封，仅对测试明确允许的意图返回固定回执；未点击/折叠/刷新/状态切换零业务 POST。不接入安装服务、真实 Runner/模型或 GitHub，不将夹具成功说成真实业务完成。

| 场景 | 需要实际断言的状态与交互 |
| --- | --- |
| B1 运行与信息层级 | 运行候选的四区、已知/未知/回退阶段、独立待答标记、五条活动、六行消息及展开、真实测试/审查摘要、用量；普通与展开视图无独立技术字段 |
| B2 等待与异常 | queue/calling/backoff/exhausted/清除、源时间、陈旧与离线；待答/可稍后答/阻塞/拒绝/未知、消息/测试查询失败、材料过期；错误直显，旧内容保留，倒计时不产生执行 |
| B3 结束与交付 | PR-ready/合并待交付/真实交付、冲突观察、PR 缓存失败、资源待核实/停止；有旧 final 但本轮未核实、正文缺口、无 final、最新可确认回复与历史展开；直接深链接打开/刷新/切换到已结束候选均默认展开，运行转结束也展开，用户收起后同身份轮询不重开 |
| B4 跨页与审阅 | DAG/列表/依赖/分页边界、方案/派发/禁用原因；Roadmap 当前/历史、问题、依赖、Spec/Plan/tests/acceptance 正文及发布前后内容/目标；全部环境、维护深链接、活动三筛选/计数/原对象跳转/复制、项目切换与唯一连接 |
| B5 生命周期与布局 | 同身份轮询保留展开/焦点，不同候选/实例/项目迟到数据隔离；按相同时间窗口、可见性和查询启用/展开状态比较改版前后各 endpoint 的 GET/POST 次数，无重复请求或提高既有轮询频率。结束态默认展开所需的首次 final 读取是 Spec 要求的行为，应单列验证同源单次订阅，不能与旧页未打开 final 时的零请求粗略相减；三种宽度与收起导航、键盘展开/关闭、无抢焦点及 reduced motion |

B1–B4 的代表状态在三种宽度均取证；至少覆盖运行、长正文、等待、待答、失败、离线、历史与结束。使用 DOM 断言关键区域顺序、内容可见/可达、页面 scrollWidth 与视口关系、按钮边界及操作后的 URL/原请求目标，不以截图文件存在代替断言。trace、console/pageerror、请求计数、前后截图和服务关闭结果保存原始证据。finally 关闭本测试服务、浏览器及其连接，核验端口释放；不清理其他现场。

已有四个专属浏览器文件按生产文案/可访问名称的必要变化更新选择器，保留原业务断言；现有 `runtime.async-activities` 的真实受控观察与发起阶段都要运行，不能让拦截 API 的新场景替代该整链。若无法保持原断言，登记明确缺口并修正规划，不能通过无文件过滤执行全部含 live 前置的 tests 目录。

### 6.3 执行顺序、环境与证据

1. P1–P4 每块完成后运行相应 Vitest 文件做定向诊断；完整变更后运行正式 web.unit、生产构建及专属浏览器，再运行 L1/L2 和 runtime.async-activities；构建与正式场景按共享目录要求串行。最后一次代码修改后补跑受影响验证，最终提交前固定回归完整通过。
2. 环境为 Windows、Go/Git、Node 24+、npm 依赖与 Edge，按正式清单核实。`npm --prefix web ci`、Web 构建及包生成不得在同一源码中并发。无需真实模型登录或真实外部 PR 权限。
3. 证据 scope 使用 `C:\SteerlineDebug\wt-c06b-web-ui-simplification`，每次调用按 paths.md 产生独立 `<test-run-id>` 叶目录。正式入口显式 `-out`，浏览器设置 `STEERLINE_WEB_EVIDENCE` 并在 finally 恢复旧值，构建/定向检查存 diagnostics。实际命令、受测 SHA/脏状态、生产资产来源/摘要、退出码、原生报告、截图/trace/请求记录及清理结果逐项登记专属验收。
4. 失败保留原证据，修复后另开目录，不自动重试掩盖失败；skip、零匹配、缺清理、缺前置或无法复核的截图不算通过。普通文档检查不创建调试现场、不运行业务测试，也不写上述外部证据根。

## 7. 本轮文档交付与后续实现门槛

本轮完成 Spec/Plan、同名测试 JSON、专属验收规划及索引，核对 WUS-A01–A14/WUS-M01 全覆盖，检查所有正式 ID 存在、有序唯一，执行链接与 diff 检查。独立审计只读检查这些材料与现有实现的可行性、边界、状态语义及测试覆盖；主聊天修正确认问题，再请审计者复核修订。具体问题、处置和复核结论登记专属验收，不能据文档审计声称 UI 已通过产品验收。

后续实现按 Spec 提供前后页面对照和被删除/整合内容清单，不通过缩小字体、隐藏全部内容或搬走进度完成任务。没有提示词/黄金样本/相关 CI 改动；一旦确需越过生产接口或提示词边界，先修订规格和必要验证/审核安排，不将其当作本 Plan 的隐含步骤。
