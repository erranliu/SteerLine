# 验收：Orca 普通终端完成补查询

唯一归属：普通开发“Orca 普通终端完成补查询”。日期：2026-10-13。

[Spec](../../spec/orca-completion-query.spec.md) · [Plan](../../plans/orca-completion-query.plan.md)

| ID | 自动验收项 | 状态及证据 |
| --- | --- | --- |
| Q-A01 | 无 working 的完成受理、绑定反例、独立查询与故障恢复、正文及一次结算 | 通过；复审后 Q-RULE：57 项、0 skip；独立复跑 37 项、0 skip。Q-DIFF：diff 检查及文档链接检查通过 |
| Q-A02 | 本机/environment report 无 working，三阶段输入/结果各一次，旧 publish 结果不提前推进；在途重启报告一次结算且保留下一输入确认 | 通过；最新主干六个子场景全部通过，各报告 2 项、0 skip；详见最终证据表 |
| Q-A03 | 本机/远端独立包，两次普通短回合仅查询，真实回执/基线/新 done/完整回复及清理 | 通过；Q-LIVE：本机/远端各 1 项、0 skip；各 inputs=2/results=2，WorkingAt=0，cleaned_up=true |

## 执行证据

初次验证源码基点 bd3a3537d48147d7179b7156ec019d4de954d4d1 加本次修改；复审修复及最终定向验证已同步主干 231fa360c62ac8dc1470957026171a3587f2f454。Go 1.26.4 Windows/amd64。精确命令见 Plan，未跑 full。

- Q-RULE 最终：[runner summary](C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T092411.779932900Z/summary.json)，12.782 秒，57 项。覆盖旧 done、缺基线/过期快照、相同正文不同轮次、仅 accepted、pane/provider/身份变化、stream/query 额外输入边界、明确初始化/恢复/工具输出、中断/失败、截断/缺正文、查询故障、暂缺原记录/provider 后恢复、订阅打开/断线故障、永久冲突经过 waiting 与重建仍拒绝、恢复首读失败关闭订阅；保留原文、未知 outcome、一次交付及不重发输入。
- Q-FLOW 初次基点回归：[CompletionQuery](C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T075956.461249600Z/summary.json)，171.065 秒；[NativeStateGate](C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T075956.672064000Z/summary.json)，245.804 秒，各 3 项、0 skip。验证时修正既有夹具：ended 阶段原先错误返回完整 final，使缺正文场景提前推进；修正后两个模式通过。此处为初次证据，最终整合结果见下表。
- Q-LIVE 远端：[runner summary](C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T075524.847774300Z/summary.json)，115.248 秒；[原生证据](C:/SteerlineDebug/dev-20261013-153741-bd3a3537-dirty/evidence/native-production-lifecycle.json)。environment=e68f4edf-8a83-42c2-a9da-6d404d3aa3d8，runtime=95d30a41-2f86-43ae-ac0d-9c304b5f1f90，CLI 1.4.224 / 远端 1.4.223。专属 SteerlineTest worktree 为 `903fb315-7d5e-4246-8913-816e1d3bdf03::C:/Users/Administrator/orca/workspaces/SteerlineTest/sl-completion-query-20261013-1545`，基点 849ed9b33de86920e4b582bd7513628421bbfa99，无 PR/文件变更。两次补查询：基线/结束 1791878161296→1791878172506、1791878172506→1791878212167。
- Q-LIVE 本机：[runner summary](C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T075525.353778700Z/summary.json)，138.996 秒；[原生证据](C:/SteerlineDebug/dev-20261013-154628-bd3a3537-dirty/evidence/native-production-lifecycle.json)。environment=92d72eda-c486-4d01-93ea-07a20ae30907，runtime=fb7953d3-29a6-4f81-b6b9-fa257bb98d04，CLI 1.4.224。专属目标为包内 `projects/SteerlineTest`（repo=367dfd63-9ac9-404d-8e3f-3bd49ebf4789）。三次补查询（其中一次仍 working）：基线/结束 1791878156655→1791878169757、1791878169757→1791878236981。

两份原生证据均丢弃全部订阅通知，业务输入的原请求回执均为 input_accepted+turn_started；同一 pane/provider，两轮相同完整正文，原生 outcome 保持 unknown；无伪造 working，探针创建的 agent 终端已核验关闭。初始化 READY 不计入两次业务输入。早期探针在主代理尚未初始化时失败，未投递业务输入；补齐与生产启动器相同的 READY 初始化后通过。

