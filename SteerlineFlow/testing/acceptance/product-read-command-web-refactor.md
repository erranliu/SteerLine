# 产品读取、命令入口与 Web 容器重构验收

归属：本次普通开发重构，[Spec](../../spec/product-read-command-web-refactor.spec.md) / [Plan](../../plans/product-read-command-web-refactor.plan.md)。基线 `57fadfce9390982748d5ff39e6e378c47f0cf5fc`。三批属于同一项重构的可分审交付范围，不覆盖以前的生命周期清理或环境检查验收。

当前状态：用户授权后 A/B/C 已实施；RCW-A01–A06 自动项通过，RCW-M01 可选人工体验未安排、非阻塞。受控 L4 创建了临时隔离服务并完成清理；没有创建真实业务 PR 或安装调试包，本次开发 PR 另行提交。

## 审计结论

| 部分 | 采纳范围 | 暂缓 |
| --- | --- | --- |
| 候选读取 | 七组状态组合、显式参与/提交/停止事实、详情/历史共同原始状态规则、TaskDelivery 标签分开 | 整体替换 Runtime/RunView 与读取仓库、重命名全部历史 DTO |
| 命令入口 | 唯一仲裁 owner 下按职责拆解析/映射/派发/权威查询；保留三种受理顺序 | 通用 command bus/插件/反射配置及统一模板 |
| Web 容器 | 连接与原请求副作用 hooks、设置/操作渲染组件 | 全局状态库、第二套 cache/journal、全页面重构 |

静态审计确认详情 completed→delivered、历史 completed 原值、无 Projection 详情空/历史 SubmitState 的不同解释，以及前端以状态名单推参与、running 在 winner 之前的显示顺序。API 校验/映射/派发重复解码及 App 中的 effect/mutation 分散也已核对。完整读取链、请求锁/记录 owner、query/journal 和既有竞态测试具有保留价值，不推倒重建。

## 自动验收安排

所有命令与时点遵循 Plan 测试表。原始 Go/E2E 报告用 runner 新目录；Web 日志用本次新建的空 temp 目录。下方登记 source commit/dirty、命令、非零选择、日志/summary 路径、结果及清理。

| ID | 场景与必要夹具/能力 | 通过条件 | 状态 |
| --- | --- | --- | --- |
| <a id="rcw-a01"></a> RCW-A01 | 新增 app `TestCandidateReadFactsAcrossViews`，详情/历史同输入；Spec 七行组合；现有 runs/state、列表、答案、任务详情测试 | 公共原始状态一致，参与/提交/观察/交付分开，winner 不被旧 running 压过，未知/无观察不伪造终态；原事实不改 | 自动通过 |
| <a id="rcw-a02"></a> RCW-A02 | 原公开输入、SQLite 同事务/唯一交付/原绑定、checkpoint、依赖边界；Go/Web 字段及消费者检索 | 无额外读取事务/外部副作用，无 Runtime 整体重建，writer 资格不改变；无旧 DTO 回退 | 自动通过 |
| <a id="rcw-a03"></a> RCW-A03 | 复用 HTTPCommandRegistry/Boundary、Reconciliation、环境检查/native 测试和应用转发；L4 runtime.dual-home-isolation | 同 ID 同意图、跨 action 冲突、原映射固定、先后顺序/未知处理/权威失败/原生未知屏障及历史只读保持 | 自动通过 |
| <a id="rcw-a04"></a> RCW-A04 | 拆出 helper 后扩展结构守卫，并加入新文件中的违规 fixture | 请求锁/记录 owner 唯一，不引入具体资源；正常 dispatch 消费原映射；改名不能绕过守卫 | 自动通过 |
| <a id="rcw-a05"></a> RCW-A05 | app/project-switch 与 client/queries mounted 测试；既有 fake-indexeddb/deferred fetch，补必要 offline/unmount/手动并发断言 | StrictMode/切换/卸载无旧结果污染、单个自动对账循环、原实例失效；原 journal/提交顺序/重放资格不变 | 自动通过 |
| <a id="rcw-a06"></a> RCW-A06 | 设置及操作组件原测试、npm build、最终 L1/build-only、选定 controlled L4 | 路由/缓存/回执行为保持；TypeScript/Vite 成功；L1 complete/passed、E2E passed/cleanup_complete；不把搬动计成功能收益 | 自动通过 |

