# PR 5 Plan：人工决策与恢复闭环

任务 ID：3；任务名称：人工决策与恢复闭环。

依据：[Spec](pr-05-intervention.spec.md)。依赖：PR 2、4A、4B、4C 审核合并；从最新 origin/main 实施，复用已落地的权威存储与接口。

2026-09-28 验收补充：在独立本机 Orca 1.4.209 目标上，经过真实能力认证后只丢弃一次已接受的原续跑 `terminal create` 回执。[H-A15 回执](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/pr-05-h15-response-loss-2026-09-28.json)显示原 Operation/Attempt/Decision 与执行槽跨服务重启保留，工作树只有原 terminal 和单次标记，无重发或 PR；由于 Orca 回执未带平台 request ID，保持 unknown 待人工核实。后续[同一 SQLite 恢复预览](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/pr-05-h15-recovery-preview-2026-09-28.json)已证明缺失的当前格式答案可从原 accepted command 按精确身份恢复，但这次故障没有合格的 `retry_run`/`mark_stopped` 动作。该段记录当时的开放状态；后续 H-A6 单字段故障注入、H-A16 真机 TTY 和合格恢复确认均已另行归档，平台自然发生同句柄换代不再作为 #3 关闭条件。

2026-09-28 后续：[独立合格恢复现场](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/pr-05-recovery-confirmation-2026-09-28.json)已完成只读预览、原终端停止核实、确认命令、同 Run/工作树的新 Attempt 和后续同库状态核对。对再次确定性失败的步骤，中心现在保留已结算 `failed/blocked` 并继续在线；CLI 将可选确认依据写入 accepted `retry_run` 命令。此补充更新上段当时尚未取得可执行恢复对象的结论，H-A16 精确 TTY 折叠仍未通过。

## 实施步骤

1. **统一答案命令。** 建立固定 decision_id、first-wins 与相同内容幂等；扩展 free-text 语义。UI/CLI 使用同一当前权威，不读取或导入旧 pending 答案文件。
2. **接入答案动作。** 为 answer_decision 加 handler 与能力声明；完成回执只表示答案被服务接受，后续 `applyDecisions` 单独推进。相同答案作为可恢复证据，不能重发 continuation 来补回执。
3. **完善续跑守卫与证明。** 把单槽/unknown/stop 条件纳入 applyDecisions，沿用 4A 的固定 ref、Operation 与能力，保留 resuming 先落盘及身份核验。实现 saved/accepted/reply_accepted/turn started/applied 的独立事实与应用证明校验；无证明保持待核实，不能因 terminal send 成功解锁。
4. **拆开恢复核心与外围进程。** 从 `MarkStopped`、`AcceptResult`、`RetryOne/startRetry` 提取可在持锁服务里调用的业务方法。全局 mode/stopped/current_task 的清理留在外围或统一槽重算函数，禁止在线动作误改服务生命周期。
5. **实现恢复核验和控制动作。** 提供只读 InspectRecovery，不调用 NewService。retry/mark/accept 使用同一命令 handler；服务离线只读。结果文件按 H5 固定内容、核验后存入受管 recovery 目录。
6. **实现不确定指令恢复。** resolve_command 关联原请求/回执与 blocker，复用进程/claim/worktree 核验，生成独立恢复回执并只解除目标阻塞。为没有完整 run 的路径保留真实缺失字段，沿用孤儿记录约束。
7. **完成交互页面。** 将简易决策 modal 换成可滚动详情、选项详情和 Unicode 文本输入；增加草稿、冲突、各级接受/轮次启动/应用待核实状态。问题卡片独立于过程折叠；增加恢复面板、只读核验结果与显式停止确认，禁用项显示能力及业务原因。execution/attempt/session/turn/decision/command 历史按真实绑定互相导航，缺失字段不猜测。
8. **更新使用契约。** 修改 decisions/state-model/data-contract/README、恢复命令说明和相关样例；说明 local retry 范围、单一答案命令、答案不可改以及恢复审计位置。

## 验证

- 决策协议单测覆盖 free-text 组合、精确 decision 身份、Unicode 长度拒绝、同义重复和冲突。用两个独立真实 OS 客户端进程同时争写答案，验证当前权威 first-wins。
- 扩展 fake Orca 测试：两轮决策、incarnation 改变、服务重启和 resuming 故障。断言 terminal send 次数，不能只验证最终 View 中出现“成功”。
- Windows 服务离线时只展示固定入口、不提交答案；用户打开服务窗口后再通过同一命令回答。
- 用同一参数化后端契约验证 H-A13～H-A15：平台接受无证明、turn.started 无证明、错误 decision/attempt/答案摘要、迟到应用事件、无去重能力时响应丢失、同 Operation 恢复。断言无重复外部调用、问题仍保留、槽未错误释放。local unsupported 与 Orca 支持范围分别使用真实能力报告。
- 扩展 retry/manual recovery fixtures，覆盖在线服务保持 running、其他 PR 继续跟踪、另一 unknown 继续保留槽、dirty/advanced/foreign claim/unsupported host 拒绝。
- 接回结果测试在 UI 读取后替换原文件，确认服务处理冻结字节；再覆盖 digest 错误、超限、错误 PR 目标、重复 PR 和缺失 workspace 证据。
- 对 resolve_command 覆盖副作用前中断、已有 unknown run、只有 claim 的孤儿状态、已知进程存活和无法查询外部事实。
- 延续 PR 2 故障注入，在答案发布、resuming 保存、terminal send、worker start、结果绑定、blocker 解除与回执发布之间中断，断言没有第二次执行或伪造完成。
- 运行 `go test ./... -count=1 -timeout 20m`，完整回归为合并条件。用 Windows TTY 对长问题、中文多行、12 选项、冲突与恢复确认做交互冒烟；执行对象使用测试 fixture。
- 在隔离真实 Orca 路径验证同会话回答与实际应用证明；仅有 send 回执时如实记录能力不足，不能宣称 applied 验收成功。H-A16 的等待/折叠/新轮切换用真实 TTY 验证；运行 `go vet ./...`、平台构建、`git diff --check`，按验收 ID 归档版本与证据。

## 完成条件

- H-A1～H-A16 覆盖，总览七个交付阶段（1、2、3、4A、4B、4C、5）的整体验收场景完成；不能用原“五个 PR”计数提前结束。
- UI 与 CLI 对同一动作给出一致资格和拒绝理由；无直接 runtime 编辑或 out-of-band terminal send。
- 审计可以回答“谁对哪个问题/运行做了什么、服务实际执行到哪里、为何拒绝或未知”。

## 风险与回退

主要风险是把停服维护代码搬入在线服务后清掉其他运行的全局状态，以及把答案保存误认作续跑完成。先建立核心方法与状态转换测试，再接 UI；旧答案路径和旧客户端协议不进入新状态权威。

## 测试计划（2026-10-07 导航维护）

本次仅将已清理的证据导航改为固定 Git 快照，不改原实现契约或追认历史通过。最小范围为文档静态 `doc-links`、`task-identities`、`whitespace`：当前 Windows/Git/Python 3 环境，交付前执行 `python scripts/check_doc_links.py`、`python scripts/roadmap_task_id.py check`、`git diff --check`；通过条件为无缺失本地链接、任务身份一致及无空白错误。固定历史链接的目标另用 `git cat-file --batch-check` 核对存在；结果见[本次清理验证](../../../validation/README.md#测试计划与本次结果)。不选择 L1/L2/L3、Full 或原生模型组；导航变更不需要这些验证，不新增场景或环境能力。
