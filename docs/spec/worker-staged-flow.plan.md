# Plan：Worker 分步执行与结构化状态推进

源码追溯：已迁出的本地源码链接固定到拆分前提交；当前模块归属见 [代码架构](../architecture/current-code-architecture.md)。

> 2026-10-12：本文保留为原实现的历史契约。当前有效业务规则和实施/测试安排已由 [统一 PR 流程 Spec](worker-pr-flow.spec.md) 与 [Plan](worker-pr-flow.plan.md) 替代；下文的候选 SHA/阶段快照比较不得继续用于当前执行，历史验收结果不改写。

任务 ID：84；任务名称：Worker 分步执行与结构化状态推进。唯一执行依据：[Spec](worker-staged-flow.spec.md)。已由用户确认纳入 [execution-map](../roadmap/execution-map.md)，尚未启动实现；无未完成硬前置，一个独立实现 PR 交付。#83 已通过 PR #247 合入；复用其隔离基线，在一期组合/24 小时验收之前交付新流程。

## 实施范围与集成

主要所有者是 execution：同一任务协调状态规则、输入/结果绑定、提交边界、双 adapter 与查询/TUI 消费，不拆出只有签名的框架前置。流程只有 `implement.execute → implement.report → publish → await_merge → delivered`，开发步骤和核验由普通代码定义；adapter 不选择下一步。

1. **最小状态与数据。** 核对 [状态规则](../../internal/execution/model/state.go)、[领域记录](../../internal/execution/model/records.go)和既有存储提交。收敛 WorkflowRun.Phase 为唯一业务状态及流程/步骤位置；Execution.Status 继续表达原生运行事实。复用 Command/Operation/Attempt、输入版本和配置冻结，增加必要来源/结果引用，不引入第二状态表、事件框架或旧格式迁移。修改存储、codec、DTO 时同步所有消费者及格式版本。
2. **步骤与三个短模板。** 将 [Prompt](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/prompt.go) 的一次性实现/发布要求拆为 Spec 中的三个短模板。输入只带固定任务引用、本步骤必要事实及用户反馈；report 绑定原执行终态/final 和紧凑快照，publish 固定候选 SHA。用现有输入冻结机制保存定义版本及实际输入，不建设模板引擎。
3. **结果改变状态。** 在 [完成接收](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/worker_completion.go)与[停止处理](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/worker_state.go)校验规范 final 来源、原步骤/输入/轮次、严格结果契约及业务产物。implement 执行终态在资格满足时触发 report；有效报告 done 才进入 publish，有效发布 done 才等待合并。原子保存接受结果、新状态和下一投递意图，然后沿 [既有生命周期](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/worker_lifecycle.go)投递。重复/迟到不推进，失败/问题留在本步骤，未知只读对账。
4. **双后端接线。** 复用 [LifecyclePort](../../internal/execution/backend/lifecycle.go)、执行目录和 app 注册。Codex/Orca adapter 只补齐规范输出来源、原输入绑定及真实能力，不解析业务 JSON。业务 step_id 与原生 business/readiness 分类分开；report 复用原会话，实际逐轮工具限制按能力采用，缺少强只读时按 Spec 核验事实变化并显示保证边界。
5. **反馈、重开与交付。** 沿原 Decision/Command 保存及投递答案，明确代码反馈回 implement、发布修复留 publish。报告有界重试只重 report；publish 不为补 JSON 重推。重开先对账，继续需现有人工授权且只继续原会话；不可恢复则人工处理，不开放新会话替代。等待合并无必答 Continue；provider 核验及唯一 TaskDelivery/后继释放复用现有端口。
6. **查询、展示及文档。** 同快照提供步骤/status 和源执行/报告事实，TUI 保留颜色、焦点和原活动正文，区分“执行失败，报告已生成”，publish 不出现虚构报告轮。同步整体架构、执行共同约束、数据/反馈/控制台设计与源码索引，注明新流程替代旧的“中心不管理业务步骤”限制，同时保持任务 Spec/Plan 负责测试/审查范围。后续 #45/#50 消费这一字段语义，不另复制转移逻辑。

复用 #83 已合入的隔离入口和 clone/exe/home，不在用户安装版或原任务库调试。与 #45/#50 的 DTO/API/展示接线先协调字段语义和实际接口版本，不静默更换已有执行的固定契约；共同宿主/存储/客户端接线串行集成，独立部分可并行。正式清单安排 #84→#45→#44，#45→#44 的硬依赖保持；一期人工活动不作为二期研发硬前置。

## 测试计划