复用能力：定向 test-feedback、Windows 独立进程 Scenario 选择、controlled 双 home 重放夹具、Vitest mounted App 和 fake-indexeddb 均已存在。不新增自动化平台/场景 ID。已补 app 跨视图用例、改名文件偷读记录的守卫反例，以及自动对账在途时手动查原信封/原缓存与卸载取消的 mounted 断言；未删除原注册竞争、固定映射、未知/权威失败等测试。

测试无需真实 Agent/PR；不访问 C:/SteerlineInstall 或 C:/SteerlineRuntime。完整原生专项仍归 Runner，旧未验项不因本项重构改为通过。固定 L1 的新增选择核对和零匹配检查由同一实施批次完成。

## 人工参与

| ID | 内容 | 时点与阻塞范围 | 状态 |
| --- | --- | --- | --- |
| RCW-M01 | 可选：设置导航、退出参与/已交付与资源 running 同时显示时是否易读 | 自动项完成后用户窗口；非实现前置，不阻塞其他批次；需要浏览器现场则生成新的 simulate 调试包 | 未安排、非阻塞 |

## 实施与引用审计

- 读取链仍为 RunnerSnapshot → taskrunner.Project → Runtime/RunView → productServicePage → query.ServicePage/Candidate → DTO；历史从同一候选调用 RunnerHistory/CandidateSummary。两处状态解释收敛为 runnerstate.ObservedStatus，投影阶段不再二次覆盖状态。七组矩阵拆为 9 个有意义输入（pending/unknown、closed/discarded 分开），原值通过序列化前后比较确认未改。
- app/runner_recovery 继续使用公开观察；decisionAvailability 依赖原 Decision/连接/能力，query 的 async blocks_progress 仍从 waiting_input/blocked 判断；taskrunner 的 Decision/InputForAnswer、Project 的 SlotOccupied/CurrentTask、actor/checkpoint 和 SQLite 停止/交付事务的判定未改。查询不增加事务或外部 I/O。Web 显式参与计数与动作门槛分别消费原事实，未知目标或仅剩一个已知可操作参与者时仍禁止停止，停止请求阻止新输入。
- 普通命令首次业务解析结果供 mapIntent 与 dispatch 使用，记录的固定 intent 派发；权威匹配独立验证原信封。三条受理顺序见[本机协议](../../../docs/product-design/local-client-protocol.md#命令映射)。新文件仍属同一 commandRegistry，未新增 owner、锁、总线或兼容路径；只供历史绑定夹具使用的映射包装移入 reconciliation_test.go。
- Web 的 Api activate/deactivate/cancel 仍在原 layout effect，query keys/interval/StrictMode/prefetch、AbortController 和有界 setTimeout 轮询保持；Operations 仅渲染并调用 useOriginalRequests 的手动回调。新 hooks 没有另一套 cache、journal 或 retry。

实现提交：A `c6b7c0b3`，B `3020a7a9`，C `1dc6e02f`。验证时的 dirty 实现按这三个提交分批保存；文档更新单独提交。

## 自动验证证据（2026-10-16）

环境：Windows/amd64，Go 1.26.4，Node/npm 使用本机工具，Web 为 jsdom/fake-indexeddb。所有原始 Go 报告根目录 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/`，下表 `G/<目录>/summary.json` 均相对于此；同目录保留 go-test.jsonl/stderr，L1 按组保存。source 为 `c510bd556fc95f381025c36adb445a15c8eb8e08` 加本次未提交实现，runner 内含完整 dirty 清单。固定 L1 后仅收尾删除投影重复赋值、移入测试包装和导入格式；对受影响包及跨视图用例补跑下表定向回归，没有再跑全量或重复固定 L1。

Web 原始日志 `T = C:/Users/lmas1/AppData/Local/Temp/steerline-rcw-20261016-184320`，每条日志旁 `.exit` 记录退出码。没有以无测试包或 build-only 的零测试充当产品测试通过。

| 验收项 | 命令/实际选择 | 结果与原始证据 |
| --- | --- | --- |
| RCW-A01/A02/A03/A04 | `go run ./cmd/test-feedback -- ./internal/taskrunner ./internal/product/query ./internal/product/httpapi -count=1`（最终定向收尾） | 3 包、116 项含子测试通过、0 失败/跳过；`G/20261016T104749.283570900Z/summary.json` |
| RCW-A01/A02 | `go run ./cmd/test-feedback -- ./internal/app -run '^TestCandidateReadFactsAcrossViews$' -count=1` | 1 主用例 + 9 子用例通过；`G/20261016T104850.595213500Z/summary.json` |
| RCW-A01/A02 | Plan 中 SQLite 的四个精确主用例 | 4 主用例 + 1 子用例通过；`G/20261016T104420.851188400Z/summary.json`；命令、匹配及事务/交付/调度断言均在报告 |
| RCW-A03/A04 | `go run ./cmd/test-feedback -- ./internal/product/httpapi ./internal/product/applicationhttp ./internal/testkit/boundary -count=1` | 3 包、157 项通过、0 失败/跳过；`G/20261016T104401.124515600Z/summary.json`；包括不同文件的非法 helper 反例。随后 httpapi 最终版本再次纳入第一行 |
| RCW-A01/A02/A05/A06 | `npm --prefix web test -- src/features/runs/state.test.ts src/features/runs/runs.test.tsx src/features/runs/answer.test.tsx src/features/tasks/task-detail.test.tsx src/app/app.test.tsx src/app/project-switch.test.tsx src/shared/api/client.test.ts src/shared/api/queries.test.ts` | 最终 8 文件、71 项通过、0 跳过；`T/web-final-tests.log` / `.exit=0`；含新增手动/自动并发与卸载用例 |
| 全部自动项 | `go run ./cmd/test-feedback -level=L1` | complete=true、passed=true、16 组、272 项全部通过、0 失败/跳过；`G/20261016T104436.262067600Z/summary.json`；quick-owner-unified-startup 实际选择并运行新跨视图测试 |
| RCW-A03/A05/A06 | `./scripts/test-runtime-isolation.ps1 -Level L4 -Scenario runtime.dual-home-isolation` | selected-L4/controlled，passed=true、cleanup_complete=true；`G/20261016T104438.934470900Z/summary.json` 与 `scenarios/runtime.dual-home-isolation/scenario.json`；独立 temp 双 home，未创建真实 PR |
| RCW-A06 | `go build ./...`；`go run ./cmd/test-feedback -build-only` | 编译退出 0；默认测试仅编译，53 包，11 个无测试包，test_count=0；`G/20261016T104613.996418200Z/summary.json`，不能据此声称执行了全部测试 |
| RCW-A01/A05/A06 | `npm --prefix web run build` | 最终 tsc/Vite 退出 0；`T/web-final-build.log` / `.exit=0`；产物不提交 |
| 文档 | `python scripts/check_doc_links.py`；`git diff --check` | 461 Markdown、2802 local navigation links、0 errors；diff 无空白错误 |

调试期间曾出现两类已修复问题：新 guard 以文件为单位误包含同文件无关函数，改为从 registry 方法沿本包自由函数调用检查；TypeScript 的一个 history 夹具遗漏新字段，已补齐。首次 command mapping 回归也识别出历史 stop_service 夹具需独立于新提交校验，已将专用包装收归测试而保留新提交拒绝。最终定向/构建和固定边界均通过，失败日志保留，不覆盖成通过。

## 文档静态检查

设计阶段：`python scripts/check_doc_links.py` 检查 461 Markdown、2798 local navigation links、0 errors，`git diff --check` 无诊断，仅为设计证据。实施阶段：461 Markdown、2802 local navigation links、0 errors；`git diff --check` 无空白错误。
