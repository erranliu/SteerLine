# Plan：本机 Codex app-server 执行迁移

适用说明（2026-09-30）：本 Plan 的独立进程存活及重接验收属于历史范围；当前本地 worker 由服务管理，停服后的人工继续按[新 Plan](worker-service-recovery.plan.md)实施。

执行依据：[Spec](local-codex-app-server.spec.md)。按以下顺序实施，一个实现 PR 交付。实现及 A/B/C 验收已完成；当前环境缺少定向 race detector 工具链，记录在 [验收状态](../validation/local-codex-app-server/status.md)。

1. **验证最小执行与恢复链。** 在目标 Windows/Codex 版本核对生成 schema。已验证建 thread、启动 turn、Default 模式 `steerline_ask` 原生工具请求、WebSocket 断开重接后由独立连接回复原请求并完成原 turn。第一条 turn 待答时第二个独立 app-server 已完成，第一条仍保持 `inProgress`。内置 `request_user_input` 在 Default 模式不可用，按 Spec 使用 `dynamicTools`。后续在独立宿主上完成 SteerLine 服务级双任务及宿主进程退出后的重启验收。

2. **接入现有身份与 Operation。** 在 Execution 中区分 Command ID、原生 thread/turn 和宿主代次，给 Decision 保存原生请求绑定；新增本地协议并提升受影响的持久格式。复用 Operation 的意图、回执与未知状态，不另建发送状态机。按探针结果选择能证明发送归属的证据；不足时保留未知，不预先强制给所有 prompt 加消息标记。按验收 B、C 补发送幂等和旧格式拒绝测试。

3. **替换执行并保留并行。** 改造 [本机启动](../../internal/execution/adapters/codex/lifecycle.go)、[worker-flow 服务](../../internal/execution/worker_start.go)和[worker 路由](../../internal/execution/service_workers.go)，使用原生 thread/turn 驱动执行。每个 Execution 独立处理阻塞 I/O，回传固定身份及冻结配置；中心沿用单写者提交。移除本地 `exec`、进程退出和问号判断的执行接线，复用[历史读取](../../internal/execution/adapters/codex/history.go)及会话持久化。按验收 B 检查异常隔离、第三任务领取与自动单活动策略。

4. **接入统一决策。** 扩展现有 Decision 请求/答案 DTO 以保留原生多题和请求身份；查询与运行投影不得覆盖同时待答的请求。两类反馈共用 `answer_decision`，分别回复原请求或启动原 thread 的新 turn。更新控制台提示、送达进度和失效反馈处理；等待回答时保留 active turn。恢复采用有界逐执行核验，核验结束后允许其他合格任务继续。执行验收 A 的一条双任务真机闭环，其余输入和故障分支按 B 自动验证。

5. **回归与交付。** A–C 三组验收按 Spec 记录证据；C 复用现有 PR/TaskDelivery 回归，未修改交付逻辑时不另设人工合并验收。实现审计已完成完整 Go 测试，主线同步及补验按当前 AGENTS.md 使用最小入口与定向测试，并运行 `go vet` 和 Windows/Linux 构建；仅在新改动或失败影响结论时补验。证据保存在 `docs/validation/local-codex-app-server/`，标明真实、模拟和未测范围。更新 [数据契约](../data-contract.md)、[决策说明](../decisions.md)、[会话事件说明](../conversation-events.md)和受影响运行说明；执行 `git diff --check`。本次实现可合并并使用新本地路径；仅 race detector 的环境补验留在第一阶段残留。


## 测试计划（本次文档整理）

本次只修正文档导航及重构后的源码定位，不改变原实现或待补验收要求；原验证结果和限制见上述验证目录。

| 级别 / 组 ID | 环境 | 命令 | 执行时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- | --- |
| 文档静态 / doc-links | 当前仓库、Python 3 | `python docs/validation/document-cleanup-2026-10-06/check_docs.py` | 归档及引用修正后 | 导航无缺失目标，活动表一致，Spec/Plan 成对归档 | `docs/validation/document-cleanup-2026-10-06/README.md` |
| 文档静态 / task-identities | 当前 Git 仓库、Python 3 | `python scripts/roadmap_task_id.py check`；`git diff --check` | 交付前 | 任务 ID、名称和 Spec/Plan 一致，无空白错误 | 同上 |

不选择 L1/L2/L3、Full 或原生真机组：本次没有修改实现、测试场景、协议或平台接线，运行它们不能补齐文档导航证据；后续实际实现或补验仍按原 Spec 的范围选择。无需新增场景或测试环境能力。