本次远端临时 worktree 及其默认空 shell 已通过 Orca worktree rm 清理；随后对同一精确 selector 的 show 返回 selector_not_found。两个本机调试包和证据保留。

真实探针验证初次实现时源码的生产适配器和公开 Orca 字段；复审新增的拒绝/恢复规则由最终定向及集成验证覆盖。未运行安装版 worker 或真实 PR 发布。封套 codec 从 4 升到 5，不读取旧 codec；这次没有将安装版 105 迁移到新实现。

## 独立复审及修复

2026-10-13，独立只读 reviewer `/root/completion_query_review`。初审后直接修复并独立复核，未发现剩余可证实缺陷；[reviewer 定向复跑](C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T092406.171412300Z/summary.json) 37 项、0 skip、17.455 秒。

1. 未保存 working 时，已观察到的其他输入原先被忽略，包括订阅和纯 query 的 working 路径。现共用新主代理记录来源核验，冲突持久化；补查 matching done 和仅靠封套重建都不能清除。
2. 后续 waiting 原先可覆盖永久冲突。现 signal 入口保留永久拒绝；覆盖同批次“原 working → 其他输入 → waiting”以及跨重启反例。
3. 订阅打开时的身份冲突原先没有持久化，transport 的 request/runtime 冲突未分类。现打开、接收、查询统一保存明确身份冲突；测试错误分类与重建后拒绝。
4. worktree.ps 暂缺原记录及缺 provider 原先可能误锁永久身份冲突。现只读重试，明确替换仍拒绝；保持当前 100 项查询页，原 worktree 持续不入页时仍不能保证完成，不将页外缺行解释成另一个结果。
5. 恢复首读报错、未交付观察器时，原先可能没有释放已打开订阅。现错误出口关闭 feed，并有关闭计数反例；后续重试才打开的 feed 由观察器释放。

新增 Q-RESTART 在真实 service/SQLite 隔离夹具中于 report 在途停止服务，订阅断开并丢弃 working/done，原 agent 离线完成。初次测试错误地要求自动发送 publish；实际已结算 report，下一输入受已有恢复确认门槛阻止。已修正预期为一次受理报告及保持确认门槛，不修改 worker 职能。

同步主干 #302 后，既有集成 Orca 创建响应缺 worktree instanceId，被生产校验在业务输入前拦截；补齐夹具固定实例字段，保持生产检查，最终按三个场景 × 两个 host 分别重跑。

## 最新主干整合验证

以下均为 231fa360 加最终实现/夹具修复，精确命令见 Plan 的六条入口。并行反馈 runner 的默认时间戳目录出现碰撞，混写日志判 incomplete 的结果不计通过；受影响的三条使用各自 UUID 的 `--out` 目录重跑。

| 场景 / host | 最终状态 | summary 证据 |
| --- | --- | --- |
| Q-FLOW CompletionQuery / orca | 通过，2 项、0 skip，133.934 秒 | [summary](C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/orca-completion-review-edb8c0a6-9c52-42b0-9caf-0419ee6ae2b8/summary.json) |
| Q-FLOW CompletionQuery / orca-environment | 通过，2 项、0 skip，204.717 秒 | [summary](C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T092906.375841500Z/summary.json) |
| Q-FLOW NativeStateGate / orca | 通过，2 项、0 skip，166.490 秒 | [summary](C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/orca-completion-review-87831557-1455-4adf-8b5e-5fe2a74c4c59/summary.json) |
| Q-FLOW NativeStateGate / orca-environment | 通过，2 项、0 skip，169.860 秒 | [summary](C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/orca-completion-review-a5ec755b-e3a4-4a74-93db-ec4fe52c8b80/summary.json) |
| Q-RESTART / orca | 通过，2 项、0 skip，223.571 秒 | [summary](C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T092906.381317000Z/summary.json) |
| Q-RESTART / orca-environment | 通过，2 项、0 skip，224.247 秒 | [summary](C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261013T092906.632355100Z/summary.json) |

没有独立人工验收项。不修改 C:/SteerlineRuntime，不宣称安装版 105 已恢复。能力范围为实际验证的 Codex 普通终端；未证明任意外部并发输入或其他 agent 的同等保证。
