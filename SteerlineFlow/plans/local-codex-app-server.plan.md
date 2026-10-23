# Plan：本机 Codex app-server 执行迁移

源码追溯：已迁出的本地源码链接固定到拆分前提交；当前模块归属见 [代码架构](../../docs/architecture/current-code-architecture.md)。

适用说明（2026-09-30）：本 Plan 的独立进程存活及重接验收属于历史范围；当前本地 worker 由服务管理，停服后的人工继续按[新 Plan](worker-service-recovery.plan.md)实施。

执行依据：[Spec](../spec/local-codex-app-server.spec.md)。按以下顺序实施，一个实现 PR 交付。实现及 A/B/C 验收已完成；当前环境缺少定向 race detector 工具链，记录在 [验收状态](../testing/acceptance/local-codex-app-server.md)。

1. **验证最小执行与恢复链。** 在目标 Windows/Codex 版本核对生成 schema。已验证建 thread、启动 turn、Default 模式 `steerline_ask` 原生工具请求、WebSocket 断开重接后由独立连接回复原请求并完成原 turn。第一条 turn 待答时第二个独立 app-server 已完成，第一条仍保持 `inProgress`。内置 `request_user_input` 在 Default 模式不可用，按 Spec 使用 `dynamicTools`。后续在独立宿主上完成 SteerLine 服务级双任务及宿主进程退出后的重启验收。

2. **接入现有身份与 Operation。** 在 Execution 中区分 Command ID、原生 thread/turn 和宿主代次，给 Decision 保存原生请求绑定；新增本地协议并提升受影响的持久格式。复用 Operation 的意图、回执与未知状态，不另建发送状态机。按探针结果选择能证明发送归属的证据；不足时保留未知，不预先强制给所有 prompt 加消息标记。按验收 B、C 补发送幂等和旧格式拒绝测试。

3. **替换执行并保留并行。** 改造 [本机启动](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/adapters/codex/lifecycle.go)、[worker-flow 服务](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/worker_start.go)和[worker 路由](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/service_workers.go)，使用原生 thread/turn 驱动执行。每个 Execution 独立处理阻塞 I/O，回传固定身份及冻结配置；中心沿用单写者提交。移除本地 `exec`、进程退出和问号判断的执行接线，复用[历史读取](https://github.com/erranliu/SteerLine/blob/d20a72284705b4daac476d4fa5770151dca03e56/internal/execution/adapters/codex/history.go)及会话持久化。按验收 B 检查异常隔离、第三任务领取与自动单活动策略。

4. **接入统一决策。** 扩展现有 Decision 请求/答案 DTO 以保留原生多题和请求身份；查询与运行投影不得覆盖同时待答的请求。两类反馈共用 `answer_decision`，分别回复原请求或启动原 thread 的新 turn。更新控制台提示、送达进度和失效反馈处理；等待回答时保留 active turn。恢复采用有界逐执行核验，核验结束后允许其他合格任务继续。执行验收 A 的一条双任务真机闭环，其余输入和故障分支按 B 自动验证。

5. **回归与交付。** A–C 三组验收按 Spec 记录证据；C 复用现有 PR/TaskDelivery 回归，未修改交付逻辑时不另设人工合并验收。实现审计已完成完整 Go 测试，主线同步及补验按当前 AGENTS.md 使用最小入口与定向测试，并运行 `go vet` 和 Windows/Linux 构建；仅在新改动或失败影响结论时补验。原始证据保存在本次新临时目录，最小结果登记于任务专属验收文档，标明真实、模拟和未测范围。更新 [数据契约](../../docs/product-design/data-model.md)、[决策说明](../../docs/product-design/decisions.md)、[会话事件说明](../../docs/product-design/conversation-events.md)和受影响运行说明；执行 `git diff --check`。本次实现可合并并使用新本地路径；仅 race detector 的环境补验留在第一阶段残留。


## 测试计划（历史文档整理，非产品验收）

本次只修正文档导航及重构后的源码定位，不改变原实现或待补验收要求；原验证结果和限制见上述验证目录。

| 级别 / 组 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 文档静态 / doc-links | 当前仓库、Python 3 | `python scripts/check_doc_links.py` | 产品设计迁移及引用修正后 | 本地导航无缺失目标 | `SteerlineFlow/testing/validation/product-design-layout-2026-10-07/README.md` |
| 文档静态 / task-identities | 当前 Git 仓库、Python 3 | `python scripts/roadmap_task_id.py check`；`git diff --check` | 交付前 | 任务 ID、名称和 Spec/Plan 一致，无空白错误 | 同上 |

不选择 L1/L2/L3、Full 或原生真机组：本次没有修改实现、测试场景、协议或平台接线，运行它们不能补齐文档导航证据；后续实际实现或补验仍按原 Spec 的范围选择。无需新增场景或测试环境能力。

## 测试计划（当前自动验收与补验）

任务专属记录：[app-server 验收](../testing/acceptance/local-codex-app-server.md)。A/B/C 已验结果保持原日期/版本，旧独立存活语义不重新作为当前自动验收要求；当前必须落实的是 AS-RACE-A，不能只引用文档整理的静态检查。

| 验收项 | 级别 / 组 / 场景 | 环境 / 命令 | 执行时点 | 通过条件 | 证据与登记 |
| --- | --- | --- | --- | --- | --- |
| AS-RACE-A（并行/恢复/回复） | 定向 Go / app-server race | Windows、已可用 C/CGO 工具链，CGO_ENABLED=1；`go run ./cmd/test-feedback -- ./internal/execution ./internal/execution/adapters/codex -race -run '^Test(AppServer|CandidateResumeErrorDoesNotStarveSibling)' -count=1` | 工具链预检通过后补验；相关并发逻辑改变时定向重验 | 实际覆盖并行/恢复/回复断言，非零命中/无 skip、race 报告无竞争且运行完整；若必要分支未覆盖则先补最小用例 | 默认新临时目录 summary.json/日志；专属文档逐项记录实际源码/命令与 AS-RACE-A 状态 |
| A/B/C 原通过范围 | 历史复用 / 有影响才重验 | 核对专属文档固定版本/原始证据及旧宿主语义；仅新实现影响相应范围时固定该组实际命令 | 并发补验前核对；不默认重跑模型/PR 链 | 证据与声称范围一致，未知/旧宿主不冒充当前通过 | 本任务记录中的原来源与复用说明 |

准备步骤：在测试外确认 C/CGO 能力，不自动安装/提升；所需环境缺失就保持待验，不能归类为人工后置。用例范围不足时在原 owner 补最小场景，既有实际测试不能只编译后当作 race 通过。执行后恢复本次环境覆盖，在 acceptance 写结果并同步 execution-map；实时结果不写入 Plan。当前未单列人工验收，不默认 Full/L2/L3 或重复原生历史链。
