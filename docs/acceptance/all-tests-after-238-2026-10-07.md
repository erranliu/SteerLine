# #238 后全项目回归与修复

2026-10-07，按用户要求 rebase 到最新主线并跑全项目测试。最终 rebase 基线为 `4eb722f16ceaccdcf3c19828213727e66bee403f`（PR #238），全部最终 Go/E2E 报告来自 clean 源码 `6e4f1504627c4e5febf52a947d2b10e96b91773a`。本记录后的提交只增加验收摘要和导航，不改实现。

| 范围 | 最终结果 | 命令（省略独立临时 `-out`） |
| --- | --- | --- |
| Full，三 worker | 1228 条父子记录，0 失败，15 默认 opt-in skip；完整覆盖，约 52 分钟 | `go run ./cmd/test-feedback -workers=3` |
| L1 | 428 条记录，0 失败、0 skip | `go run ./cmd/test-feedback -level=L1` |
| 完整 L2 | controlled 门槛与两条真实 smoke，3/3 | `go run ./cmd/test-feedback -level=L2` |
| L3 controlled | delivery / recovery，2/2 | `go run ./cmd/test-feedback -level=L3 -suite=delivery,recovery -env=controlled` |
| L3 live | 本地 Codex / Orca 的 delivery / recovery，4/4 | `go run ./cmd/test-feedback -level=L3 -suite=delivery,recovery -env=live` |
| L3 candidates | 双候选、反馈重放、连接故障、替代候选、在途落选、唯一交付和后继，1/1 | `go run ./cmd/test-feedback -level=L3 -suite=candidates -env=live -scenario=candidates.local` |
| 带 tag 基础设施与边界 | 173 条记录，0 失败，1 默认 sandbox preflight skip | `go run ./cmd/test-feedback -- -tags=e2e ./cmd/test-feedback ./tests/e2e/internal/... ./tests/e2e/cmd/... ./internal/testkit/boundary/... -count=1` |
| 离线 Python | roadmap-runner 18/18、任务身份脚本 10/10 | `python .agents/skills/roadmap-runner/scripts/test_state.py`；`python scripts/test_roadmap_task_id.py` |

注册表 10 个场景全部通过，均 `cleanup_complete=true`，真实场景均 `usage_verified=true`。candidates 使用本次独立 manifest，源码固定为上述 40 位 commit。环境为 Windows、Go 1.26.4、真实 `gpt-6-luna/medium`，采用当前固定 `workspace-write`、`unelevated`、`on-request`、`auto_review` 和 network allow。默认 skip 仍是未执行的 opt-in 检查，不算通过；本记录不扩展为人工界面或 race 验收。

## 修复

- Orca relay 接受 Windows PowerShell 生成的 UTF-8 BOM，并给出 ConstrainedLanguage 可用的原子写入指引。原 Run/Turn/Task/Dispatch/capability、文件路径和大小验证保留；新增 BOM 及错误身份拒绝回归。
- 完成 worker 在重启恢复时，不再让可选反馈决策覆盖已验证 PR 的 `waiting_pr_merge`。保留原 PR、决策和执行身份，新增接线回归并通过真实本地恢复场景。
- 同步当前 codec 和配置断言，修复移除 raw args 后的 fixture panic、过期控制台选择/标题断言；历史会话样本改读不可变 Git 快照，保留原字节和断言。
- 测试发布脚本采用只限 SteerlineTest remote 的进程环境 HTTP 认证，避免 unelevated sandbox 中 Git Bash credential helper 失败；原始和编码凭据均脱敏。候选精确文件写入改用 PowerShell 7 cmdlet，保留 UTF-8 无 BOM、单 LF 的内容合同。
- usage 只选原 root 业务会话，排除同 cwd 的自动审核子会话；错误业务模型仍拒绝，不按模型名筛掉错误证据。
- 按实际多进程耗时调整有界测试预算：store 生命周期 30 秒→2 分钟；双回合 2→3 分钟；候选控制台及五 Dispatch 恢复总预算为 8 分钟；相应原会话启动/反馈等待为 60 秒；正向 Orca fixture 准备 1→2 分钟。候选真实场景总预算 20→30 分钟，Full 总期限 50→60 分钟，各 Full 组命令原期限及覆盖完整性检查保留。

## 证据与范围

[最小结果索引](all-tests-after-238-2026-10-07.json) 保存实际源码、选择、结果、默认 skip、场景 proof、报告路径和 SHA-256。原始产物及完整索引保留于系统临时目录 `C:\Users\lmas1\AppData\Local\Temp\steerline-all-tests-after-236-20261007-134446`，不提交原始报告树，不承诺临时目录长期保留。

早期编译失败、fixture 错误、业务失败、并发负载超时、组合诊断的 Go 默认 10 分钟中断、50 分钟 Full 中断和重跑均保留原结果，未被最终通过覆盖。最终场景覆盖只使用 #238 后七份完整通过报告，没有用早期部分通过替代最终覆盖。

文档链接检查通过。`roadmap_task_id.py check` 仍遇到另一个工作树把任务 49 改名造成的共享 Git ref 冲突（#238 已记录的环境限制）；没有覆盖对方共享登记。当前分支的 tracked 文档身份独立检查通过，不宣称共享 ref 检查通过。
