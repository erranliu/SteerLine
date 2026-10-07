# 第一阶段剩余验收核对（2026-09-28）

基线：`8b79952`，Windows/amd64，本机 Orca 1.4.209。此核对只整理当前仓库已保存的证据并补一条中心 verify 缺回执回归；没有启动新的远端 worker、改写原 SQLite 或把旧主机样本外推到台式机64g。

| 任务 | 已证实 | 下一项验收动作 |
| --- | --- | --- |
| #3 | [原答案应用、重启、响应丢失和恢复确认](pr-05-intervention.md)已有真实本机回执；同句柄新 incarnation 的[活跃 worker 注入](pr-05-h6-incarnation-2026-09-28.json)保留 unknown 和执行槽 | 在绑定原生 Codex history 的新隔离 Task 中，以可见 Windows TTY 核对真实 `turn.completed` / `final_answer` 的折叠、展开与导航；Orca 平台自行产生的同句柄换代仍须单列，不以注入替代 |
| #4 | [当前台式机 UAC](spec03-desktop64g-2026-09-28.md)的提示原图和人工拒绝/批准、[home 活跃问题的超 8 KiB 发送前拒绝](../archive/spec/retired/orca-probe/samples/p10-home-live-reply-size-2026-09-27.json)、[P13 正向](../archive/spec/retired/orca-probe/samples/spec03-supervised-staged-implement-verify-2026-09-25.json)及[非零退出负向](../archive/spec/retired/orca-probe/samples/spec03-supervised-negative-verify-retry-2026-09-25.json)已有真机证据 | 在隔离远端受监管 verify 中分别提交缺少必需检查回执、检查或候选 commit 不符的原 Dispatch 结果，读取中心拒绝、无 verified revision/PR、原身份与资源结算；纯校验器样本不替代此试次 |
| #6 | [台式机64g 的本项目任务](spec03-desktop64g-2026-09-28.md)已在同一 Attempt 应用答案，五步后用固定 `gh pr create` 创建 PR #85，固定 head 合并后中心关闭 Task，五个 Dispatch 释放 | 在新主机测可见 viewer 的 2 秒门槛和故障矩阵，增加持续无人值守试次；旧 home 的延迟和故障样本仍限原目标 |
| #26–#28、#30–#31 | [独立 Spec/Plan 与中心 DAG](phase1-task-creation/README.md)已固定，当前基线未实现新能力 | 先交付无硬前置的 #26、#30，再依边推进；共享 Service/SQLite/CLI 改动串行集成 |
| #29、#32 | 前置任务未交付 | #29 等待 #28/#3/#4/#6；#32 等待 #29/#31，再以固定 stable 构建完成真实双后端与连续 24 小时验收 |

本次检查发现 [版本交付方案](../roadmap/phase-1-delivery.md)仍写着台式机64g 因 Smart App Control 未完成回传/结算，以及当前 UAC 提示尚未捕获；两条均已被 2026-09-28 的原始回执超越，现已按实际结果更新。当前代码的 `AdvanceMinimalExecution` 通过 `validateChecks` 和 `validateSupervisedChecks` 核对必需检查、脚本退出与固定提交；`internal/steerline/remoteprotocol` 的独立纯校验器并不是此状态转换的调用入口。新增回归让缺少整条必需检查回执的 verify 结果进入实际状态转换函数，确认不推进；已有同一测试覆盖缺 evidence reference、错误 actual commit 和非零退出冒充通过。该自动回归不能替代 #4 上述真机负向试次。

验证：`go test ./internal/steerline -run '^TestMinimalWorkflowRevisionGatesAndRepairLimit$' -count=1 -timeout 3m` 通过；`go test ./... -count=1 -timeout=45m -p 1 -parallel 1` 全部通过，核心包 477.151 秒；`go vet ./...`、Windows `go build ./...`、Linux/amd64 `go build ./...`、变更文档相对链接检查与 `git diff --check` 均通过。此处未宣称 #3/#4/#6 或第一阶段整体完成。

H-A16 只读预检：当前 Codex CLI 为 0.157.1。本机 Orca 1.4.209 可用，但 `orca search --index-status --json` 返回 `enabled=false`。将原 4B 独立真实会话绑定传给 `TestCodexHistoryNativeAcceptance` 重新读取时，`local` 子项通过；旧 `orca` 子项返回 `workflow has no persisted run for its execution profile`，整次命令退出 1。这只说明该旧 Orca 会话当前不可复读，不推翻当时保存的[原生验收回执](pr-04b-public-history-acceptance-2026-09-25.json)，也不能作为 H-A16 的新 TTY 折叠证明。下一次须从新的隔离任务绑定当前可读的原生会话。
