# SteerRunner 接入验收

归属：[接入 Plan](../spec/steerrunner-cutover.plan.md)。普通开发，不纳入 roadmap。Runner 内部及真机验收由其独立仓库 `docs/acceptance/steerrunner.md` 管理。

| ID | 自动验收项 | 当前结果 |
| --- | --- | --- |
| SLC-A01 | 公共 CLI 严格解码、丢响应原请求、终态不重发、显式新授权 | 已有定向单元通过，最终证据待固定 |
| SLC-A02 | 候选原子领取和固定请求、绑定/事件/游标 CAS、终态停止义务 | SQLite 定向通过，最终证据待固定 |
| SLC-A03 | 正确合并候选唯一交付、无 pr_ready 门槛、兄弟未知不阻塞、原子回滚与幂等 | SQLite 定向通过，最终证据待固定 |
| SLC-A04 | 实际产品命令→Runner CLI 的完整闭环、双库、重启和停止 | 待完成实际接入与 driver |
| SLC-A05 | 最小既有 L3 场景切换到新 CLI 后通过 | 待 driver 接入 |
| SLC-A06 | 单一执行 owner、旧 actor/worker/适配器/写表删除、固定可获取 Runner 版本 | 待完成；当前分支不能作为已完成产品切换交付 |

人工项 SLC-M01：两个独立项目的安装/升级、服务与日志定位体验；窗口待双方自动链路可用后安排，可后置，不阻塞无关开发。

所有运行证据归本 worktree `.evidence/steerrunner-cutover/`，登记实际源 commit/dirty 状态、命令、退出码及非零场景数。代码和文档尚未提交时的通过只说明当前定向边界，不代表最终版本验收。

2026-10-15：`client-release.log` 的 runnerclient/runnerbridge 定向通过，新增每次调用前 EXE 摘要校验、公开 schema 校验及外部 $ref 零读取。`control.log` 的 `go test ./internal/storage/sqlite -run '^TestRunner' -count=1 -timeout=2m` 通过：包含候选/交付、Task service、真实 host scheduler、WAL 快照、调度事务回滚和已领取但外层回执丢失后的原 ID 恢复。测试保留了夹具缺少 profile digest、契约目标和验收标题时的失败；修正夹具后运行成功，未放宽生产规则。

应用页面/LoadDatabaseRuntime 已改读 caller RunnerSnapshot，不再从旧执行表重建；分级环境快照驱动同步该格式。版本 3 公开配置和新控制协调器已加入，但实际配置加载、service factory、答案/恢复/作废入口及旧 owner 删除尚在实施，仍不能将本次提交视作已完成产品切换。旧 L3 场景尚未迁移运行，不计通过。
