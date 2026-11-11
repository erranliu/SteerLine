# 异步操作统一反馈与活动中心衔接 Plan

状态：2026-11-09，审计修订后按用户授权实施。依据 [Spec](../spec/async-operation-feedback.spec.md)，原安排保留，实际结果及未验边界只登记于[专属验收](../testing/acceptance/async-operation-feedback.md)。普通开发不分配 Task ID，不启动 roadmap-runner，不改 execution-map，不部署。

测试选择固定于同名 [async-operation-feedback.tests.json](async-operation-feedback.tests.json)，定义以同提交[正式清单](../testing/test-list.json)为准。流程依照[文档规范](../../SteerlineProjectGuideDoc/spec-plan-guidelines.md)、[测试指南](../testing.md)、[验收流程](../testing/acceptance/README.md)和[路径清单](../paths.md)。异常与恢复复用 [Runner 调用生命周期](../../docs/product-design/runner-lifecycle.md)，没有新增业务重试器。

## 1. 实施范围与约束

首批动作与阻塞粒度以 Spec 第 2、4 节为准：Roadmap 全流程（长操作进入活动中心，确认/编辑/初始化/查询/停止原位反馈）、environment_start、手动 start_task/start_candidate、maintenance_check。其他环境/候选控制、回答、冲突处理、配置保存、maintenance_update 不接入公共反馈；只保留与首批操作已有的互斥关系。

实现主要限于 Web 交互、原请求观察/查询失效及现有受控验收驱动；不修改 Go 生产业务 owner、SQLite schema、HTTP/Runner 契约和提示词。已有活动来源与请求持久关联足以表达本次目标，不能新增通用活动写服务。实现及测试定义的实际改动登记于专属验收。

审计后的最小结构：原请求 hook 记录在途信封；业务入口用这些事实判断自身禁用和交接；活动模块只接收展示 props 和动效通知。禁止 feedback.run/execute、全局 acquire/release、冲突策略注册、第二份提交阶段表及动画队列。精确跨页活动定位和完整 Roadmap 受控成功整链不在本批新增。

## 2. P1：固定身份、冲突范围及状态交接

再次核对 `app/use-original-requests.ts`、`app/use-candidate-starts.ts`、`shared/api/client.ts`、`shared/api/journal.ts`、`features/activities/model.ts`、`activities.tsx`、各业务页面及 `shared/api/queries.ts`。以实际实施提交更新验收基线；不得沿用调查提交冒充受测版本。

- 列出首批入口到动作、原请求/LocalStart、source/environment/task/candidate、原有详情及查询键的映射；同一草稿流程的不同 record/子记录在 Roadmap 业务代码内用真实 source 身份关联，不建设公共冲突注册表。
- 明确各适配器的提交返回和读取事实：保留 Receipt、未发送证明及未知原因；逐步让首批入口消费完整回执，避免公共状态建立在 `send(): boolean` 上。保持既有 Api.submit 内 saveRequest/post、原请求 reconcile 和候选排序的调用顺序及次数。
- 为“提交结束、事实未刷新”“已 applied、活动未关联”“读取失败、原请求未知”固定业务入口的衔接条件及原因文本。发送阶段复用原请求 hook 的在途记录，未知/后续交接从 journal/业务事实派生，不由动效 Provider 或卸载页面 finally 解锁。活动关联成功不作为业务可操作的条件。
- 为候选区分一次确认意图与 task 资格串行：同次确认去重，新的明确并行候选有独立 ID。原 hook 创建 LocalStart/信封，公共层不能抢先生成第二个请求。
- 环境检测的公开信封仍以项目为 target；按 Spec 的环境粒度细化既有 Api.submit 发送锁及待核实检查，使用原 payload.environment_key 区分实际对象，同环境不同 profile 不能绕过保护；身份不明时保守阻断。原信封、journal、POST/核实顺序保持，不增加锁服务或新协调层。

交付检查：可逐项审计 Spec 动作覆盖和身份映射；后续 P2/P4 的延迟 Promise 测试按这些条件验收，不另写一套后端资格状态机。

## 3. P2：活动模块内建立独立 feedback 能力

