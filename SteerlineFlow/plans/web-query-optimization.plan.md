# Plan：SSE 前的 Web 查询与缓存优化

范围归 [Spec](../spec/web-query-optimization.spec.md)，要求与结果归[专属验收](../testing/acceptance/web-query-optimization.md)。2026-10-15 用户在规划复核后明确授权实施；普通开发，不启动 roadmap 或其他聊天，实际结果统一登记专属验收。

2026-10-15 按用户要求 rebase 到 `origin/main`，调查基线更新为 `47fafc12721ceb8fa39ba1ab227872fd14513bcb`（原基线 `411110d901c62d389cb2b328a5ba72d1268617ba`）。主干已合入 PR #320 的独立 Web 与项目切换，本任务直接基于其现有接线实施。已重新读取 `docs/spec/SpecGuide.md` 全文，规范未变化：文件最后修改 commit `94e704300485b82d54e44991148074841ed66019`，SHA256 `85b656090674e1f953320b6be62d17b6888248ef7eec5fe044b9d3f408bfe4de`。

## Rebase 后的整合结论

五项优化范围保持，具体入口须更新：App 已负责浏览器会话/项目目录/当前项目 identity，业务查询和命令位于按 home/项目/原生实例挂载的 Workspace。`Api.get` 已组合调用方 signal 与 Workspace 生命周期，切换时 deactivate 取消旧读取；原实例变化后允许只读查询旧回执，禁止向新实例补交。本批直接复用这些能力，不能按旧 App 布局重新接一套逻辑。

调试生成器已支持 `-WebProjects 1|2`，认证入口为独立 `steerline-web.exe open`，项目 API 为 `/api/v1/application/projects/{project_id}/...`，深链接为 `/projects/{project_id}/{dag|runs|settings}`。浏览器验收必须使用这些实际入口；原单项目 smoke 与新查询场景使用各自全新包，不合并到同一现场运行。

## 整合与实施顺序

所有步骤属于同一批普通开发，顺序实施，不拆任务 DAG。

1. **统一查询与参数。** 新增小型 `web/src/shared/api/queries.ts`，用已有 TanStack Query options 承接资源键、规范化参数和 queryFn；保留 `[api.home, project_id, native_instance_id, ...资源与参数]` 前缀，原 `Api.get/final` 继续负责应用路由、取消和正文核验。统一 DAG / Runs 的 tasks 首屏及 Settings / TaskDetail 的 presentation，集中使用正式 TS 类型。Workspace snapshot 也复用同一身份键，移除 run-tasks。App 的会话/目录/identity 和同 home 请求日志保持既有键与所有者。不封装 QueryClient、不创建全局 store，不改 DTO/后端。
2. **基础预取与缓存展示。** 在 Workspace 原 online 门槛下，按 home/项目/原生实例及连接生命周期预取 presentation、tasks 首屏；直接调用已有 QueryClient.prefetchQuery，与页面共用 options，并沿用 Api.activate/deactivate 和读取取消。tasks/presentation 不消费页面观察者的 Query signal，使在途共享读取跨页面延续；Workspace 销毁时取消传输及原作用域 Query，其他查询保留 Query signal。App 目录/identity/snapshot 每次采样不重新调度预取，未选中项目不预取。presentation 新鲜期 60 秒，移除详情的 10 秒轮询；保留动态查询的现有新鲜期和缓存回收。修正首次加载/空数据区分及后台更新清空内容的问题；不把列表 DTO 填成详情、不跨身份保留 placeholder。保持现有 session、目录、identity、snapshot 及命令可操作条件。
3. **关闭隐藏查询。** Dag 的 graph 加图视图条件，图计数只在图视图展示；Output 的 activity 加活动标签条件。final 原有条件保留。启用条件变化复用 Query 的观察/缓存生命周期，不自行创建轮询、取消或后台可见性调度层。
4. **集中最小失效规则。** 在现有 API 查询模块或相邻小函数维护 Spec 的操作/资源族映射，Workspace mutation、Operations 手动对账与自动对账使用原 Api.home 和原信封，不从当前选中项目/新实例推断。只在对账观察结果变化时触发业务失效，保留原日志更新。对账轮次、原 ID、补交、取消和持久化算法不变；不增加账本状态。原候选无法可靠定位时用原 home/原命令实例的全部动态查询覆盖，读取旧实例回执也只作用于旧身份键。App 向 Workspace 提供现有会话/目录/选中 identity 的显式刷新回调，工作区手动刷新再刷新当前实例已有查询及同 home 日志；替换两处无范围 invalidateQueries，不刷新其他项目。取消普通操作的项目全量 predicate。
5. **定向验证及交付记录。** 增加下面表中的必要场景，扩展现有浏览器驱动记录只读 API 请求和延迟单次真实响应；不伪造业务响应。新查询场景复用 project-switch 的全新双项目包守卫、授权与正常清理方式，仅提取实际共用的最小测试 helper，不增加安装器或通用进程框架；保留原 smoke 的新单项目包入口，并将 `scripts/test-web.ps1 -DebugConfig` 的单项目浏览器调用显式限制到 smoke.spec.ts，避免误调要求不同现场的新场景。运行最小自动验收，更新 acceptance 实际结果、证据和未验限制；更新 Web 技术说明中缓存/轮询描述。SSE 的协议与轮询替换留给后续任务。

