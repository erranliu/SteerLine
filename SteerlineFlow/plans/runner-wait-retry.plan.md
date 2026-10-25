# Plan：Runner 准备查询等待与执行页展示

日期：2026-10-25。状态：调用方已实施，Runner #57 已合入；正式构建的远端 Orca 真机联验、原 Run 停止/宿主结算及最终回归通过，已提交 [PR #393](https://github.com/erranliu/SteerLine/pull/393)，不自动合并。历史告警、失败轮和未验条件见[Spec](../spec/runner-wait-retry.spec.md)及[专属验收](../testing/acceptance/runner-wait-retry.md)。不执行 roadmap，不恢复 Task 18，不部署。

## 1. 依赖与实施顺序

调查基线：SteerLine `9607fb0099dfc00d9f3de35429d4c43fcdc80407`、Runner `af5e244ddae05d4c19069852d56c5a74a68ab1ce`。共同权威是 Runner `docs/spec/orca-cli-wait-retry.spec.md`，实际开发固定文档/构建/schema 身份。

Runner 独立实现/验收，不需要本仓。SteerLine 可据公开 fixture 开发，真实接入再核验已发布 capability/构建/目标资格。只消费两项准备读取的最小 execution_wait，不建设调用方重试器，不接回原稿已删除的通用调度/副作用窗口/重复状态字段。

## 2. 实施步骤

### P0：契约与最小链路

- 固定 queue/calling/backoff/exhausted/null、缺能力、坏组合、错 Run/倒退 revision 正反例；fixture 取 Runner 权威，无另一套预算。
- 核对 runnercontract/client→bridge/原投影→product API→Web 链路，确认最小字段改动；不迁移旧开发现场。

### P1：原投影与公开 API

- 在原 ProjectRuntime 能力检查中增加 execution_wait 前置；缺能力明确拒绝本项新 Orca 接入，不热切实例或扩大其它 adapter 门槛。
- 严格解码/验证并沿原 observe 同快照/CAS 保存字段/事件和 controls/phase。坏对象保留原有效快照，经原查询错误通道标不可核实。
- 仅在 runner_delivery 返回完整对象/null；使用原 HTTP/query 路由，无 wait 表、顶层复制状态或新执行端点。

退出：A01/A02 后端反例通过，重复 GET 不推进业务或重置预算。

### P2：复用执行摘要与活动

- 以 query/stage/reason_code 显示标题、原起点、次数和实际 next_attempt_at；calling 不证明模型开始。
- 时间显示复用现有组件；仅有可信采样基准时显示近似倒计时，无基准显示绝对时间。到期等待新快照，陈旧时保留内容并停止确定倒计时。
- 复用查询缓存/可见性作用域，项目/候选切换取消旧查询；本地文字计时不额外请求网络。
- 消费原 execution_wait_changed 活动及 null 清除，重投幂等。未知副作用和其它 blocker 保留原 UI，自动等待不新建恢复 Decision。
- 人工继续仍原 controls/Decision/版本/outbox；候选退出与 Runner stop 保持区别，不新增取消等待入口。

退出：A03/A04 通过，旧未知/人工问题/停止组件无平行状态机。

### P3：恢复与本仓验证

- 宿主/Web 重启只读原候选/Run/预算，零自动 submit/reply/resume；事件与旧响应/实例隔离正确。
- 新增计划 L3 controlled `recovery.runner-wait-retry`，真实本仓服务/Git/SQLite+公开 Runner fixture；实施时同步 suites.json、执行器、test-list、L3 列表和证据断言，不注册空入口。
- 使用固定真实 Runner 构建与相应 Orca 资格，在本任务新独立调试包联验 >15 秒竞争→自动恢复、取消与清理；此步只属于 SteerLine 验收，不是 Runner 完成前置。
- 更新 worker-lifecycle 增量、原 DTO/接入说明与实际资格；Runner 原生策略不复制到调用方。

## 3. 验证计划

按[测试指南](../testing.md)与[验收流程](../testing/acceptance/README.md)执行以下测试；实际命令、失败与补测证据登记到专属验收。

| 验收 | 环境/入口 | 时点、通过条件 |
| --- | --- | --- |
| A01/A02/A05 后端 | 定向 isolated：`go run ./cmd/test-feedback -out <独立diagnostics目录> -- ./internal/runnercontract ./internal/runnerclient ./internal/runnerbridge ./internal/product/... -count=1`，实际存储/app 改动补 owner 包 | P1/P3；schema/原绑定/CAS/事件、坏投影和原请求零重发；不冒充完整 L1 |
| A03/A04 | `./scripts/test-web.ps1 -EvidenceDir <新绝对目录>` | P2；build/client/server tests，文案/时间/陈旧/切换/原控件资格 |
| A02/A04/A05 | 注册后 `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled -scenario=recovery.runner-wait-retry -out <新目录>` | P3；原候选恢复/耗尽/停止/重启、零多余输入、事件和清理 |
| 固定回归 | `go run ./cmd/test-feedback -level=L1 -out <新L1目录>`；`go run ./cmd/test-feedback -level=L2 -out <新L2目录>` | 实现修改后与提交前，l1.minimum/smoke.delivery 及原唯一交付语义通过 |
| A06 | 按[路径清单](../paths.md)/[调试指南](../../docs/runtime-install-debug.md)生成新包，固定实际身份/命令后真实 API/Web 联验 | Runner 已发布后；原候选 >15 秒等待自动恢复，无重复输入，资源可核实清理 |

输出 scope `C:/SteerlineDebug/wt-507e-runner-wait-retry/`，各级 test-results/diagnostics 使用新叶目录；包身份读取生成器返回值。真实 CLI cache 竞争与同用户其它原生测试串行或证明隔离。失败/补验分目录，原报告路径、两仓 source/构建、实际命令/选择和清理写专属验收。

P0–P3 已完成：严格公开对象/null 解码、schema/同快照能力准入、原投影/CAS/活动与公开 DTO、列表/详情和单调文字计时；controlled 场景已登记，A06 在固定正式 Runner 与远端 Orca 的独立包通过。真实链只取得本次远端目标资格，不代替上游 local/remote 全矩阵。新 Orca 的等待能力检查放在 profile 展示与候选预留前，不让缺能力阻断其它 adapter 或原候选的读取/停止。

合入续跑固定 Runner `5df57b69e98382abcd63676b04a73079fd939fa6` 的独立源码，直接复制公开 schema 作为测试夹具，删除手写简化 schema。09:20 因上游安全隔离记录保留真实测试待验；后续用户授权控制变量 build 排查，正常正式构建扫描、只读及原触发路径受控复验通过，当前可继续 P3 的新隔离包真机测试。历史告警和有效排除项未核实的限制继续单列，防护设置保持不变；再次拦截即停止并保留失败，不变形受检程序强行执行。通过真机与最终回归后，按用户授权 commit/push 并提 SteerLine PR，不自动合并。

复用原 Task 取消资格：无已核实 PR 绑定时仍返回 `merge_state_unknown`；等待状态不绕过取消保护，也不提供新的取消等待入口。Task 终态后的停止沿原义务，受控场景分别核验唯一交付的停止和测试资源清理。人工文案体验可后置但不替代已知失败；不要求先修改 Runner 来满足本仓私有展示需求。

真机补验另明确 Orca 调试 fixture 无需项目清单自动项，仍核对文件字节与提交；这是测试模板说明，不改变产品测试门槛。最终包 `dev-20261025-132926-1883879d-dirty` 与逐轮失败、公共停止回执、Edge trace、独立 Git 产物和清理证明均见专属验收。用户已取消定时，后续不恢复 heartbeat。