在现有 `app/use-original-requests.ts` 暴露原信封/原回执的只读选择，优先复用现有 mutation 记录，避免新增并行状态表；同步防重需要的最小 ref 仍留原 owner，不复制回执。首次异步等待前在途事实可见，结束前原回执已进入现有 mutation/journal 缓存，不能产生等下一轮 journal 轮询的空隙。缓存读取失败也保留已取得回执，不退成“未提交”。候选复用原 LocalStart。业务层判断自身冲突，通用请求 hook 不解释 Roadmap source 或环境资格。

活动模块的展示代码可放 `web/src/features/activities/feedback/`；不预设需要几层 hook 或几个文件：

| 内容 | 职责与依赖 |
| --- | --- |
| 纯展示组件 | 接收 label/busy/reason 等业务层派生 props，统一状态和 aria-busy/status；不保存回执或提交阶段 |
| 动效 host / 必要的轻量 Context | 只接收已被入口接受的视觉通知及入口引用；最多一项短期动画，不调用原提交、不排队、不保存已播历史 |
| 原 activities.model 合并函数 | 在现有占位逻辑中只读合并原在途信封、journal、LocalStart 与正式关联，按原 ID 去重；关联查询的 request_ids 同时覆盖这些实际请求，不复制到 feedback 子模块 |

由 `app/app.tsx` 的 Workspace 组装展示数据及动效 host。只有动效需要跨路由挂载；发送、journal 与候选状态本来就由现有 Workspace/hooks 承接，不挪到 Provider。scope 改变清除动效，原请求沿原 Api 生命周期处理，UI 不另外 abort 业务请求。

只在确有重复时共享现有 action 名称/展示标题，不为所有动作新建描述器体系。业务入口直接调用原 submit/start，再发一次非阻断的视觉通知；不经过 feedback 执行包装器。动效抛错不能跳过原提交。新 feedback 子模块不导入 Api、Workspace、journal 或业务 hooks；现有 Activities 只读消费 Workspace 的路径不要求重构。

在途记录、原 journal/LocalStart、正式项只在现有 activities.model 中合并。Provider 不复制这些记录，也不做“提交中→等待中→成功”的状态机。发布/入图/resume 的关联已由 SQLite roadmap_requests 与只读投影提供，直接复用；不改后端、不新增顶层活动。

## 4. P3：动画、静态反馈与活动查询衔接

1. 活动入口注册目标；用户操作提供激活按钮的几何快照，动画层在壳中承载。采用平台/CSS 已有能力，不为单个动画引入新依赖。一次意图 300–450ms，不等待 animationend 再提交或解锁。
2. 减少动态效果、窄屏空间不足、目标不可见、隐藏页面或动画失败均保留文字反馈；导航/视口改变可结束位移。重叠事件可只响应入口高亮，不新增动画调度/排队能力。
3. 活动列表、关联及业务查询使用各自 owner 的 query key/matcher。扩充命令结果变化失效范围，覆盖活动相关键和首批业务键，处理 Roadmap 目前不同的排列；不做无范围 invalidateQueries。
4. 点击时业务原在途记录立即更新并驱动按钮/占位；journal 出现时去重，提交结果和原请求核实变化时及时刷新。隐藏页面只标记失效，沿原可见性策略读取。活动新关联不重置分页/筛选或把查询失败当空列表。
5. “查看活动”仅主动打开现有抽屉、保留筛选；本次结果走原有详情/原请求入口，不加跨分页定位服务。活动更新不展开、不跳筛选、不抢焦点，状态播报去重。

## 5. P4：分入口接入并删除重复反馈

| 顺序 | 入口及改动 | 保留的业务边界 |
| --- | --- | --- |
| P4a | Roadmaps.action 读取原请求在途/journal；在本模块派生 source 范围的推进限制与回执交接，按钮共用展示组件并通知动效，收拢多段刷新 | source/record/parent、静态材料、历史只读、审阅摘要、revision、原发布/入图请求及停止规则；查询/停止不得跟推进入口共用总锁 |
| P4b | EnvironmentWorkspace.command 接入；以 environment_key 处理 profile 别名，减少重复 pending/submitting 状态 | 环境选择记忆、ready/停止/失效资格、原请求核实和原环境查询 |
| P4c | TaskDetail 防止同一确认面板的重复激活；原 start 返回 LocalStart ID 后从组件通知动效并导航，目标页继续读原 items | 原 LocalStart ID、首次 claim 排序、合法并行候选、原请求/journal 及未发送证明；候选 hook 不导入反馈模块或 DOM，也不发送 UI 事件 |
| P4d | ProjectMaintenance 的 maintenance_check 复用现有 Workspace submit 观察原回执/在途状态并通知动效，维护查询定向刷新 | 本次检查与后台扫描分开；maintenance_update 保留原执行路径，不因组件公共 action 函数重构而改变其回执/更新语义；已有互斥只在本业务内维护 |

