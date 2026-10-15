# 验收：SSE 前的 Web 查询与缓存优化

本文件只归属本次普通开发查询优化。[Spec](../spec/web-query-optimization.spec.md) · [Plan / 测试计划](../spec/web-query-optimization.plan.md)。2026-10-15 用户后续授权实施；实现与 WQ-A1–A6 自动验收全部通过，人工体验 WQ-M1 待安排；未更新安装环境。不复用已有 Web 验收结论作为本次通过证据。

## 自动验收

| ID | 通过条件 | 状态 | 证据 |
| --- | --- | --- | --- |
| WQ-A1 | 相同 home/项目/原生实例/参数共享查询、并发去重；不同 home/项目/cursor/limit/原生实例及输出 Attempt 隔离，删除 run-tasks 同义缓存 | 自动通过 | 下述 70 项前端测试及构建日志 |
| WQ-A2 | 通过原 App/Workspace online 门槛后有界预取 presentation/tasks 首屏；不阻塞首屏，不逐次目录/identity/snapshot 重触发，不递归或预取未选项目；切换取消旧预取；presentation 60 秒新鲜期、无定时轮询、过期访问和手动刷新可更新 | 自动通过 | 下述 70 项前端测试及构建日志 |
| WQ-A3 | 同目标缓存先显示且后台刷新不清空；首次无缓存显示加载、错误保留最后观察；不同 home/项目/任务/执行不借用旧内容，切换取消、离线禁写与资格门槛保留 | 自动通过 | 下述 70 项前端测试及构建日志 |
| WQ-A4 | 图/列表及活动/final 只启动可见区域查询（明确允许启动 tasks 首屏预取）；隐藏后不发起新轮询，切回恢复；App 会话/目录/当前项目 identity、Workspace snapshot 与对账行为保留 | 自动通过 | 下述 70 项前端测试及构建日志 |
| WQ-A5 | 各操作族、失败/unknown、对账变化和手动刷新按 Spec 定向失效；切换后完成与旧实例回执只失效原 home/项目/实例；普通操作不扩散到 presentation 或可明确排除的候选输出，保守分支不漏动态查询；手动刷新包含同 home 会话/目录/当前 identity；原提交/对账/资格/中文输入回归通过 | 自动通过 | 下述 70 项前端测试及构建日志 |
| WQ-A6 | 新独立双项目 SteerlineTest simulate 包验证真实浏览器请求复用、缓存展示、隐藏查询和项目切换隔离；另由 test-web-smoke.ps1 生成全新单项目包完成原 smoke 接线回归；使用真实响应，分别完成 Web/应用/项目服务清理 | 自动通过 | 查询场景与原 smoke 的独立现场、日志与清理见下文 |

## 人工验收

| ID | 内容 / 参与窗口 | 阻塞范围 | 状态 |
| --- | --- | --- | --- |
| WQ-M1 | 自动项完成后用户切换 DAG/列表/执行/设置、返回已访问任务、打开执行方案，确认内容与提示自然；具体时间待用户安排 | 仅人工体验签收，可后置，不阻塞无关开发或 SSE 规划 | 待安排 |

## 证据登记

实施时按 ID 记录实际基线、命令、结果、请求计数/渲染时序及日志目录。真实浏览器分别记双项目查询包和单项目 smoke 包路径、scheme=simulate、浏览器报告及 Web/应用/各项目服务清理结果；失败与未验如实保留。不得把静态文档检查登记为产品自动验收通过。

规划检查（2026-10-15）：`python scripts/check_doc_links.py` 通过（421 个 Markdown、2672 个本地导航链接、0 错误），`git diff --check` 通过。仅文档检查；产品自动验收未执行。

Rebase 复核（2026-10-15）：从 `411110d9` 更新到 `origin/main` 的 `47fafc12721ceb8fa39ba1ab227872fd14513bcb`，无冲突，原规划修改已恢复。主干 PR #320 已交付独立 Web 与项目切换；据此同步 Spec/Plan 的 home 隔离、App/Workspace 所有者、取消与旧回执边界，以及双项目查询/单项目 smoke 独立现场安排。复核当时优化范围未扩大，产品尚未实施、未验；文档链接检查通过（421 个 Markdown、2680 个本地导航链接、0 错误），差异格式检查通过，未运行产品测试。

## 实施与自动验证（2026-10-15）

基于 `47fafc12` 的本工作区差异实施，所有调试包均记录 `source_dirty=true`。新增 `shared/api/queries.ts`，统一缓存键与预取、按资源族失效、原回执结果变化识别；App/Workspace 继续负责原身份与生命周期，业务命令与资格核验不改。

WQ-A1–A5：`npm --prefix web test` 为 10 个文件、70 项通过（原 47 项保留，新增 23 项）；`npm --prefix web run build` 的 TypeScript/生产构建通过。证据：`C:/Users/lmas1/AppData/Local/Temp/steerline-web-query-20261015/client-tests.log`、`build.log`。覆盖共享请求/参数与 home/项目/实例/Attempt 隔离、60 秒新鲜度、连接门槛和采样不重复预取、跨项目取消及晚到响应、缓存保留与错误/新目标、隐藏图/输出查询停止及恢复、各动作资源族、旧实例回执和手动刷新；原客户端对账/输入检查继续通过。

