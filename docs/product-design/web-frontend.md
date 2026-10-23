# Web 前端技术方案

2026-10-14：用户已确认首期在本机浏览器使用，同时确认采用下列技术栈，首批交付本机浏览器接入、DAG 与执行页，并直接授权本聊天实现。本文最初按 Go 服务提供页面设计；后续实现调整为独立 Web 程序，由 Go 应用宿主授权并转接服务。当前启动与进程边界见[统一启动 Spec](../../SteerlineFlow/spec/unified-startup.spec.md)。工程位于独立的 [`web/`](../../web/README.md)，具体契约归 [Spec](../../SteerlineFlow/spec/web-frontend.spec.md)/[Plan](../../SteerlineFlow/plans/web-frontend.plan.md)，实际验证归[专属验收](../../SteerlineFlow/testing/acceptance/web-frontend.md)。本文不创建 roadmap 任务，也不改变已有任务的范围或状态。

2026-10-15：独立 Web 程序与已有项目切换由本次直接授权实施，当前进程与协议归[项目切换 Spec](../../SteerlineFlow/spec/web-project-switch.spec.md)，实际结果归[专属验收](../../SteerlineFlow/testing/acceptance/web-project-switch.md)。Web 独立运行，通过 Go 应用服务取得当前 home 的配置目录并转接项目服务。后续[项目管理方案](project-management.md)中的准备、结构核验、纳管与建库仍未实现；TUI 已退役。

分批规格已独立：[Web exe 与已有项目切换 Spec](../../SteerlineFlow/spec/web-project-switch.spec.md)/[Plan](../../SteerlineFlow/plans/web-project-switch.plan.md)先行，仅使用现有配置；[项目添加 Spec](../../SteerlineFlow/spec/project-onboarding.spec.md)/[Plan](../../SteerlineFlow/plans/project-onboarding.plan.md)后置，不成为切换功能的前置。

2026-10-15 查询体验规划：用户明确后续引入 SSE；此前先统一 Query 缓存与参数、有界预取基础数据、复用缓存展示、停止隐藏区域查询及操作后按资源族失效。见[查询优化 Spec](../../SteerlineFlow/spec/web-query-optimization.spec.md)/[Plan](../../SteerlineFlow/plans/web-query-optimization.plan.md)与[专属验收](../../SteerlineFlow/testing/acceptance/web-query-optimization.md)。本次按后续授权实施，业务查询统一由 Query options 定义；当前项目连接核验后预取执行方案与任务首屏，方案新鲜期 60 秒，动态查询只轮询可见区域，操作按资源族失效。项目切换保留原作用域取消。后续 SSE 更新同一缓存，不新增持久服务缓存、同步框架或后端接口；实际验证范围归专属验收，本轮不实现 SSE。

2026-10-23：全局入口名称调整为“执行方案配置”，原项目“执行设置”调整为“配置”，移除重复执行方案列表；共享生成并发设置移至“配置”，保留所有项目共享、默认 5 和保存后重启生效。两页复用项目页侧栏、顶栏和配色。从项目进入执行方案配置后返回原项目/页面及查询、片段；直达时采用最近有效项目，否则提供项目选择。未保存修改离开时提示，零项目仍可访问 `/settings`。验证范围见[专属验收](../../SteerlineFlow/testing/acceptance/configuration-navigation.md)。

2026-10-21：新增全局[执行配置管理](../../SteerlineFlow/spec/web-execution-config.spec.md)页，路径 `/settings/execution-configs`，零项目时也可进入。Web 前后端负责原生目录探测、配置编辑及条件保存；执行服务仅消费配置和投影原加载摘要。保存后明确待重启，冻结调试包只读，Runner 不新增探测接口。

## 现有依据

[第二阶段原型](prototypes/planning-ui/index.html)是单文件 HTML/CSS/JavaScript 演示，约 390 KB，没有前端构建工程。它已经覆盖规划工作台、版本蓝图、长期文档、Roadmap 草稿、DAG、执行、验收、测试、体验版本、Bug 和设置。交互细节见[原型说明](prototypes/planning-ui/README.md)和[归档的界面设计](../../SteerlineFlow/archive/design/planning-ui.md)。原型是产品参考，正式页面仍需按真实能力和契约接入。

仓库当前文档和相关 Git 历史未找到已确认的 React/Vue/Vite 等 Web 技术栈。历史客户端方案为 C# / WinUI 3，工程已由 #91 删除，见[源码索引](../architecture/current-code-architecture.md)。当前可复用成果是 Go 产品接口、本机 HTTP 协议和已交付的 Web 前端；客户端边界见[规划架构](planning-architecture.md)，已有路由见[本机客户端协议](local-client-protocol.md)。

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

## 浏览器接入需要新增的能力

现有 `httpapi.Server.ServeHTTP` 拒绝所有非空 Origin；协议要求读取受当前 Windows 用户保护的 discovery/credential 文件。浏览器不能直接复用 TUI 的文件发现与凭证读取，单纯增加 Vite proxy 不能构成正式浏览器认证方案。

`steerline-web.exe` 提供浏览器 handler、静态页面和同源 API；`steerline.exe serve-app` 拥有配置目录和受限项目路由，通过既有 native HTTP 客户端访问原项目服务。应用服务无项目数据库或调度器，Web、应用服务与项目服务分别正常停止。浏览器会话绑定 home/Web 实例，项目 URL 使用 `/projects/{id}/dag|runs|settings`，项目切换不改变原命令信封。