首批入口移除无关全局 busy 禁用，改读原在途信封及原业务原因；未接入入口不整体改造。保留有业务用途的 pending/版本保护，只有原 owner 已有等价事实时才删除重复字段。任何提交限制必须能指出原请求/业务事实依据，不接受反馈 Provider 的私有状态作为依据。

## 6. P5：自动验收与最小测试选择

本节安排实施后的验证；具体反例步骤及实时状态见专属验收。定向 Vitest 使用可控 Promise、必要的 fake timers、隔离 QueryClient 和现有 fake-indexeddb，断言行为及真实提交次数，不写镜像实现的测试或依赖机器快慢的毫秒阈值。构建使用 `npm --prefix web run build`，验证 TypeScript 及生产资产。

| 验收项 | 测试 ID / 所属文件与原因 | 时点及完成要求 |
| --- | --- | --- |
| AOF-A01 / AOF-A02 / AOF-A03 | `web.unit`；新增 feedback 行为测试，扩展 roadmaps、project-environment、task-detail、candidate-starts、project-maintenance 现有组件测试 | P2/P4 各步后定向检查；延迟发送/延迟刷新、连击、多入口、未知/拒绝、别名及独立并行均通过，确认每意图一次提交 |
| AOF-A04 | `web.unit`；动画/状态与活动组件测试，真实几何/交互补入下述浏览器阶段 | P3 后；不支持动画、缺目标、减少动态效果、快速完成、并发和导航不影响发送/解锁；静态反馈可达 |
| AOF-A05 / AOF-A06 | `web.unit`；activities、queries、use-original-requests、project-switch 和候选 hook 回归 | P3/P4 后；journal/正式项合并、跨筛选关联、更新不丢分页、scope 失效准确、迟到回调隔离通过 |
| AOF-A07 | `web.unit`、代码依赖自查、现有 `runtime.async-activities` 的纯观察阶段 | 全入口完成后；公共展示不调用提交回调、不保存回执、不判断冲突；活动关联失败时已具资格的查询/停止等仍可用，原请求与准入反例保持 |
| AOF-A08 | `web.unit` 覆盖所有首批动作/成功及故障反馈；`runtime.async-activities`（L3 controlled）验证真实浏览器动效与发起接线 | 组件和生产构建通过后；真实四类发起与模拟成功覆盖分别登记，见下节，清理核实 |
| AOF-A03 / AOF-A07 回归 | `web.unit` 的 project-maintenance 组件反例及 `runtime.async-activities` 的检查更新发起 | P4d 后；确认检查与本地更新互斥仍有效，maintenance_update 原调用/回执路径不变；不额外跑整套材料版本演进来证明一个提交反馈 |
| 必跑回归 | `l1.minimum`、`smoke.delivery`、`web.unit` | 代码完成及提交 PR 前按固定清单执行；L1/L2 不缩小，前端全集实际非零通过 |
| 文档与安排 | 链接检查、测试 ID 存在/有序唯一核对、`git diff --check` | 本轮文档及最终交付检查，不替代 AOF 产品验收 |

选定四个正式 ID：l1.minimum、smoke.delivery、web.unit、runtime.async-activities，完整命令/环境/断言引用正式清单。本任务没有 Go 生产 owner 或维护更新逻辑改动，不默认增加 go.full、领域 owner 全集或 runtime.project-maintenance，也不要求真实模型/真实 PR。若实际要改 maintenance_update/项目准入或任何生产 owner，先停下说明原因并更新范围、测试选择及验收，不能静默扩张实现。

### 6.1 受控浏览器能力补齐

