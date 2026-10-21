# 验收：Orca 本机输入与就绪

> 归档于 2026-10-21：退役方案，原失败和未验项保留，不作为当前补验或实施队列。当前接入见[入口](../../../integration/steerrunner.md)。固定 commit/原路径及证据内容继续按原记录取证。

[Spec](../../../archive/spec/retired/orca-local-input-readiness.spec.md) · [Plan](../../../archive/spec/retired/orca-local-input-readiness.plan.md)。

| 自动项 | 条件 | 状态 |
| --- | --- | --- |
| OLI-A01 | 一次有界复查、明确交互阻塞、预算、绑定拒绝及未知不重发、身份隔离 | 通过；定向与必要回归见下 |
| OLI-A02 | 生产本机普通终端两次只读回合，原生状态与正文，清理 | 通过；输入/结果各 2，原生 outcome=unknown，原终端已关闭 |
| OLI-A03 | 一个本机真实专属 PR 合并及 SteerLine 交付，清理 | 通过；PR #228 合并，任务 1 delivered，服务/原终端已清理 |

## 2026-10-13 调查

原生 Orca 1.4.224，runtime fb7953d3-29a6-4f81-b6b9-fa257bb98d04。独立探针包 C:/SteerlineDebug/dev-20261013-104556-efd492ca；evidence/local-routing-comparison.json 保存两条路由各三次状态查询，都返回同一 ready runtime。未复现历史管道故障；本机 1 秒预检是已确认代码机制，不声明它已被证明是现场根因。配对凭据不进入包或仓库。

## 实施与最小验证

基线 efd492cac9940a50966b3ccdb65d56012931d1d6 加本次工作树；固定 CLI C:/Users/lmas1/AppData/Local/Programs/orca/resources/bin/orca.exe，Codex CLI 0.159.2。本机普通 CLI 和原生订阅共用进程中的配对认证，先由本机 bootstrap runtime 核验配对目标；environment 不继承本机认证。等待预算由固定 40 秒改为实际等待加 10 秒，发送预算 50 秒覆盖 30 秒提交观察及响应余量。

只认完整绑定且 accepted=false、没有接受/启动阶段的拒绝回执；已接受未启动或泛化 runtime_unavailable 不当作明确未投递。现有 CLI 的泛化错误包没有可靠投递阶段字段，因此不对历史错误文案加启发式结算。本机切换配对路径后正常交付已验证；不能据单次成功承诺所有投递失败都可自动解决。

报告根 C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/，以下路径均为该根下目录：

- OLI-A01：Plan 的三包定向命令，20261013T030647.365016800Z/summary.json，17 项、0 skip，通过。涵盖本机 bootstrap 身份、认证/原生 runtime 拒绝、一次就绪复查、交互阻塞、预算、绑定拒绝持久化、错误请求/泛化错误保持未知、共用回执不把 rejected 改成 unknown。
- 普通终端与状态原回归：20261013T025727.147544600Z/summary.json，46 项、0 skip，通过；之后仅补了拒绝负例和超时回执的 runtime 限制，最终定向命令已覆盖。
- 受控集成最初组合命令的五分钟包期限不足：20261013T030110.228720900Z/summary.json 不完整，不计通过。Codex 用例约 202 秒，本机 gate 约 94 秒，远程 gate 开始后总期限截断；改为 Plan 的两个独立命令。
- 受控本机/environment gate：20261013T030833.205062700Z/summary.json，3 项（父项及两目标）、0 skip，165.885 秒，通过。共用 Codex round-trip：20261013T030833.210454000Z/summary.json，1 项、0 skip，137.552 秒，通过。
- OLI-A02：固定 CLI、已有本机配对认证，运行 Plan 的生产 lifecycle 探针。20261013T025301.790385600Z/summary.json，1 项、0 skip，通过；包 evidence/native-production-lifecycle.json 记录两次 started/working/结束、完整回复及 unknown outcome，cleaned_up=true。没有 PR 或业务文件改动。

## 本机真实交付

OLI-A03 使用包含最终实现的新包 C:/SteerlineDebug/dev-20261013-105910-efd492ca-dirty，单一 live 方案。之前生成但未启动的 dev-20261013-105557-efd492ca-dirty 不计验收。没有重置或使用安装环境。

- 本包 repo ID c8863a05-7040-42e3-89bc-470031286356；workflow f2e7612b7ffc459bb2070f141989f0e5，terminal term_8f91146f-681b-440f-bab6-db7aa81f6e65，incarnation 3d95c1bd-6491-4543-9dae-d9bb140b3b69，runtime 同调查记录。本包 helper 的 debug run/control/snapshot 启动任务 1，唯一原终端完成 implement.execute、implement.report、publish；输入和结果各 3，三个原生 outcome 均为 unknown，普通回复均 final_verified，没有 publish.report、重复输入或替换资源。
- [SteerlineTest PR #228](https://github.com/erranliu/SteerlineTest/pull/228)：base codex/debug/dev-20261013-105910-efd492ca-dirty/base，head erranliu/sl-1-f2e7612b7ffc459bb2070f141989f0e5，head SHA cf98c92c93286fae8311d14dcfe9b828c5a5fbc0。核验仓库、base/head/SHA 与仅修改 result.txt，再通过 GitHub immutable-head contents 检查 UTF-8 两行及末尾 LF；mark ready 后按 match-head-commit 合并。merge SHA fe600fafdc9efebcdec4fb86482f4dd9116e6cba，2026-10-13T03:07:49Z。
- post-merge-snapshot.json：workflow completed/delivered，任务 1 closed，任务 2 dependency_ready=true 且没有执行记录。后继未启动，不恢复已取消的第二轮。
- evidence/pr-before-merge.json、pr-artifact-check.json、pr-merged.json、native-real-flow-summary.json 保存核验、回合与原身份。service-stop.json 确认进程退出和锁释放；cleanup.json 确认原 incarnation 的 terminal.close ptyKilled=true、原工作区剩余终端 0。保留专属分支、数据库和 PR 事实，不做重置。

没有全量测试或远程真实流程；本次只补本机问题与一个真实交付。文档导航、Task ID 静态检查与 git diff --check 单独核验，不能替代以上原生/业务结果。
