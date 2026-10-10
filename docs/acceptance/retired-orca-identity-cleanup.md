# 旧 Orca 编排身份残留清理验收

仅归属当前普通开发的 [Spec](../spec/retired-orca-identity-cleanup.spec.md) / [Plan](../spec/retired-orca-identity-cleanup.plan.md)。实施/验证基线 `d77c399f`，提 PR 集成 `6d4b4fa2` 的文档复测成果，不占任务编号。

| ID | 自动验收项 | 状态 / 证据 |
| --- | --- | --- |
| RC-A1 | 删除无当前消费者的旧编排身份与 host 死函数；当前资源身份往返保持，旧字段严格拒绝，清理结算身份不变，准备规则的现有回归保持 | 已验收：execution 定向 34 项（含子测试）及 SQLite 1 项通过，0 fail/skip；源码核对无旧字段生产消费点 |
| RC-A2 | 当前 local / Orca local / Orca environment 的受控三阶段、原绑定及重启不重投保持，所有消费者编译与定向静态检查通过 | 已验收：三个受控集成场景通过，0 fail/skip；普通包与带 tag 编译、定向 vet 通过 |
| RC-A3 | 当前文档与执行汇总引用新路线的实际验证范围，保留历史缺口、原生失败、本机最终保护未跑真机、远端已准备环境有限样本及首次信任后原任务自动继续限制和一期待签收事实；导航/身份/格式检查通过 | 已验收：逐项文档/差异核对及链接、任务身份、格式检查通过；不变更原任务范围/依赖或历史验收结算 |

原始报告使用 test-feedback 默认临时目录，静态日志使用本次独立临时目录；不提交原始日志。本次验证不运行真实模型或产品真实 PR 测试，不修改安装环境，不增加一期验收通过项。

## 2026-10-10 实施与验证

Windows/amd64，Go 1.26.4；源码 `d77c399fc7f4daac081c54fee01e5361e3d373c6` 加本次未提交工作区增量，报告保留 source/worktree/命令与实际终态。测试使用既有隔离夹具。以下目录均在 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/`，含 summary.json 和原始 Go JSON 日志。

| 报告目录 | 对应范围与实际结果 |
| --- | --- |
| 20261010T112536.898217800Z | Plan 的 execution 定向命令：13 个顶层/34 项含子测试，0 fail/skip，478.850 秒墙钟。当前终端 target/process 往返、9 个旧字段拒绝、结算后的 cleanup 回执不改写身份、原输入结果绑定及准备/故障/重启门槛全部通过 |
| 20261010T112537.585065900Z | Plan 的三个集成场景：LocalRoundTrip / OrcaTerminalRoundTrip / OrcaEnvironmentRoundTrip，3 pass/0 fail/0 skip，398.217 秒墙钟。真实隔离 Git/SQLite/进程，模型及 provider 为受控替身；不作为远端真机证明 |
| 20261010T112759.904430900Z | SQLite 定向：1 pass/0 fail/0 skip，6.025 秒墙钟；同一 Workflow 拒绝第二个 Execution，快照断言保持 |

静态日志目录：`C:/Users/lmas1/AppData/Local/Temp/steerline-identity-cleanup-887a192a997a467093cbeee9cd3bea2e/`。普通包仅编译（compile-final.log）、带 e2e tag 仅编译（e2e-compile.log）及 execution/app 定向 vet（vet.log）通过。首轮 compile.log 暴露 SQLite fixture 中遗漏的 Dispatch 字段，失败原日志保留；移除该无消费者 fixture 字段后补跑其原业务断言及完整普通包编译，两者通过。没有生产存储/schema/事务改动；初始集成运行之后的代码增量仅这一 SQLite 测试夹具，因此无需重复三个已通过的流程。

最终文档检查日志为 final-links.log / final-ids.log / final-diff.log。执行清单只校准当前能力及待验引用，历史执行记录未改写，Task ID/依赖/人工窗口不变。旧 #84/#45 supervised 缺口保留历史；本机单终端有限真实样本、本机最终 draft 保护未跑真机、远端升级前真实失败及一期组合/24小时/同构建恢复待验继续保留；当前远端复测范围按下一节更新。

未运行 Full/L2/L3/live/native；本次没有新的原生证据，也没有跨构建安装接管验收。

## 提 PR 时同步最新主干

用户授权提 PR 后 fetch 核对最新主干 `6d4b4fa2`（PR #288），上游相对 d77c399f 只修改三份 Orca 验收文档。保留本次提交后无冲突 rebase，并依据[远端升级后复测](orca-terminal-remote-retest-2026-10-10.md)修正当前汇总：预先确认测试仓库目录信任、Codex CLI 0.162.1/default 模型环境中的有限样本三阶段/重启不重投/真实 PR #204 交付通过；首次信任可正确阻塞零投递，被拒绝原任务自动继续未通过。升级前失败仍保留。

本次代码与首个提交 bf21ffc7 的 internal/cmd/tests Git blob 差异为空；rebase 与后续修订只有文档，前述 38 项测试、编译及 vet 证据按此映射沿用，未重复长流程或新增真机验收。提 PR 文档复验日志为 pr-links.log / pr-ids.log / pr-diff.log，证据目录同上。