复用 `tests/e2e/internal/testenv/async_activities.go` 和 `web/tests/async-activities.spec.ts`，保持原纯观察阶段；追加独立的用户发起阶段，必要时拆一个被同场景显式执行的浏览器文件，避免不带文件过滤启动不适用场景。

- 原观察阶段继续比较调用快照，打开/关闭/查询活动零新增执行输入；不得删掉该断言以容纳新交互。
- 两个独立项目的准备和验证共用 runtime.async-activities 场景；按现有重型场景约定给该测试 10 分钟正文、12 分钟包预算，清理沿原独立上下文。原 5 分钟取消的证据保留，不修改生产 owner 的任何期限/超时，也不延长浏览器各断言来掩盖失败。
- Roadmap 全部入口与生成/发布/入图成功、拒绝及未知反馈放在现有 Roadmaps 组件测试中，用明确标注的固定 API 回执/查询夹具验证 UI，不冒充真实生成/发布结果。
- 当前 `tests/e2e/internal/controlled/runner.go` 明确拒绝 generate 工作流，不为此 UI 任务扩建生成结果、来源清单和发布合并模拟器。真实浏览器 Roadmap 只需准备合法静态材料/方案后发起解析，核验单次命令、即时反馈及受控拒绝/未知结果；不要求在该现场走到文档发布或入图成功。
- 同场景从浏览器显式发起环境检测、首次手动候选及独立并行候选、检查更新，复用已有能力，核验同一激活只有一次原命令；未知只核实原 ID。正常环境/候选与 Roadmap 拒绝/未知分支使用独立工作，避免相互污染。
- 原观察项目保留单候选交付/合并/停止完整证明；反馈项目必须观察到两个原候选及 Run，不复用限定单候选的 WaitPhase。反馈项目停服务后，沿场景已有独立上下文清理原 Runner，并核对 cleanup_complete 和原 Runner 清理凭据；不扩建并行交付模拟器。
- 截取每类动作的按钮即时状态、活动入口/目标状态及原请求/正式活动身份；至少在真实页面证明提交中局部锁、独立对象可操作和请求返回后的交接无短暂放行。浏览器不得伪造业务成功 JSON；不可控边界用组件延迟 Promise 测试补齐。
- 浏览器取证正常几何动画、320px/200% 等效布局、键盘、reduce motion、快速检查完成及候选导航。home/项目/实例迟到响应、同渲染前连击与组合故障用既有组件测试，不为这些反例重建双项目/重启系统验收平台。
- “无新增横向滚动”对照同一视口操作前后的页面宽度，并要求新增反馈控件边界落在视口内。原全局侧栏若已有溢出，单独记录，不能声称整页已适配，也不因本轮反馈验证扩改全局导航；原活动抽屉的窄屏断言保留。
- 扩展场景定义时同步正式 test-list.json、可读 L3/L4 列表、tests/e2e/suites.json 及受管证据校验，保持原 runtime.async-activities ID 的观察目的和报告真实性。仅新增前端测试不意味着已有场景已覆盖四类发起。

### 6.2 执行环境与证据

实施时使用 `C:\SteerlineDebug\wt-d332-async-feedback` 独占 scope，按 paths.md 给每次执行生成新目录；用户授权实施后的实际运行见专属验收。Windows、Go/Git、Node 24+/npm/Edge 依赖按清单核实。Web 安装/构建及包生成按共享输出约束串行；不读取或修改真实安装数据库/服务。

每项实际执行记录受测 SHA/脏状态、冻结应用/Web/公开 Runner fixture 身份、原命令、退出码、非零匹配及清理证据；失败与补验保留各自目录。所有 AOF-A 项需要实际证据，后置 AOF-M01 只用于动画可理解性判断，不接收自动失败。

## 7. 本轮文档交付检查

完成 Spec、Plan、同名测试选择、专属验收规划及索引；自查范围/身份/未知状态/并行候选/动画降级与测试映射一致。执行文档链接、JSON 清单 ID 和 diff 检查，将实际结果登记专属验收的文档检查区。文档检查不代替实施自动验收，不沿用旧活动中心或其他任务的通过结论；本次实现与最终证据见专属验收，不提交/推送/部署。