具体行为和实时结果集中于 [#84 专属验收](../acceptance/worker-staged-flow.md)，下表固定最小选择、环境、时点及证据。`TestWorkerStage*` 是拟新增测试，实施后必须枚举并确认非零匹配，不能用零命中证明通过。

| 级别 / 组或场景 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 文档静态 / WFS-DOC | Windows、Git、Python | `python scripts/check_doc_links.py`；`python scripts/roadmap_task_id.py check`；`git diff --check` | 本次规划及实现文档完成后 | 导航存在，#84 Spec/Plan/身份一致，差异无空白错误 | 本次临时目录；acceptance 保存最小摘要 |
| L1 + 规则/提交 / WFS-A1 | Windows，无 live 开关 | `go run ./cmd/test-feedback -level=L1 -- ./internal/execution -run '^TestWorkerStage' -count=1` | 状态及提交接线完成后 | 三轮、来源绑定、失败/未知/重试、重复/迟到和事务失败的实际断言通过；原领取/唯一交付边界保持 | runner 默认临时目录的 `summary.json`；WFS-A1 |
| 定向 console / WFS-A2 | Windows，规范产品快照，无模型 | `go run ./cmd/test-feedback -- ./internal/console -run 'TestWorkerStage\|TestWaitingPR' -count=1` | 查询/TUI 修改后 | 同快照的步骤/状态显示正确，源失败不被报告覆盖，待合并无必答 Continue；保留原颜色/焦点规则 | runner 默认临时目录；WFS-A2 |
| 定向 integration / WFS-A3 | Windows，隔离 exe/home/clone，现有双后端 controlled fixture | `go run ./cmd/test-feedback -- ./tests/integration -run '^TestWorkerStage' -count=1` | 双 adapter 和恢复接线完成后，串行 | 三轮真实接线；输入/原生 purpose 与规范 final 来源一致；重开、缺能力、未知操作不盲发；partial publish/JSON 错不追加报告或重推 | runner 临时目录，原生回执/输入绑定；WFS-A3 |
| L3 delivery / WFS-A4 | Windows live，本地 Codex、本机 Orca；隔离 loopback Git/PR provider，沿当前 E2E 权限与模型 | `go run ./cmd/test-feedback -level=L3 -suite=delivery -env=live '-scenario=delivery.local-codex,delivery.orca-codex'` | controlled 通过后 | 每个后端一条真实 implement/report/publish 链，步骤归属/usage 正确，报告无业务写入，PR head 等于固定候选；交付及资源清理正确 | runner 临时目录及每个场景证据；WFS-A4 |
| 人工 / WFS-M1 | 一期安装基线，参与人/窗口待安排 | 按专属验收及一期 P1A1-M 审阅、反馈和合并 | 自动能力完成后，可后置至一期组合验收 | 人能辨认步骤/阻断并按正确入口处理；真实合并后唯一交付及后继资格正确 | acceptance 的人工结果及现场证据；不占 Task ID |

优先更新既有 delivery 场景的最小任务材料和步骤/usage 断言：将 fixture 的候选提交与推送/PR 动作分开，不能靠旧一次性发布脚本跳过新步骤。重开和未知分支主要在 controlled 及规则测试覆盖，不在每条 live 链重复故障矩阵。只有发现现有环境无法提供所需事实时，才沿 [E2E 扩展约定](../../tests/e2e/README.md)补小能力并同步注册/说明；不新增完整模拟框架或第三 adapter。

不默认跑 Full、完整 L2、候选矩阵或额外 live recovery：本次选择 L1、受影响接线和双 live 正常链，恢复边界由 controlled 定向覆盖。实际改动新增风险时补对应最小场景并记录理由。报告工具权限与真实审核按触发事实记录，未触发不冒充通过；环境不足、真实失败或未知动作不能后置豁免。

## 交接与纳入执行

交接源码/二进制来源、定义/格式版本、双后端能力和本次自动结果给[一期验收](../acceptance/phase1.md)。一期活动固定包含 #83/#84 成果的安装构建，再组织组合、24 小时及同构建恢复；不把本任务的短时 live 结果算为一期签收。

用户已确认 #84 纳入 execution-map；本轮只更新清单和文档，不启动产品实现、不创建外部 Task/DAG、不安装新构建。实际启动时固定已提交的 Spec/Plan commit/路径，并核对最新主干、状态与前置；不要求外部中心准入。实现者按本计划执行自动验收、登记专属 acceptance 并同步 execution-map 汇总；人工后置项保留实际状态。文件编辑不重定向在途执行。
