# PR 5 Plan：人工决策与恢复闭环

任务 ID：3；任务名称：人工决策与恢复闭环。

依据：[Spec](pr-05-intervention.spec.md)。依赖：PR 2、4A、4B、4C 审核合并；从最新 origin/main 实施，复用已落地的权威存储与接口。

## 实施步骤

1. **先统一答案发布。** 重构 `WriteAnswer` / `AnswerDecision`，加入明确 decision_id、first-wins 跨进程互斥和相同内容幂等；扩展 free-text 语义。新请求使用按 ID 分目录的路径，既有 pending 路径保持原值。更新 `docs/decisions.md` 对旧覆盖规则的说明。
2. **接入在线答案动作。** 为 answer_decision 加 handler 与能力声明；完成回执定义为答案被接受，后续 `applyDecisions` 单独推进。复用相同校验/发布函数支持明确离线的 answer；已有相同答案作为可恢复证据，不能重发 continuation 来补回执。
3. **完善续跑守卫与证明。** 把单槽/unknown/stop 条件纳入 applyDecisions，沿用 4A 的固定 ref、Operation 与能力，保留 resuming 先落盘及身份核验。实现 saved/accepted/reply_accepted/turn started/applied 的独立事实与应用证明校验；无证明保持待核实，不能因 terminal send 成功解锁。确认 CLI 文件答案和在线控制答案不会形成两条重复续跑路径；在调用前展示能力缺口。
4. **拆开恢复核心与外围进程。** 从 `MarkStopped`、`AcceptResult`、`RetryOne/startRetry` 提取可在持锁服务里调用的业务方法。全局 mode/stopped/current_task 的清理留在外围或统一槽重算函数，禁止在线动作误改服务生命周期。
5. **实现恢复核验和控制动作。** 提供只读 InspectRecovery，不调用 NewService。在线 retry/mark/accept 使用协议 handler；offline CLI wrapper 取得锁后复用业务方法。结果文件按 H5 固定内容、核验后存入受管 recovery 目录。
6. **实现不确定指令恢复。** resolve_command 关联原请求/回执与 blocker，复用进程/claim/worktree 核验，生成独立恢复回执并只解除目标阻塞。为没有完整 run 的路径保留真实缺失字段，沿用孤儿记录约束。
7. **完成交互页面。** 将简易决策 modal 换成可滚动详情、选项详情和 Unicode 文本输入；增加草稿、冲突、各级接受/轮次启动/应用待核实状态。问题卡片独立于过程折叠；增加恢复面板、只读核验结果与显式停止确认，禁用项显示能力及业务原因。execution/attempt/session/turn/decision/command 历史按真实绑定互相导航，缺失字段不猜测。
8. **更新使用契约。** 修改 decisions/state-model/data-contract/README、恢复命令说明和相关样例；说明 local retry 范围、在线 retry 与 standalone RetryOne 的外围差异、答案不可改以及恢复审计位置。

## 验证

- 决策协议单测覆盖 free-text 组合、精确 decision 身份、同义重复、冲突及旧路径。用两个真实客户端进程争写答案，验证文件层 first-wins。
- 扩展 fake Orca 测试：两轮决策、incarnation 改变、离线答案后启动、resuming 故障。断言 terminal send 次数，不能只验证最终 View 中出现“成功”。
- Windows 离线答案场景使用真实服务窗口入口：回答客户端只保存答案，不代启服务；由用户重新打开窗口后核验接受与应用状态。
- 用同一参数化后端契约验证 H-A13～H-A15：平台接受无证明、turn.started 无证明、错误 decision/attempt/答案摘要、迟到应用事件、无去重能力时响应丢失、同 Operation 恢复。断言无重复外部调用、问题仍保留、槽未错误释放。local unsupported 与 Orca 支持范围分别使用真实能力报告。
- 扩展 retry/manual recovery fixtures，覆盖在线服务保持 running、其他 PR 继续跟踪、另一 unknown 继续保留槽、dirty/advanced/foreign claim/unsupported host 拒绝。
- 接回结果测试在 UI 读取后替换原文件，确认服务处理冻结字节；再覆盖 digest 错误、超限、错误 PR 目标、重复 PR 和缺失 workspace 证据。
- 对 resolve_command 覆盖副作用前中断、已有 unknown run、只有 claim 的孤儿状态、已知进程存活和无法查询外部事实。
- 延续 PR 2 故障注入，在答案发布、resuming 保存、terminal send、worker start、结果绑定、blocker 解除与回执发布之间中断，断言没有第二次执行或伪造完成。
- 运行 `go test ./... -timeout 10m`，完整回归为合并条件。用 Windows TTY 对长问题、中文多行、12 选项、冲突与恢复确认做交互冒烟；执行对象使用测试 fixture。
- 在隔离真实 Orca 路径验证同会话回答与实际应用证明；仅有 send 回执时如实记录能力不足，不能宣称 applied 验收成功。H-A16 的等待/折叠/新轮切换用真实 TTY 验证；运行 `go vet ./...`、平台构建、`git diff --check`，按验收 ID 归档版本与证据。

## 完成条件

- H-A1～H-A16 覆盖，总览七个交付阶段（1、2、3、4A、4B、4C、5）的整体验收场景完成；不能用原“五个 PR”计数提前结束。
- UI 与 CLI 对同一动作给出一致资格和拒绝理由；无直接 runtime 编辑或 out-of-band terminal send。
- 审计可以回答“谁对哪个问题/运行做了什么、服务实际执行到哪里、为何拒绝或未知”。

## 风险与回退

主要风险是把停服维护代码搬入在线服务后清掉其他运行的全局状态，以及把答案保存误认作续跑完成。先建立核心方法与状态转换测试，再接 UI。新决策目录由已有绝对路径字段承载，老 pending 文件继续可读；答案 first-wins 是明确的行为变更，不能回退到旧覆盖写客户端与新服务混用。
