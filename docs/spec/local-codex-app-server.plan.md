# Plan：本机 Codex app-server 执行迁移

执行依据：[Spec](local-codex-app-server.spec.md)。按以下顺序实施，一个实现 PR 交付。

1. **验证最小执行与恢复链。** 在目标 Windows/Codex 版本核对生成 schema。已验证建 thread、启动 turn、Default 模式 `steerline_ask` 原生工具请求、WebSocket 断开重接后由独立连接回复原请求并完成原 turn。第一条 turn 待答时第二个独立 app-server 已完成，第一条仍保持 `inProgress`。内置 `request_user_input` 在 Default 模式不可用，按 Spec 使用 `dynamicTools`；当前证据表明无需额外宿主。SteerLine 服务退出后的双任务真机闭环仍需在可脱离 Job Object 的宿主验证。

2. **接入现有身份与 Operation。** 在 Execution 中区分 Command ID、原生 thread/turn 和宿主代次，给 Decision 保存原生请求绑定；新增本地协议并提升受影响的持久格式。复用 Operation 的意图、回执与未知状态，不另建发送状态机。按探针结果选择能证明发送归属的证据；不足时保留未知，不预先强制给所有 prompt 加消息标记。按验收 B、C 补发送幂等和旧格式拒绝测试。

3. **替换执行并保留并行。** 改造 [本机启动](../../internal/steerline/worker_flow_local.go)、[worker-flow 服务](../../internal/steerline/worker_flow_service.go)和[worker 路由](../../internal/steerline/service_workers.go)，使用原生 thread/turn 驱动执行。每个 Execution 独立处理阻塞 I/O，回传固定身份及冻结配置；中心沿用单写者提交。移除本地 `exec`、进程退出和问号判断的执行接线，复用[历史读取](../../internal/steerline/codex_history.go)及会话持久化。按验收 B 检查异常隔离、第三任务领取与自动单活动策略。

4. **接入统一决策。** 扩展现有 Decision 请求/答案 DTO 以保留原生多题和请求身份；查询与运行投影不得覆盖同时待答的请求。两类反馈共用 `answer_decision`，分别回复原请求或启动原 thread 的新 turn。更新控制台提示、送达进度和失效反馈处理；等待回答时保留 active turn。恢复采用有界逐执行核验，核验结束后允许其他合格任务继续。执行验收 A 的一条双任务真机闭环，其余输入和故障分支按 B 自动验证。

5. **回归与交付。** 完成 A–C 三组验收；C 复用现有 PR/TaskDelivery 回归，未修改交付逻辑时不另设人工合并验收。先做定向测试，最终候选再运行完整 Go 测试、`go vet` 和 Windows/Linux 构建；真机闭环已通过后，仅在新改动或失败影响其结论时补验。证据保存在 `docs/validation/local-codex-app-server/`，标明真实、模拟和未测范围。更新 [数据契约](../data-contract.md)、[决策说明](../decisions.md)、[会话事件说明](../conversation-events.md)和受影响运行说明；执行 `git diff --check`。所需能力和验收未通过前不启用新本地路径。
