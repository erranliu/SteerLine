# 产品读取、命令入口与 Web 容器重构验收

归属：本次普通开发重构，[Spec](../spec/product-read-command-web-refactor.spec.md) / [Plan](../spec/product-read-command-web-refactor.plan.md)。基线 `57fadfce9390982748d5ff39e6e378c47f0cf5fc`。三批属于同一项重构的可分审交付范围，不覆盖以前的生命周期清理或环境检查验收。

当前状态：审计/设计完成，产品未实施、全部产品项待验。本轮仅写文档及做静态检查，不创建 Runner/服务、调试包或真实 PR。

## 审计结论

| 部分 | 采纳范围 | 暂缓 |
| --- | --- | --- |
| 候选读取 | 七组状态组合、显式参与/提交/停止事实、详情/历史共同原始状态规则、TaskDelivery 标签分开 | 整体替换 Runtime/RunView 与读取仓库、重命名全部历史 DTO |
| 命令入口 | 唯一仲裁 owner 下按职责拆解析/映射/派发/权威查询；保留三种受理顺序 | 通用 command bus/插件/反射配置及统一模板 |
| Web 容器 | 连接与原请求副作用 hooks、设置/操作渲染组件 | 全局状态库、第二套 cache/journal、全页面重构 |

静态审计确认详情 completed→delivered、历史 completed 原值、无 Projection 详情空/历史 SubmitState 的不同解释，以及前端以状态名单推参与、running 在 winner 之前的显示顺序。API 校验/映射/派发重复解码及 App 中的 effect/mutation 分散也已核对。完整读取链、请求锁/记录 owner、query/journal 和既有竞态测试具有保留价值，不推倒重建。

## 自动验收安排

所有命令与时点遵循 Plan 测试表。原始 Go/E2E 报告用 runner 新目录；Web 日志用本次新建的空 temp 目录。执行时登记 source commit/dirty、命令、非零选择、日志/summary 路径、结果及清理；当前没有产品通过证据。

| ID | 场景与必要夹具/能力 | 通过条件 | 状态 |
| --- | --- | --- | --- |
| <a id="rcw-a01"></a> RCW-A01 | 新增 app `TestCandidateReadFactsAcrossViews`，详情/历史同输入；Spec 七行组合；现有 runs/state、列表、答案、任务详情测试 | 公共原始状态一致，参与/提交/观察/交付分开，winner 不被旧 running 压过，未知/无观察不伪造终态；原事实不改 | 待实施/待验 |
| <a id="rcw-a02"></a> RCW-A02 | 原公开输入、SQLite 同事务/唯一交付/原绑定、checkpoint、依赖边界；Go/Web 字段及消费者检索 | 无额外读取事务/外部副作用，无 Runtime 整体重建，writer 资格不改变；无旧 DTO 回退 | 待实施/待验 |
| <a id="rcw-a03"></a> RCW-A03 | 复用 HTTPCommandRegistry/Boundary、Reconciliation、环境检查/native 测试和应用转发；L4 runtime.dual-home-isolation | 同 ID 同意图、跨 action 冲突、原映射固定、先后顺序/未知处理/权威失败/原生未知屏障及历史只读保持 | 待实施/待验 |
| <a id="rcw-a04"></a> RCW-A04 | 拆出 helper 后扩展结构守卫，并加入新文件中的违规 fixture | 请求锁/记录 owner 唯一，不引入具体资源；正常 dispatch 消费原映射；改名不能绕过守卫 | 待补能力/待验 |
| <a id="rcw-a05"></a> RCW-A05 | app/project-switch 与 client/queries mounted 测试；既有 fake-indexeddb/deferred fetch，补必要 offline/unmount/手动并发断言 | StrictMode/切换/卸载无旧结果污染、单个自动对账循环、原实例失效；原 journal/提交顺序/重放资格不变 | 待实施/待验 |
| <a id="rcw-a06"></a> RCW-A06 | 设置及操作组件原测试、npm build、最终 L1/build-only、选定 controlled L4 | 路由/缓存/回执行为保持；TypeScript/Vite 成功；L1 complete/passed、E2E passed/cleanup_complete；不把搬动计成功能收益 | 待实施/待验 |

已有能力：定向 test-feedback、Windows 独立进程 Scenario 选择、controlled 双 home 重放夹具、Vitest mounted App 和 fake-indexeddb 均已存在。不新增自动化平台/场景 ID。待补的跨视图用例、守卫违规反例、必要竞态断言已安排在 Plan 实施步骤，必须在验收前可执行；不以缺少驱动为由标通过。

测试无需真实 Agent/PR；不访问 C:/SteerlineInstall 或 C:/SteerlineRuntime。完整原生专项仍归 Runner，旧未验项不因本项重构改为通过。固定 L1 的新增选择核对和零匹配检查由同一实施批次完成。

## 人工参与

| ID | 内容 | 时点与阻塞范围 | 状态 |
| --- | --- | --- | --- |
| RCW-M01 | 可选：设置导航、退出参与/已交付与资源 running 同时显示时是否易读 | 自动项完成后用户窗口；非实现前置，不阻塞其他批次；需要浏览器现场则生成新的 simulate 调试包 | 未安排、非阻塞 |

## 本次文档检查

2026-10-16：`python scripts/check_doc_links.py`：Markdown: 461 files, 2798 local navigation links, 0 errors；`git diff --check` 无诊断。另逐项核对 Spec/Plan/本记录的六个自动验收 ID 与 Plan 目标锚点；产品代码未修改、产品测试未执行，未创建服务、调试包或 PR。以上仅为设计静态核查，不计产品验收通过。
