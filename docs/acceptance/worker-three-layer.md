# Worker 三层职责重构验收

唯一归属：[Worker 三层职责重构 Spec](../spec/worker-three-layer.spec.md)与[Plan](../spec/worker-three-layer.plan.md)。普通开发，无 Task ID，不覆盖后续初始化重试任务。状态：三层代码已接通，TL-A01–TL-A07 约定自动验收全部通过；G1–G3、六个 controlled 场景及关闭查询计数补验均有本轮证据。旧任务证据不计为本次通过。

## 自动验收项

步骤与命令 G1–G3/R1–R3 以 [Plan 测试计划](../spec/worker-three-layer.plan.md#测试计划) 为准。每项执行后登记实际源码/文档版本、命令、日期、证据路径、覆盖分支和结论；缺入口、零匹配、意外 skip、证据或 cleanup 缺失不算通过。

| ID | 验收步骤和通过条件 | 命令映射 | 当前状态与证据 |
| --- | --- | --- | --- |
| TL-A01 | 运行三层依赖/符号/传递依赖及负例；检查中层不接业务/Service、Workflow 不读原生字段、adapter 无中心写权限 | G1；结构源码核对 | 通过；G1 三层规则/边界 33 项及 L1，依赖/符号/传递负例有效，结构核对无 forbidden 依赖 |
| TL-A02 | 用纯 adapter fixture 验证准备/发送/观察/恢复规则及部分成功、未知、身份、期限；Workflow fixture 无模拟 thread/terminal | G1、G3 | 通过；G1＋G3，显式适用性、部分成功/未知、期限/身份/授权纯中层断言 |
| TL-A03 | 组合提交成功/回滚/重放、意图前置、结果与后继一次提交、新旧 Attempt/原绑定、迟到与兄弟隔离 | G2、R1、R2 | 通过；G2 最终 89 项＋阶段/候选 47 项；R1/R2 原身份与唯一交付；原子成功/回滚/重放 |
| TL-A04 | 两 adapter 的正常/错误契约和实际中层接线；Orca local 重接、environment 未知请求对账，原生身份/结果保持，零重发/补 Enter/屏幕门槛 | G3、R3 | 通过；G3 108 项及 R3 两场景，必要/跳过门槛与原终端/原请求零重发；真机 opt-in 未运行 |
| TL-A05 | 原问题答案、业务反馈、服务换代及停服/原会话恢复；授权只消费一次，未知/旧身份/失效目标拒绝 | G2、G3、R2、R3 | 通过；G2/G3/R2/R3，原问题/反馈、版本授权、旧写者与服务换代 |
| TL-A06 | 原 implement/report source、格式次数、publish/固定 PR、合并前后依赖、唯一 Delivery、closed 后零查询/新输入 | G2、R1、R2 | 通过；G2/R1/R2 主体、关闭后的新 actor 计数及 R1 实际查询计数通过，原输入/唯一 Delivery 保持 |
| TL-A07 | 检查旧路径/转发删除、app/查询/fixture/driver 接线，公开/持久行为和文档归属保持；完整最小回归非零匹配 | G1、R1、R3；结构源码核对 | 通过；G1/R1/R3，旧 planner/worker_resume/混合上下文删除，实际组装与文档接线核对 |

## 环境与证据

- G1–G3：现有 Windows/Go 定向 runner、隔离测试 fixture及受控进程，无模型或安装状态；新包用例/边界负例由 P1–P4 补齐。
- R1–R3：已注册的六个 L3 controlled 场景，真实产品、独立 Git/SQLite/home、受控 app-server/Orca native state 与 loopback PR provider；不访问安装环境或真实 merge 项目。
- 原始产物保留在 runner 临时目录；仓库仅登记最小摘要，不复制认证、完整会话过程或业务数据库。
- 记录 input/request/turn/Attempt/资源绑定与计数、事务前后事实及 owned cleanup。各场景可复用既有证据格式，不能手写成功回执冒充原生事实。
- controlled 覆盖不声明本轮真实模型、真实远端、硬件断电或新的代理恢复能力已验。

## 人工验收与阻塞范围

本次未设强制人工项：公开 UI/命令与业务行为保持。发现需要改变公开行为时，先明确新的验证安排；不能将未知问题登记为“人工后置”以绕过必须通过的自动项。真实原生接口变化的验证要求见 Plan。

## 文档静态检查（D1 设计阶段记录）

以下是实施前 D1 记录，仅证明设计文档一致性。当前实施结果见下一节，静态检查不替代产品验收。

2026-10-14 文档生成后执行 D1：`python scripts/check_doc_links.py` 通过（379 份 Markdown、2305 个本地链接、0 错误）；`python scripts/roadmap_task_id.py check` 通过（92 个身份文件、0 个草稿行、1 个正式行，未修改规划）；`git diff --check` 通过。

同时静态核对 TL-A01–TL-A07 均映射到 Plan 的测试表，六个 R1–R3 场景均已登记且为 controlled；Spec/Plan/验收正文的编号、索引和空白检查通过。当时 G1 新包与 G2 组合提交用例尚待实施，未运行 Go/L3；该历史记录不代表当前验收状态。

## 实施与验收结果

2026-10-14 已实现三层职责、显式适用性门槛与原操作保护；产品源码候选 `80a95337`，controlled 原生身份补齐 `44f54055`；关闭查询计数补验见后续提交。主干 rebase 基线 `1b7468b0`。本轮无模型、真机、真实 PR 或安装现场操作。

下表每个 runner 的 `summary.json` 保存实际命令、source_commit 与工作树差异；G1 适用性定向基于 `4a6034f0`＋待提交修复，L1/G3/阶段组基于 `80a95337`，最终 G2/R3 基于 `44f54055`。同轮场景失败不外推为组通过。

当前证据（runner 原始目录均在 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/`）：

| 范围 | 报告目录 | 结果与边界 |
| --- | --- | --- |
| 三层规则/边界与两路适用性 | `20261014T081630.170486400Z` | 33 项通过、0 skip；必要/跳过/不支持/未知、部分 thread 绑定、原请求不重发 |
| 原操作模式与层内规则/静态停服守卫 | `20261014T080838.454900200Z` | 18 项通过；未知/accepted/已结算操作不能被调用方改投 Execute，pending 意图零副作用 |
| L1 固定范围＋层内/边界 | `20261014T082921.120921400Z` | passed=true，完整固定范围及追加规则通过 |
| 两 adapter 定向 | `20261014T082932.594594800Z` | 108 项通过，1 项显式 opt-in 真机场景 `TestOrcaNativeStateLiveLifecycle` 未启用而跳过；不计为真机验收 |
| 阶段/候选停止与启动保留 | `20261014T084614.678143000Z` | 47 项通过、0 skip；受影响业务和候选控制分支 |
| 最终组合提交/恢复/停服/Prompt/反馈 | `20261014T090307.377391300Z` | 89 项通过、0 skip；包含原子提交成功/回滚/重放、新旧 Attempt、原意图、未知反馈、旧身份与服务换代 |
| 组合提交/阶段/Prompt/生命周期反馈 | `20261014T073047.904946800Z` | 29 项通过，1 个旧 PR 替换断言失败；已按当前原 PR 复用契约修正并单测通过，最终 89 项定向复跑已通过 |
| 提交/恢复/停服 | `20261014T073706.549060600Z` | 59 项通过，1 个静态路径引用失败；已改为新 owner 路径并通过静态停服守卫，最终 89 项定向复跑已通过 |
| closed 查询/新 actor 初始化补验 | `20261014T101931.925859500Z` | G2 新用例通过；open＋正式完成命令实际查询 provider 共 2 次，原 actor/新 Ready epoch 初始化及后续 poll 后仍为 2；1 个 Delivery、原候选/Attempt/输入事实不变 |
| delivery.basic 最终含关闭查询计数 | `20261014T102543.072896000Z` | passed=true、cleanup_complete=true、context_error 为 nil；closed-poll-provider-queries.json：before=12、after=12、additional_queries=0，dispatches=3、sessions=1、deliveries=1 |
| recovery.waiting-merge-restart | `20261014T075339.424960400Z` | 独立复跑 passed=true；前次在交付后的末尾核验超时，未增加期限 |
| recovery.codex-resume | `20261014T083915.139044300Z` | 独立复跑 passed=true，沿原场景期限、同 thread 新 Attempt，cleanup 完整 |
| R3：terminal-local-reconnect、terminal-environment-unknown-send | `20261014T091728.743894500Z` | 两场景 passed=true、cleanup_complete=true、context_error 为 nil；原终端重接/原请求对账 |
| recovery.report-source-changed | `20261014T080532.271092200Z/scenarios/recovery.report-source-changed` | passed=true、cleanup_complete=true；同轮 codex-resume 失败不能算该组全通过 |

失败与修复记录：`recovery.codex-resume` 首轮因后台核验版本变化而明确拒绝旧答案，产品零投递；驱动现仅对提交前明确 stale_recovery 拒绝有限刷新同执行/Attempt 的合格决策。第二轮同 thread 恢复及四轮路由已到 await_merge，但最终状态读取达到场景期限；cleanup 首次文件占用，随后核对记录 PID 均已退出、临时目录已释放。独立复验已沿原期限通过；未将业务完成快照替代场景通过。阶段/候选定向已通过。Orca 首次创建被当前原生身份守卫拒绝：旧 controlled 响应缺 instanceId；已补齐同一 create/show 身份，沿原期限复验，未放宽产品门槛。后一次 R3 与重型 G2 并行时在 task validate/create 前达到期限，尚未到 Orca 协议；G2 结束后独立复验两场景均沿原期限通过。

适用性专项验收（归属 TL-A02/TL-A04）：中层表驱动覆盖 `not_applicable`、`unsupported`、`unverified`、零值与声明/事实冲突；两 adapter 定向用例覆盖各自必要流程和 Codex 跳过 Orca 专属启动/初始化/连接机制。原请求未知只对账，不能由机制跳过扩大权限。证据见三层 33 项与 G3 表行；两条受控路径 R3 均 passed=true 且 cleanup_complete=true。

关闭后查询计数补验：`20261014T094307.375464300Z` 已实际记录 closed poll 的 before=7、after=7、3 次输入、1 个会话、1 个 Delivery，但末尾重启 readiness 达到原 5 分钟期限，整场不通过；cleanup 首次文件占用，随后核对本场记录的 PID 36852/60844/64660 均已退出，临时目录已释放。`20261014T095213.688723000Z` 在 task create 的独立 CLI 期限失败，未进入业务执行，cleanup_complete=true；这两次结果均不计为关闭重启补验通过。第三次 `20261014T095620.770132200Z` 同样在末尾整进程重启 readiness 达到 5 分钟期限，cleanup_complete=true。未把这些超时认定为成功，也未扩大时限。测试 Plan 明确调整为 R1 正常 closed poll 计数＋G2 原 SQLite 重建 actor 的 BeginActor/Initialize/poll 门槛；整进程 preflight 不重复串入 delivery.basic，已有服务重启链路仍由 R1/R2 验证。G2 已通过。`20261014T101652.973976700Z` 的 R1 复跑在 await_merge 等待达到期限，cleanup_complete=true；最终快照显示三步结束并绑定原 PR，但落库晚于截止时间，没有执行新增 closed 计数断言，仍按整场失败记录。所有定向测试结束后独立复跑 `20261014T102543.072896000Z` 沿原期限通过，cleanup 完整，实际 closed 查询计数 12→12。

G2 关闭补验的新夹具最初分别被 PR/Git 目标不一致和缺原会话拒绝；改用既有正确 PR command fixture、原 session/policy 绑定，再通过实际 provider 查询与正式 CompleteNativeTask 提交 Delivery。未放宽产品资格守卫；最终新 actor 的 ServiceInstanceID 不同且 Ready=true，零额外查询与业务输入断言通过。

最终 D1：`python scripts/check_doc_links.py` 通过（387 份 Markdown、2339 个本地链接、0 错误）；`python scripts/roadmap_task_id.py check` 通过（92 个身份文件、0 个草稿行、1 个正式行，未修改规划）；整分支 `git diff 1b7468b0 --check` 通过。变更 Go 文件统一 gofmt；不执行 Full/live/native 或安装更新。

收尾再 Rebase：主干在实施期间新增 PR #309–#311，最终重放至 `ab814da9`；实现候选 `715b011f`。冲突仅文档，保留主干 HTTP Ports、命令边界及部署默认。前述报告保留各自重放前真实 source_commit，不能改写为新候选全量通过；新候选 L1＋三层边界/规则、两 adapter 适用性、原子提交及 closed 新 actor 定向复验通过：`20261014T103820.388440300Z`，source_commit=`715b011f`，运行输入 clean；固定 L1 与追加 targeted 37 项均通过、0 skip（所有组共 579 项）。重放只合并文档并接受已由主干实现的 HTTP Ports 组合，未改变 Worker 原生算法，故 R1–R3 不扩大复跑；其实际已验源码/差异保留于原报告。

二次 rebase 的最终 D1：403 份 Markdown、2526 个本地链接、0 错误；编号检查 97 个身份文件、4 个草稿行、3 个正式行、next ID=97（主干新增任务，当前分支未修改规划）；`git diff origin/main --check` 通过，`go list ./...` 仍为 51 包。修正新主干对已搬迁结果解析文件的链接，保留 #94 任务状态并提示后续重核交叉调查输入。