已重新调查 `main.tsx`、`app/app.tsx` 的 App/Workspace 分工、`features/tasks/{dag,task-detail,dispatch}.tsx`、`features/runs/{runs,answer}.tsx`、`shared/api/{client,types,journal}.ts`、`app/project-switch.test.tsx`、`web/tests/{smoke,project-switch}.spec.ts`、`web/README.md`、`scripts/{new-debug-release,test-web-smoke}.ps1` 及包内启动/停止入口。HTTP/存储调查只用于确认现状，本批不修改其查询实现。answer.tsx 的提交资格核验、原请求 journal 及 API 的身份/生命周期核验仍是既有边界。

## 测试计划

全部自动项链接[本任务验收表](../testing/acceptance/web-query-optimization.md)。WQ 前缀场景在实施时补入现有前端测试目录；新增 `queries.test.ts`、`features/runs/runs.test.tsx`，扩展 App/TaskDetail/DAG 测试。它们是 Web 定向测试，不冒充 Go L1–L3 注册场景。使用隔离 QueryClient、可控 Promise 与必要的 fake timers，不靠真实网络速度断言耗时。

| 验收项 | 级别 / 组与场景 ID | 环境 / 命令 | 执行时点与通过条件 | 证据登记 |
| --- | --- | --- | --- | --- |
| WQ-A1 | Web 定向 `web.query` / WQ.cache-sharing | Vitest/隔离 QueryClient；`npm --prefix web test -- src/shared/api/queries.test.ts` | 步骤 1 后：同身份同参数并发读取只发一次；DAG/执行页首屏共键；home/项目/原生实例/游标/参数不串用，输出保留 Attempt | WQ-A1，Vitest 日志 |
| WQ-A2 | `web.query` / WQ.bootstrap | 同上及 App/切换测试；`npm --prefix web test -- src/shared/api/queries.test.ts src/app/app.test.tsx src/app/project-switch.test.tsx` | 步骤 2 后：完整核验前不预取，选中 Workspace 首次 online 只预取两项，目录/identity/snapshot 采样不重复触发；presentation 新鲜复用、过期访问重查；切换取消旧预取，其他项目不预取 | WQ-A2，日志/请求计数 |
| WQ-A3 | Web 组件 / WQ.cached-render | Vitest/延迟 Promise；`npm --prefix web test -- src/features/tasks/task-detail.test.tsx src/app/app.test.tsx src/app/project-switch.test.tsx src/features/runs/runs.test.tsx` | 步骤 2 后：响应未完成前同目标缓存可见，首次加载不是空结果，错误保留观察；跨 home/项目/Task/Attempt 不显示旧目标，切换取消与离线禁写保留 | WQ-A3，组件日志 |
| WQ-A4 | Web 组件 / WQ.visible-query | Vitest/fake timers；`npm --prefix web test -- src/features/tasks/dag.test.tsx src/features/runs/runs.test.tsx` | 步骤 3 后：补新增 dag.test.tsx（保留原 dag.test.ts 布局测试）；隐藏资源不启动新请求，切回恢复；不把已开始请求完成算作新轮询 | WQ-A4，计时器/请求断言日志 |
| WQ-A5 | `web.query` / WQ.invalidation | Vitest/显式命令目标与延迟对账；`npm --prefix web test -- src/shared/api/queries.test.ts src/app/app.test.tsx src/app/project-switch.test.tsx` | 步骤 4 后：Spec 各动作族、拒绝/unknown、对账结果变化、保守分支与手动刷新命中正确键；切换后完成/旧实例回执只失效原 home/项目/实例；手动刷新覆盖会话/目录/当前 identity，不触发其他身份/普通操作的 presentation，不清空缓存或把 accepted 当 applied | WQ-A5，命中/排除断言 |
| WQ-A1–A5 回归 | Web 已有 client/input 与构建 | `npm --prefix web test`；`npm --prefix web run build` | 最终实现后：Web 小范围全部测试及 TS/生产构建通过，原信封/取消/中文输入反例保留 | 各项补充回归，构建日志 |
| WQ-A6 / 驱动编译 | 浏览器驱动静态检查 | 在 `web/` 执行 `npm exec -- tsc --ignoreConfig --noEmit --strict --target ES2022 --module ESNext --moduleResolution Bundler --skipLibCheck --types node tests/query-cache.spec.ts tests/project-switch.spec.ts` | 驱动调整后：新场景与共用 helper 的调用类型通过；不替代实际浏览器运行 | WQ-A6，驱动检查命令与结果 |
| WQ-A6 / 查询与切换 | 定向真实浏览器 / web.query-cache | 全新双项目包；设置 `STEERLINE_WEB_PACKAGE` 后 `npm --prefix web run test:e2e -- query-cache.spec.ts` | 组件通过后：新增 query-cache.spec.ts，测试自行启动/正常清理；真实两项目切换、返回缓存、同键请求复用和隐藏查询停止通过，不向未选项目预取 | WQ-A6，双项目包 evidence/browser/ 与请求摘要 |
| WQ-A6 / 原接线回归 | 定向真实浏览器 / web.smoke | `./scripts/test-web-smoke.ps1 -WebAddress 127.0.0.1:<另一空闲固定端口>` | 查询场景后：脚本生成另一个新单项目包、启动、只运行 smoke.spec.ts、正常清理；已有操作/反馈/final 接线通过，不沿用双项目包任务状态 | WQ-A6，单项目包 evidence/smoke.log 与 smoke-cleanup.json |
| 文档 | 静态检查 | `python scripts/check_doc_links.py`；`git diff --check` | 本轮规划及最终交付各一次：无新增链接/格式错误 | acceptance 的规划/交付检查摘要 |

