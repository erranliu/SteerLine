# 独立调试环境验收

归属：普通开发“独立调试环境”；[Plan](../plans/debug-release.plan.md)。

| ID | 自动验收项 | 状态/证据 |
| --- | --- | --- |
| DBG-01 | 统一生成完整独立 SteerlineTest 环境，复制源控制配置，默认 live | 通过；两包 ready，项目/数据库身份不同，manifest 与初始化回执 |
| DBG-02 | 显式模拟启动，真实 Git/SQLite/任务三轮、PR 交付和依赖解锁 | 通过；runtime-test.log、controlled-delivery.json |
| DBG-03 | 无备份重置；活动锁阻止重置；保留任务定义/契约/依赖/Git/PR | 通过；runtime-test.log：service/UI 锁负例、check 不改数据、实际重置、Git/provider 未变、无 backup |
| DBG-04 | 外部路径、继承环境与目录冲突拒绝/隔离，安装环境无写入 | 通过；安装路径污染隔离、配置误传拒绝、junction/hardlink 拒绝、时间目录冲突拒绝；所有运行/写入目标为生成包 |
| DBG-05 | 新 debug 包中真实 SteerlineTest PR 创建和核验，清理本次资源 | 通过；实际复制的 publish.ps1 在中文/空格路径运行，PR #183 的 base/head/head SHA/OPEN 核验，随后关闭并删除本次分支 |

## 2026-10-10 执行证据

最终命令：`powershell -NoProfile -ExecutionPolicy Bypass -File scripts/test-debug-release.ps1 -Live`。两包根目录为：

- controlled：`C:\SteerlineDebug\中文 verification-0d4ed79e61be4ebfa163b606dcea7da3\dev-8e292172-dirty-20261010-094818`。
- live：`C:\SteerlineDebug\中文 verification-0d4ed79e61be4ebfa163b606dcea7da3\dev-8e292172-dirty-20261010-094841`。

controlled `evidence/runtime-test.log`：1 test、0 skip、PASS，110.25 秒；`evidence/generation-test.json` 为 passed=true/live=true。live `evidence/live-pr.json` 和 `publish-stderr.log` 记录真实脚本发布与精确身份；[SteerlineTest #183](https://github.com/erranliu/SteerlineTest/pull/183) 已关闭，专属 base/head 分支已删除。未调用模型，不声明真实 Codex 三轮/token 验收通过。此前 ASCII 路径 [#182](https://github.com/erranliu/SteerlineTest/pull/182) 同样已关闭并清理分支。

L1：`go run ./cmd/test-feedback -level=L1` passed=true，execution=177.8s；报告 `C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261010T013811.424407600Z\summary.json`。定向 `go test ./tests/e2e/internal/driver ./tests/e2e/internal/controlled -count=1` 通过；该普通命令跳过需新生成包的 opt-in 场景，不能替代上述无 skip 的 runtime 证据。文档链接检查 0 errors，git diff --check 通过。

源码源头 commit `8e292172d84c902e93cedb9870a658e90a9db0c4`，包含本次未提交改动；各包 manifest 保存生成脚本/复制源/产品/helper SHA-256。另已生成未启动的默认 live 包 `C:\SteerlineDebug\dev-8e292172-dirty-20261010-095055`。

验证过程中修正了 native state 路径派生、服务 readiness/await_merge 的等待，以及 SQLite SHM 在路径扫描中消失或共享锁冲突的竞态；最终扫描使用 Windows 元数据句柄，不读取活动 SHM 内容。生成的 PowerShell 文件使用 UTF-8 BOM，确保 Windows PowerShell 5 正确解析中文路径。

人工项：用户在方便时检查 live 控制台与真实 Codex 体验，不阻塞本次自动验证。真实模型交付与原生 token/会话验收仍使用既有 live 场景，不能由模拟或脚本 PR 证明。
