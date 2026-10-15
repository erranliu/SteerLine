# SteerRunner 接入验收

归属：[接入 Plan](../spec/steerrunner-cutover.plan.md)。普通开发，不纳入 roadmap。Runner 内部及真机验收由其独立仓库 `docs/acceptance/steerrunner.md` 管理。

| ID | 自动验收项 | 当前结果 |
| --- | --- | --- |
| SLC-A01 | 公共 CLI 严格解码、丢响应原请求、终态不重发、显式新授权 | 已有定向单元通过，最终证据待固定 |
| SLC-A02 | 候选原子领取和固定请求、绑定/事件/游标 CAS、终态停止义务 | SQLite 定向通过，最终证据待固定 |
| SLC-A03 | 正确合并候选唯一交付、无 pr_ready 门槛、兄弟未知不阻塞、原子回滚与幂等 | SQLite 定向通过，最终证据待固定 |
| SLC-A04 | 实际产品命令→Runner CLI 的完整闭环、双库、重启和停止 | 实际 service/config/控制/读取已切换；独立 EXE 闭环仍在复验，未通过 |
| SLC-A05 | 最小既有 L3 场景切换到新 CLI 后通过 | driver 已使用 v3 配置和公开候选投影，五场景尚未全部运行 |
| SLC-A06 | 单一执行 owner、旧 actor/worker/适配器/写表删除、固定可获取 Runner 版本 | 待完成；当前分支不能作为已完成产品切换交付 |

人工项 SLC-M01：两个独立项目的安装/升级、服务与日志定位体验；窗口待双方自动链路可用后安排，可后置，不阻塞无关开发。

所有运行证据归本 worktree `.evidence/steerrunner-cutover/`，登记实际源 commit/dirty 状态、命令、退出码及非零场景数。代码和文档尚未提交时的通过只说明当前定向边界，不代表最终版本验收。

2026-10-15：`client-release.log` 的 runnerclient/runnerbridge 定向通过，新增每次调用前 EXE 摘要校验、公开 schema 校验及外部 $ref 零读取。`control.log` 的 `go test ./internal/storage/sqlite -run '^TestRunner' -count=1 -timeout=2m` 通过：包含候选/交付、Task service、真实 host scheduler、WAL 快照、调度事务回滚和已领取但外层回执丢失后的原 ID 恢复。测试保留了夹具缺少 profile digest、契约目标和验收标题时的失败；修正夹具后运行成功，未放宽生产规则。

实际 app service 已使用 taskrunner actor，版本 3 配置固定独立 Runner EXE/版本/SHA；答案、resume、作废、候选控制和 TaskDelivery 均经过公开 caller 边界。新 SQLite schema 21 删除旧执行/原生会话写表，旧根执行 actor/worker 和其专属 in-process 测试已物理删除。原生 adapters/workflow/reliable 的测试辅助依赖仍待清理；五个 L3 尚未全部执行，不能声明完成跨仓交付。


2026-10-15 迭代证据（当前 source dirty，不作为最终固定版本）：

- `all-test-compile-2.log`：全部 Go 包只编译、不运行完整测试，通过。
- `pure-contract-tests.log`：app、runnerclient、runnerbridge、taskrunner、SQLite 定向通过；storage ports/SQLite 只引用 `runnercontract`/`runnerstate` 纯值，不反向引用 CLI 应用实现。
- `caller-boundary-input.log`：taskrunner、边界 checker、storage ports 与产品 CLI 依赖闭包通过；含多原生问题完整键集/选项/输入不可变及单题展示。
- `caller-integration/result-8.json`：实际启动命令已接受，Runner 缓存 clone 失败，1 场景退出 1；未创建 Run/模型输入。
- `caller-integration/result-9.json`：1 场景退出 1，补充的脱敏诊断证明 Windows 深目录加专属分支名超过 Git 默认路径限制。Runner `5f664a9` 增加进程内 scoped longpaths，真实长分支 clone 测试通过，未修改全局配置。
- `caller-integration/result-10.json`：1 场景退出 1，调试命令扫描到原子凭据发布的瞬时双链接；修正只读扫描边界，重置在停服证明后校验删除树。需要新包复验，不能将此记录为通过。

旧测试退休范围见[删除审计](steerrunner-deletion-audit.md)。原生执行专属用例迁由 Runner 已迁出的测试覆盖；这不表示旧全部集成覆盖已等价替换，新 CLI 跨进程和 R1 场景仍为必验。