浏览器入口遵循以下契约；路由、配置及实现细节归上述 Spec/Plan：

- 固定到 `127.0.0.1` 的受配置稳定地址，严格核验 Host。当前 native API 的随机端口发现可保留；Web Origin 应在服务重启后稳定，否则 IndexedDB 中的原请求和草稿不能由新 Origin 读取。端口冲突如实失败，不静默换地址丢失恢复入口。
- 由本机启动/打开入口完成受控首次授权，建立绑定服务实例的浏览器会话；建议 HttpOnly、SameSite cookie 配合写操作的 Origin/CSRF 校验。原生 Bearer 凭证不进入页面脚本、localStorage、日志或普通 URL。不能因为来自 loopback 就免认证。
- 页面与浏览器 API 同源。开发热更新也通过明确的本机开发接线运行；不得通过全放开 CORS 或去掉 Origin 校验让页面临时可用。
- 客户端显示项目与当前服务实例；实例变化重新核验身份，不能把旧信封自动改为新实例或将原草稿转投另一候选。
- 关闭页面不停止服务/worker；离线保留最后观察和输入并禁写。浏览器不能直接观察 Windows 进程与文件锁，首期不把断线显示为停服成功；停服功能若要交付，须另落实可核实的宿主观察能力。

远程阶段再设计监听、TLS、认证和授权。远端 worker 运行能力与从其他电脑访问 Web 服务是不同能力。

## 数据与命令约束

首期轮询已实现查询，展示近期 Activity、问题背景和完整 final。原型中的实时增量文字、工具正文和重连续读是待接入能力，选择 React 不等于实现了 SSE 或过程采集。

Query key 包含项目、服务实例以及相应 Task/Run/Execution/Attempt 等身份；晚到响应只更新原 key，不覆盖当前选择。业务资格以 Go 返回及提交时的复核为准，前端禁用按钮只是交互提示。断线时展示最后观察时间，查询缓存不表示服务仍在线。

所有业务变更禁用自动重试，提交前先在 IndexedDB 保存完整、不可变的原 request_id/目标/版本/payload；保存失败不能发送。未知结果查询原 ID，只有权威 command_not_found 才按原信封补交；不得换 ID、换版本或借 Query 自动重试重放。跨标签页及刷新恢复需要在客户端请求层统一处理，组件不各自实现发送恢复。

输入草稿绑定原候选和 Decision；中文 IME 组词、Enter 换行与显式提交分别处理。文档和 Agent 正文按不可信内容安全渲染，首期不执行原始 HTML。可持久草稿与信封不是业务权威，不保存宿主原生凭证。

## 已确认首批范围与后续顺序

1. 工程与浏览器接入：构建/静态发布、稳定本机入口、会话认证、身份查询、离线状态、请求持久化与原 ID 对账；搭好原型的导航和布局基础。
2. 已有能力闭环：任务/DAG 与详情、候选切换、近期活动/final、回答问题、现有启动/候选动作、原生会话查看和 PR/交付展示。动作与选项按当时正式 API 能力接入，参数设置不伪造原型中的动态发现。必要能力缺口在实施前明确补齐范围。
3. 后续产品页：规划 Agent 与草稿转正、验收/测试管理、体验版本、Bug 自动修复及文档维护等，按各自真实后端契约逐项推进。缺能力不使用演示数据冒充真实功能。

页面骨架与客户端代码可以先推进。已有 #92/#93 是原接口解耦任务，不自动成为 Web 工程前置；浏览器接入涉及 httpapi/应用接线时，先核对它们及 Worker 三层重构的实际在途修改，按[并行边界](../architecture/parallel-development.md)保留双方成果。本建议不是正式执行 DAG，也不启动其他聊天或工作区。

正式实施的 Spec/Plan 应链接任务专属 acceptance，并按最小必要范围选择验证：浏览器认证/非法来源拒绝、固定身份与晚到响应、命令未知结果对账/刷新恢复、中文输入和一个已有任务执行闭环。纯前端组件检查可使用隔离夹具；真实服务调试先运行 `scripts/new-debug-release.ps1` 创建 `C:\SteerlineDebug` 的独立环境，AI 再使用该包的 `start.bat --simulate` 显式选择模拟方案，真实 PR 只用本次 SteerlineTest 专属分支。启动与冻结规则见[调试说明](../runtime-install-debug.md)。分级入口按[现有测试说明](../../tests/e2e/README.md)登记，不以一次前端构建代替业务接线验证，也不默认跑 full 或全平台矩阵。

## 技术依据

下列官方资料核对于 2026-10-14，选择理由为本项目判断：

- [React 的独立应用构建与 Vite/TypeScript 示例](https://react.dev/learn/build-a-react-app-from-scratch)，[Vite 后端集成](https://vite.dev/guide/backend-integration)。
- [React Router Declarative 安装](https://reactrouter.com/start/declarative/installation)，[Tailwind 的 Vite 接入](https://tailwindcss.com/docs/installation/using-vite)，[shadcn/ui 的 Vite 接入](https://ui.shadcn.com/docs/installation/vite)。
- [TanStack Query 默认行为](https://tanstack.com/query/latest/docs/framework/react/guides/important-defaults)，[查询重试配置](https://tanstack.com/query/latest/docs/framework/react/guides/query-retries)。
- [React Flow 定制节点](https://reactflow.dev/learn/customization/custom-nodes)，[布局选择](https://reactflow.dev/learn/layouting/layouting)。
- [Vitest](https://vitest.dev/guide/)，[Playwright](https://playwright.dev/docs/intro)。