真实浏览器能力安排：实施步骤 5 增加 query-cache.spec.ts，复用现有双项目驱动的授权与环境守卫，进入 `/projects/{project_id}/...` 并统计 `/api/v1/application/projects/{project_id}/...` 请求。Playwright 记录真实 GET 请求；需要控制返回时机时先读取真实响应再延迟交付，不替换 JSON，不拦截/延迟业务 POST 或资格核验。稳定窗口排除启动预取和已在途请求，以“新请求数”和“真实响应释放前 DOM 已显示”验收，不承诺固定毫秒。

查询场景准备：先运行 `./scripts/new-debug-release.ps1 -WebAddress 127.0.0.1:<本次空闲固定端口> -WebProjects 2`，设置 `STEERLINE_WEB_PACKAGE=<新包绝对路径>`、`STEERLINE_WEB_EVIDENCE=<新包>/evidence/browser`。沿用 project-switch 驱动的全新包守卫（尚无 scheme.json），测试调用本包 helper 的 `debug prepare --simulate` / `debug run --simulate` 冻结并启动两项目/应用/Web，不在测试前另行 start。使用包内 `steerline-web.exe open -print-url` 授权，结束正常停止 Web、应用及两个项目服务，核对本包进程/监听归零。若仅手动查看，才使用该包 `start.bat --simulate` 与 `web.bat`；不得把已启动包交给要求全新现场的测试。

原 smoke 回归由现有 `scripts/test-web-smoke.ps1` 负责唯一生成器、新单项目包、simulate 启动和三类服务清理，环境变量及包内配置由该脚本设置。两类测试不得使用无文件过滤的 `npm ... test:e2e`，避免误执行需要不同现场的 project-switch。所有调试只在 `C:\SteerlineDebug`；不使用或修改 `C:\SteerlineRuntime`。测试前落实构建/浏览器依赖，失败和清理结果写各自新包 evidence。

本批仅前端查询组织与显示，不改 Go/worker/原生协议，不选择 Go Full、L2/L3、live 或真实 PR 测试。现有 web.smoke 复用当前受控闭环做接线回归；不另建性能平台。WQ-M1 在自动项完成后的用户体验窗口进行，可后置，只影响人工体验签收。Plan 不维护实时通过状态。