WQ-A6 首轮失败包：`C:/SteerlineDebug/dev-20261015-120136-47fafc12-dirty`。真实浏览器发现 DAG 卸载其隐藏 tasks 观察者时取消在途预取，执行页随后再发同键请求（预期 1、实际 2）。修复 tasks/presentation 使用 Workspace 生命周期取消、跨页面保留在途读取，并新增观察者切换回归。失败保留 `evidence/query-browser.log`、`evidence/browser/`；`evidence/web-query-cache.json` 证明 Web/应用/两项目正常清理，processes=0、listeners=0、stop_errors=[]。未延长等待或修改预期掩盖失败。

修复后重新生成独立包复验，不重用已冻结现场；最终结果见下文。未执行 Go Full、L2/L3、live 或真实 PR 测试，未触碰安装目录。


WQ-A6 第二轮包：`C:/SteerlineDebug/dev-20261015-120837-47fafc12-dirty`。请求复用已验证为 1 次，真实响应释放前缓存行数为 2，隐藏图新请求为 0；切到 B 项目后，驱动误将规范化小写仓库路径与硬编码混合大小写名称比较，整体场景未通过。修正驱动为与目录 API 返回的完整 repository 精确比较，不修改产品或放宽路径身份要求；该包同样正常清理，processes=0、listeners=0、stop_errors=[]。日志、trace 和 `evidence/web-query-cache.json` 保留，重新生成新包完成整体验收。


WQ-A6 查询场景最终通过：`C:/SteerlineDebug/dev-20261015-121652-47fafc12-dirty`，scheme=simulate；`npm --prefix web run test:e2e -- query-cache.spec.ts` 为 1 项通过，4.5 分钟，无重试。真实响应仅延迟交付、未替换业务 JSON；DAG→执行页共享在途 tasks 请求数为 1，服务器响应释放前缓存行数为 2，隐藏图稳定窗口新增请求为 0，未选项目未预取；A→B→A 的 URL/规范化仓库路径与缓存隔离通过，page_errors=[]。正常停止 Web/应用/两项目服务，processes=0、listeners=0、stop_errors=[]。证据：该包 `evidence/query-browser.log`、`evidence/web-query-cache.json`、`evidence/browser/`（截图/trace）；已实际查看查询场景截图，页面与项目身份正常。

浏览器驱动严格类型检查通过，证据为 `C:/Users/lmas1/AppData/Local/Temp/steerline-web-query-20261015/browser-driver-types.log`。原 smoke 使用另一新单项目包，结果见下文。


WQ-A6 原 smoke 最终通过：`./scripts/test-web-smoke.ps1 -WebAddress 127.0.0.1:19866` 生成单项目包 `C:/SteerlineDebug/dev-20261015-122536-47fafc12-dirty`，scheme=simulate。真实 Edge 场景 1 项通过，11.2 分钟，无重试、完整 fresh 闭环；涵盖原启动请求丢回执后按原 ID 对账且仅 POST 一次、完整 final、中文草稿刷新恢复、追加反馈后原 session/execution/attempt 的第四轮、交付与后继依赖解锁、离线禁写及重连。provider 最终 sessions=1、turns=4、dispatches=4。已实际查看查询场景及执行窄屏截图，窄屏无水平溢出断言通过；用户自然交互体验仍由 WQ-M1 后置验收。

证据：单项目包 `evidence/smoke.log`、`evidence/browser/`（宽/窄截图、trace 与 real-project-summary）、`evidence/smoke-cleanup.json`。清理报告 passed=true、browser_passed=true、processes=0、listeners=0、stop_errors=[]，Web/应用/项目服务全部正常结束。

交付收尾：`scripts/test-web.ps1 -DebugConfig` 显式只运行 smoke.spec.ts，避免单项目入口误运行要求全新双项目现场的用例；PowerShell AST 解析通过。文档链接检查通过（421 个 Markdown、2680 个本地导航链接、0 错误）、`git diff --check` 与新文件空白/冲突标记检查通过。Web 改动格式检查通过，无新增依赖、后端协议或查询持久层；后续 SSE 继续复用同一 Query 缓存。未执行 Go Full、L2/L3、live 或真实 PR 测试，未修改安装目录。

## 质量审计与提交前复验（2026-10-15）

按用户要求审计全部改动，复核缓存生命周期、参数及 home/项目/实例/执行隔离、隐藏查询、命令与旧回执失效范围、手动刷新和验收覆盖，未发现新的产品逻辑阻断问题。`git fetch origin` 后主干仍为 `47fafc12`，无需再次 rebase；Spec/Plan 的行为与测试范围不变。

修正新浏览器驱动两处验证缺口：任务行收到响应并显示后再断言共享首屏请求仍为 1 次，避免只在导航初期计数；清理盘点检查失败时先保存最后盘点、停止错误、页面错误及检查错误，再抛出失败，保证失败现场也留存证据。

另生成全新双项目包 `C:/SteerlineDebug/dev-20261015-124842-47fafc12-dirty`（scheme=simulate），运行 `npm --prefix web run test:e2e -- query-cache.spec.ts`，1 项通过、3.4 分钟、无重试。共享请求 1 次、响应前缓存行 2 个、隐藏图新增请求 0 次、A→B→A 隔离通过，page_errors=[]；正常清理 processes=0、listeners=0、stop_errors=[]。证据为该包 `evidence/query-browser.log`、`evidence/web-query-cache.json`、`evidence/browser/`。生成器同时完成当前源码的生产前端构建。

修改后的浏览器驱动严格类型及 Prettier 检查通过，文档链接和差异检查通过。最初静态检查与生成器的依赖安装并发，导致 tsc 未能启动；已中止该次检查，依赖就绪后重跑通过。本轮未修改产品实现，沿用上述 70 项前端测试和原 smoke 的既有有效结果，不重复无关业务闭环；WQ-M1 仍待人工体验。
