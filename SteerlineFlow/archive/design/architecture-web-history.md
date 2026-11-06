# 架构与 Web 历史设计入口

归档于 2026-11-06。本文保留历轮架构审视、旧任务交接和 Web 首期技术方案的来源，内容按原版本理解，不构成当前实施或验收结论。当前职责见[架构总览](../../../docs/architecture.md)，模块及客户端边界见[代码架构](../../../docs/architecture/current-code-architecture.md)，工程使用见[Web README](../../../web/README.md)。

## 原文与历史证据

整理前已提交原文可在本仓使用 `git show 5b8e08bc53e3789b2fe0ab736260a26eaf7be1c7:<路径>` 读取；路径如下：

- `docs/architecture.md`。
- `docs/architecture/current-code-architecture.md`。
- `docs/architecture/parallel-development.md`。
- `docs/product-design/web-frontend.md`。
- `docs/product-design/planning-architecture.md`。
- `web/README.md`。

首次方案和各轮审计的通过、失败及未验范围仍按各自固定版本追溯。

| 记录 |
| --- |
| [验收流程与记录](../../testing/acceptance/README.md) |
| [应用与项目生命周期收拢验收](../../testing/acceptance/application-project-lifecycle.md) |
| [调用方边界收口验收](../../testing/acceptance/architecture-boundary-consolidation.md) |
| [PR #350 四轮解耦复审](../../testing/acceptance/architecture-decoupling-pr350-review.md) |
| [当前架构与解耦审视记录](../../testing/acceptance/architecture-decoupling-review.md) |
| [架构解耦第一轮验收](../../testing/acceptance/architecture-decoupling-round1.md) |
| [架构解耦第二轮验收：历史与候选读取](../../testing/acceptance/architecture-decoupling-round2.md) |
| [架构解耦第三轮验收：会话与活动读取](../../testing/acceptance/architecture-decoupling-round3.md) |
| [架构解耦第四轮验收：Task 交付类型归位](../../testing/acceptance/architecture-decoupling-round4.md) |
| [架构解耦第五轮验收：宿主 checkpoint 与状态契约](../../testing/acceptance/architecture-decoupling-round5.md) |
| [#39 本机多候选验收摘要](../../testing/acceptance/archive/completed/candidate-acceptance.md) |
| [配置入口与导航统一](../../testing/acceptance/configuration-navigation.md) |
| [执行域遗留清理验收](../../testing/acceptance/execution-legacy-cleanup.md) |
| [原生执行配置边界收口：验收规划](../../testing/acceptance/native-execution-configuration-boundary.md) |
| [产品读取、命令入口与 Web 容器重构验收](../../testing/acceptance/product-read-command-web-refactor.md) |
| [Roadmap 读取、解析与转正验收计划](../../testing/acceptance/roadmap-promotion.md) |
| [SteerLine 与 SteerRunner 职责边界审计及验收](../../testing/acceptance/runner-boundary-consolidation.md) |
| [SteerLine / SteerRunner 协作与 Workflow 接入验收](../../testing/acceptance/runner-collaboration.md) |
| [旧执行记录与空壳字段清理验收](../../testing/acceptance/runtime-shell-cleanup.md) |
| [应用级共享 SteerRunner 服务验收](../../testing/acceptance/shared-runner-service.md) |
| [SteerRunner 接入验收](../../testing/acceptance/steerrunner-cutover.md) |
| [SteerRunner 拆分删除审计](../../testing/acceptance/steerrunner-deletion-audit.md) |
| [TUI 退役与 Web 唯一交互前端验收](../../testing/acceptance/tui-retirement.md) |
| [Web 候选即时反馈与连续创建验收](../../testing/acceptance/web-candidate-start.md) |
| [Web 候选停止参与修复验收](../../testing/acceptance/web-candidate-stop.md) |
| [Web 执行配置管理与能力探测验收](../../testing/acceptance/web-execution-config.md) |
| [本机 Web 前端验收](../../testing/acceptance/web-frontend.md) |
| [独立 Web 程序与项目切换验收](../../testing/acceptance/web-project-switch.md) |
| [验收：SSE 前的 Web 查询与缓存优化](../../testing/acceptance/web-query-optimization.md) |
| [任务/执行/控制台独立 owner 探针的固定历史记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/three-line-10-independence-cleanup/independence.md) |
| [控制台交互用例](https://github.com/erranliu/SteerLine/blob/47fafc12721ceb8fa39ba1ab227872fd14513bcb/internal/console/interaction_test.go) |
| [执行恢复隔离用例](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/candidate_resume_isolation_test.go) |
| [真实 app/client 接线](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/tests/integration/candidate_console_test.go) |

## 后续解耦的历史建议

以下按整理前记录保留，完成标记仅表示当时范围；尚未实施部分须重新核对源码后单独规划。

### 原建议及当时状态

当前外部执行边界已经成立，适合继续做少量内部依赖收口。优先收益是减少跨功能修改和编译牵连，而不是增加服务、包或接口数量。下表保留原建议顺序并标注本轮完成范围，不是已纳入 execution-map 的任务或实施 Plan。

| 顺序 | 建议 | 收益、范围与完成判断 |
| --- | --- | --- |
| 1（本轮完成） | 共享路由白名单归纯 HTTP 契约层 | `httpcontract.AllowedProjectRead` 为后端与 Web 的唯一共享来源；保留后端专用 Task 写入/回执路由。`web/server` 与 Web 命令已纳入消费端守卫，Windows/Linux 生产图及全标签源码检查阻止后端/SQL 依赖回流。 |
| 2（三轮范围已完成） | 收口 SQLite 与产品投影/DTO 的依赖 | 六项旧例外和剩余会话/活动 DTO import 已移除，SQLite/存储端口均无产品包生产依赖。查询层负责公开字段、游标/协议和序列化字节预算；存储继续提供原身份、顺序、正文和同快照事实。活动使用有界只读记录流，避免为投影预算预读整页。 |
| 3（Task 与宿主状态归位已完成） | 把仍使用的类型按真实所有者归位 | Task 交付/项目权威与宿主 checkpoint/服务状态/控制回执/进程观察契约均已归位；旧定义无别名转发，原子交付、同快照和关闭语义已回归。`StartupBlockers`、候选/活动展示与控制传输接口仍可按实际消费者继续收窄，避免创建新的通用大 model 包。 |
| 4 | 在必要时拆开 app 中的组装和产品适配 | 先稳定配置与资源契约，再把 `clientReader`/命令适配等实现移出组装根；`ProjectRuntime` 保留项目本地资源的唯一所有权。依赖多本身不是问题，若日常改查询/命令频繁触及同一包，才值得做这一轮。 |
| 5 | 按实际需求收窄 Runner 调用面与 Web context | `runnerbridge.CLI.Call` 仍接收 CLI 命令串，接口方法也使用 `runnerclient` 的协议别名；需要替换传输或频繁改协议时，可改用 `runnercontract` 值和语义方法。Web 的项目/连接/原请求 hook 已分责，暂可保留现有 context，出现独立功能并行冲突后再拆。 |

第一轮已完成第 1 项及候选错误映射；第二、三轮已完成第 2 项；第四、五轮已完成第 3 项中的 Task 与宿主契约归位。依赖规则由全标签源码与 Windows/Linux 依赖图核对。余下可继续收窄展示/控制传输值及 app 中的组装与产品适配；这些尚未实施。

暂不建议把 Task、候选、交付拆成独立服务/数据库，也不建议引入通用事件总线、插件注册框架或重建 workflow 层。当前一个原子交付事务、每项目资源 owner、Runner 单写执行事实的关系是必要的一致性边界。业务规则、生命周期和真实支持范围分别归[整体架构](../../../docs/architecture.md)、[Worker 生命周期](../../../docs/product-design/worker-lifecycle.md)及[接入验收](../../testing/acceptance/steerrunner-cutover.md)维护。

Runner 的 Orca 协议能力层共用本机/远端目录读取，通过 native-catalog 返回目标注册 Agent 及模型/provider/effort 事实。Web 每次查询重新调用固定 EXE，仅合并同上下文在途请求，失败时保留明确过期结果。配置owner独立发布可生成组合并验证完整schema；前端合并展示。模型provider凭据留在原Agent，Web不建key配置、provider发现或账号数据库；Go执行服务仍仅消费execution.json。

## 旧任务共享边界与交接

以下是原交接记录，不作为当前任务安排、依赖或格式依据。

## HTTP 接入后的草稿边界

以下仅为当前[草稿](../../Roadmap/drafts/roadmap.md)导航，真实职责/契约与通过条件由各自Spec/Plan固定，不改变源码当前owner或自动生成依赖。

| 草稿 | 拟议收敛及独立修改面 | 共享条件 |
| --- | --- | --- |
| #92 [HTTP命令](../../spec/http-command-boundary.spec.md) | httpapi消费端口、同包信封登记/对账、app的HTTPendpoint段 | #93共同wire/API/httpfs只读；普通与原生查看保留各自重放规则 |
| #93 [客户端原请求/观察](../../spec/http-client-request-lifecycle.spec.md) | 原 TUI journal/observer 已退役；共享 HTTP Client/Open 与进程身份保留 | 新客户端能力按当前 Web/application 边界维护；原交付事实保留 |
| #94 [固定流程规则](../../spec/development-flow-rules.spec.md) | 保留原development纯包方案供重审 | 暂不纳入；原先与三层 Workflow 规则重叠，须按 Runner 迁出后的现状重审，不同时建立两套owner |
| #95 [最终交付](../../spec/task-delivery-usecase.spec.md) | 保留delivery窄端口用例的剩余收益 | 暂不纳入；原方案涉及任务/PR业务桥接；迁出后按当前 actor 与记录接线重核 |
| #96 [运行时资源](../../spec/worker-runtime-ownership.spec.md) | 保留原资源/观察/在途owner及确认交接反例 | 暂不纳入；与三层ReliableExecution和actor资源组装交叉，不按旧runtime边界并行开工 |

已确认的 #92/#93 均已交付。Worker 三层重构随后合入，执行又迁至独立 Runner；#94/#95/#96 仍留草稿，原范围和共享边界须按当前源码重审，不自动转正。新的契约、验证及资源安排由后续确认的 Spec/Plan 和验收固定。

#92/#93 已交付；原入口接线和定向组合的历史结果见各自验收。#94/#95/#96的旧未知发布/阶段/清理组合只保留为重审材料，不安排执行、不用三层验收自动补记通过。


## #84 / #45 历史交接

用户于 2026-10-09 明确允许 #84 与 #45 独立并行实现；共享状态、格式、流程语义由 #84 交付，主干集成先 #84 后 #45，#45 的 HTTP/client discard 接线仍由原执行者负责。#84 提供 WorkflowRun `status/flow_ref/step_id`、输入/结果版本绑定，产品 CandidateSummary 的同快照步骤、源执行事实及报告权限边界；SQLite schema 18、runtime 20、worker-flow-v2、Orca execution-state codec 11。#45 后续消费真实字段和结果，不复制转移逻辑；该交接已交付，#44 后续删除由 #91 规划。完整状态按 [execution-map](../../Roadmap/execution-map.md)核对。

## Web 首期技术方案

以下保留原选型、首批实施安排和当时资料链接；其中拟议语气、旧路径及前置按历史解释。当前依赖以工程清单及 lockfile 为准，运行与开发方法见 Web README。

## 已确认技术栈

| 职责 | 选择 | 本项目用途 |
| --- | --- | --- |
| 页面与类型 | React + TypeScript strict | 将原型拆为可复用组件和按功能组织的页面，类型化精确目标与命令信封 |
| 构建与开发 | Vite + npm | 开发热更新，生产生成静态资源；提交 package-lock.json，CI 使用 npm ci |
| 路由 | React Router，Declarative 模式 | 一级导航、任务/候选定位与深链接；数据读取统一由 Query 管理 |
| 样式与控件 | Tailwind CSS + shadcn/ui | 保留原型色彩、密度和布局，复用对话框、表单、菜单等基础控件；控件源码由仓库维护 |
| 服务数据 | TanStack Query + fetch | 查询、分页、定向失效和轮询；业务事实由 Go 决定 |
| 界面状态 | React state / reducer / context | 当前选择、对话框和输入草稿；首期不增加全局状态库 |
| DAG 图 | React Flow | 定制 Task 节点、依赖边、缩放与选择；自动布局在图页接入时按实际需求选择 |
| 本机持久草稿与请求 | IndexedDB | 保存按原身份绑定的未发送输入与完整原命令信封，保留回执未知时的对账依据 |
| 最小验证工具 | Vitest + Testing Library；Playwright | 关键状态/目标绑定组件测试，以及本机真实浏览器定向接线场景 |

工程位于 `web/`，按 `app/`、`shared/ui/`、`shared/api/`、`features/` 组织。浏览器服务归 `web/server`；DTO/请求编码在 API 层维护，页面消费 Go 实际 JSON 字段、分页和错误码。

本产品以本机交互和已有 Go 服务为中心，采用客户端 SPA 即可。运行时只需要 Go 服务和浏览器，Node.js 用于开发与构建。首期不需要 SSR、Next.js 或常驻 Node 后端；生产静态产物建议嵌入 Go 发布包，接入现有发布/调试生成流程。

具体依赖版本在工程初始化时核对兼容性并由 lockfile 固定，本次不凭技术讨论预填版本号。


## 已确认首批范围与后续顺序

1. 工程与浏览器接入：构建/静态发布、稳定本机入口、会话认证、身份查询、离线状态、请求持久化与原 ID 对账；搭好原型的导航和布局基础。
2. 已有能力闭环：任务/DAG 与详情、候选切换、近期活动/final、回答问题、现有启动/候选动作、原生会话查看和 PR/交付展示。动作与选项按当时正式 API 能力接入，参数设置不伪造原型中的动态发现。必要能力缺口在实施前明确补齐范围。
3. 后续产品页：规划 Agent 与草稿转正、验收/测试管理、体验版本、Bug 自动修复及文档维护等，按各自真实后端契约逐项推进。缺能力不使用演示数据冒充真实功能。

页面骨架与客户端代码可以先推进。已有 #92/#93 是原接口解耦任务，不自动成为 Web 工程前置；浏览器接入涉及 httpapi/应用接线时，先核对它们及 Worker 三层重构的实际在途修改，按[并行边界](../../../docs/architecture/parallel-development.md)保留双方成果。本建议不是正式执行 DAG，也不启动其他聊天或工作区。

正式实施的 Spec/Plan 应链接任务专属 acceptance，并按最小必要范围选择验证：浏览器认证/非法来源拒绝、固定身份与晚到响应、命令未知结果对账/刷新恢复、中文输入和一个已有任务执行闭环。纯前端组件检查可使用隔离夹具；真实服务调试先运行 `scripts/new-debug-release.ps1` 创建 `C:\SteerlineDebug` 的独立环境，AI 再使用该包的 `start.bat --simulate` 显式选择模拟方案，真实 PR 只用本次 SteerlineTest 专属分支。启动与冻结规则见[调试说明](../../../docs/runtime-install-debug.md)。分级入口按[现有测试说明](../../../tests/e2e/README.md)登记，不以一次前端构建代替业务接线验证，也不默认跑 full 或全平台矩阵。

## 技术依据

下列官方资料核对于 2026-10-14，选择理由为本项目判断：

- [React 的独立应用构建与 Vite/TypeScript 示例](https://react.dev/learn/build-a-react-app-from-scratch)，[Vite 后端集成](https://vite.dev/guide/backend-integration)。
- [React Router Declarative 安装](https://reactrouter.com/start/declarative/installation)，[Tailwind 的 Vite 接入](https://tailwindcss.com/docs/installation/using-vite)，[shadcn/ui 的 Vite 接入](https://ui.shadcn.com/docs/installation/vite)。
- [TanStack Query 默认行为](https://tanstack.com/query/latest/docs/framework/react/guides/important-defaults)，[查询重试配置](https://tanstack.com/query/latest/docs/framework/react/guides/query-retries)。
- [React Flow 定制节点](https://reactflow.dev/learn/customization/custom-nodes)，[布局选择](https://reactflow.dev/learn/layouting/layouting)。
- [Vitest](https://vitest.dev/guide/)，[Playwright](https://playwright.dev/docs/intro)。
