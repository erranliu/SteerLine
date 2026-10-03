# #185 后 smoke 修复与验收

2026-10-02 16:02:43–16:09:55（Asia/Shanghai），完整 L2 **通过**，`coverage=full-L2`、`cleanup_complete=true`。候选生产代码为 `0fcd678`；后续 `eb86901` 仅修正环境回归 fixture 的加锁读取，不改变真实执行代码。

命令：`go run ./cmd/test-feedback -level=L2 -out=<fresh>`。原始报告目录：`C:/Users/lmas1/AppData/Local/Temp/sl-smoke-lifecycle-fix-20261002-160243`。本目录只保存 summary、场景元数据、usage 数值及筛选的权限/回合证据，不复制认证、私有数据库或含 dispatch capability 的完整输入。

| 场景 | 结果 | 用时 | 原生累计 tokens | 测试 PR |
|---|---|---|---|---|
| smoke.delivery | pass | 73.50s | 无模型请求 | controlled |
| smoke.local-codex | pass | 124.17s | 65,672 | [#37](https://github.com/erranliu/SteerlineTest/pull/37)，MERGED |
| smoke.orca-codex | pass | 222.48s | 291,348 | [#38](https://github.com/erranliu/SteerlineTest/pull/38)，MERGED |

总墙钟约 7m12s。成功运行合计 357,020 tokens（含 cached input，取最终累计样本、不重复相加）；模型固定 gpt-6-luna / medium，两条链路 danger-full-access / approval_policy=never。Orca 原 readiness/business 两回合均 settled/succeeded，准备 verified/ready；交付轮询与清理 usage 不增加。三场景清理完成，额外 `git ls-remote --heads` 检查本次两个 base、两个 head 全部零匹配。

修复：

- Codex 空线程在首个回合前没有 rollout，prepare 断开后不能 resume。保留原连接到中心提交身份并启动首回合；拒绝或取消时释放连接。start 失败回执保留原 SessionID，进入明确的未知结果阻断。
- Orca 同一终端的 agentIdentity 尚为空时属于初始化。返回原资源 pending 回执并让出 actor，下轮只读识别；保留 terminal/runtime/incarnation/worktree 和原 deadline，替换或错误 agent 仍拒绝。
- 完全访问配置没有 Windows sandbox 字段，原 managedWindowsOrca 条件漏掉 readiness。恢复 host-write 的准备路径，不削弱真实两回合断言。修正 lifecycle 中两处项目 WriteScope 与原生 sandbox 名混用。
- control helper 原 25s 回执等待短于正常原生资源准备；等待绑定到产品已有的 request.ExpiresAt（2m），不改变产品截止、不重发请求。外层进程预算包含身份解析和退出，普通 CLI 仍为 30s。
- 两套受控 app-server 都拒绝空线程 resume，使无 token 门槛覆盖本次首回合回归。环境 fixture 读写共用锁，消除 Windows 文件占用竞态。

定向验证：codexworker/orcaworker/execution/test-feedback/E2E 内部包通过；本地首回合、同会话反馈、部分回执通过（57.843s）；full-access freeze、原终端检测/deadline、readiness 成功失败、原问题反馈通过（107.128s）。加锁后反馈场景连续两次通过（35.306s），先前失败的 CD64 反馈子测试独立复核通过（21.405s）。E2E 各场景编译通过。未跑全量 Go 或 L3；用户本次要求修复并重跑 smoke。

保留失败记录：attempt-1 本地通过且 PR #34 合并，Orca control helper 提前超时；attempt-2 本地通过且 PR #35 合并，Orca 业务成功提 PR #36，但严格 readiness 断言失败，PR 按本次所有权关闭。两轮清理均完成，最终验收是上述单次完整 L2，未拼接失败运行。失败运行缺少 usage 不代表没有计费；其可读取原生累计样本另见各 attempt 的 usage JSON。

排错运行已有原生累计样本合计 307,965 tokens；加上最终成功运行，本轮所有已取得样本合计 664,985。该数值只汇总原生报告，不换算账单，不将缺失样本当作零。
