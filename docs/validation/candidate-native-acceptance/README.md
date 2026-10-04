# #39 多候选本机验收：本机自动验收通过，待独立复审

起点/开工最新 main：`a8ff0f01d38299d77ecc9cd4463b78a1286e1bec`（PR #208 真实合并）；独立托管 worktree：`C:\Users\lmas1\.codex\worktrees\d831\SteerLine`；分支：`codex/candidate-native-acceptance-39`。#38 CC5 仍待用户人工补验；限定合入例外不扩大到 CN1–CN8。

唯一运行入口：`go run ./cmd/test-feedback -level=L3 -suite=candidates -env=live '-scenario=candidates.local' -out <新的空证据目录>`。设置 `STEERLINE_CANDIDATE_ACCEPTANCE_CONFIG` 指向绝对 manifest 路径；结构如下，source_commit 替换为本次实际固定 HEAD。两份 profile 按本机 Orca、本地 Codex 顺序固定，以在 Orca 完成准备后立即启动本地兄弟并记录真实业务重叠：

```json
{
  "version": 1,
  "source_commit": "<40位实际实现commit>",
  "repository": "erranliu/SteerlineTest",
  "profiles": [
    {"id":"candidate-orca","chain":"orca-codex"},
    {"id":"candidate-local","chain":"local-codex"}
  ]
}
```

用户已将任务名称/范围收缩为「多候选本机验收」，应用协调文档提交 `b53ef52abdd36bfc7d1e8d65bd5b86957794f27c`（本分支对应 `da34963`）。当前仅保留本机字段和场景，无旧远端 manifest 兼容；生产远端规则未改。本任务不提供 #42 的远端证明，也不把远端列为阻塞或待补验。

随后应用全阶段本机范围提交 `9b864afd546c249847339648b74b6d5d4556dc21`（本分支 `6952916`）；#42 同样仅保留本地 Codex/本机 Orca 两条依赖链，仍有五任务/24小时/隔离提升等独立要求。#73 纯类型主线 `3905903` 已同步到本分支，当前实际类型所有者/适配调用与来源记录保持一致；验收固定构建不在现场中替换。

本机只复制既有 auth.json 到隔离 home，不复制用户配置或任务库。新随机 base 分支位于 erranliu/SteerlineTest；worker 独立 commit/push/create，操作者按期望 head 合并。产品查询规范数据库中的 Task/WorkflowRun/Execution/Decision/Operation/Delivery；driver 读取规范 Conversation/完整 Body，native logs 仅为诊断/usage 证据。清理只关闭本次 base 的 open PR、删除本次分支、停止 owned 进程及本次 Orca 资源并删除 scratch，保留 PR 历史与证据。

已有结果：`native-wiring-01` 的27测试事件通过（含 MC5–MC6）、`adapters-final` 通过；`local-04` 在固定 `1b361a8` 构建上真实通过，两个候选与后继对应测试 PR #63/#64/#65，胜出与后继真正合并，落选晚到更新保持归属，清理完成。`local-05` 使用 `2eddcd6` 的新本机范围构建验证新增连接故障/放弃/兄弟继续/人工新增；最终逐项结果随后登记，不能用先前通过覆盖尚未执行的本机增量。

失败记录全部保留：local-01 漏 current Task/graph 版本被服务拒绝；local-02 暴露 Orca 生命周期缺 history binding，未用 raw 填产品正文；local-03 已通过正文/原会话/双PR但原生 resume 前的旧 usage 快照被测试适配器错误拒绝，等待诊断被10分钟总期限掩盖，已修为精确新 turn/started 前缀处理和即时错误报告；local-04 修复重验通过。remote-mixed-01/capabilities.json 是范围收缩前的只读/前置诊断历史（无远端资源或模型请求），不计本任务当前验收、阻塞或待办。

仅尚未执行的人工操作可登记 roadmap 后推进；实际本机功能或自动验证失败必须修复。CN5 和完整故障矩阵复用 [#37 证据](../candidate-workers/README.md)并已定向复核本次受影响接线；不默认 Full/L2 或模型矩阵。#38 CC5 仍由原记录跟踪，没有宣称通过。

## 最终结果与重跑

最终自动验收构建：`6c3e1735b7b71adba834f4fdbb2145a9881d8e12`。产品 SHA-256 `28d59e43b36f1cc132e421c52f31a53556ef9b3ef3cece6258e468b18fc9f1e8`，helper `6d7fd1d0ad497126ca256d7c7646565626abdd6c37dd18302c47e202fca0a038`。该构建包括 #73 主线、全阶段本机范围和本次 pending PR 修复；本地 manifest、home/config/目标/原 session/turn 与原 Operation 见现场记录。worker 固定 gpt-6-luna/medium、danger-full-access/never。

| 验收 | 最终证据与实际结果 |
| --- | --- |
| CN1、CN7 | [local-08/summary.json](local-08/summary.json) 与 both-waiting、逐候选 conversation/body/profile/usage：同 Task 本机 Orca + 本地 Codex 真实重叠、独立原会话反馈；四个执行（原双候选、人工替代、后继）真实正数 total 分别 912836/226626/248314/242839，均为原生累计 CLI usage，未重复相加或换算账单 |
| CN2 | 原胜出 [PR #73](https://github.com/erranliu/SteerlineTest/pull/73) 真合并 `bb353357646d4cd525403d652d55246d19c54884`；后继 [PR #75](https://github.com/erranliu/SteerlineTest/pull/75) 真合并 `343dea960ebaf24476e22ec66aa39e8361d44dd0`；同 Task 唯一 Delivery、后继 base 包含原 merge、原 Task 合并前阻塞、重复轮询不重复交付 |
| CN3、CN4 | 原反馈 command ID 重读、精确 WorkflowRun/Execution 及受控 #37 故障边界；connection-fault/disconnected-original-request/after-sibling-fault：只终止隔离 local app-server 的同一 PID/creation/executable handle，原请求保留不补投，Orca 兄弟原 session 完成下一轮与 PR 更新；等待反馈中心重启后原身份/usage保持 |
| CN5 | [native-wiring-01](native-wiring-01/summary.json) MC5/MC6 等27事件、0 fail/skip；多合并/未知/放弃后合并/晚到不覆盖 Delivery。搬包后 [native-wiring-final](native-wiring-final/summary.json) 复核相应原生接线，非全矩阵 |
| CN6 | loser-in-flight-before-merge/late-loser-retained 与原 Operations：人工替代的原 session 在胜出交付之后实际更新落选 [PR #74](https://github.com/erranliu/SteerlineTest/pull/74)，晚到正文/PR/usage不串线；物理停止与实验退出独立，未将 accepted 记为已停止 |
| CN8 | abandoned-resource-facts/manual-replacement：原 [PR #72](https://github.com/erranliu/SteerlineTest/pull/72) 及历史保留，合并前真实放弃立即退出、无需等待停止即人工新增；实际提交合并后放弃并收到拒绝。原放弃 command/stop Operation 分开记录 |
| 适配/守卫 | [adapters-final-02](adapters-final-02/summary.json) 57事件、0 fail/skip；本机 manifest 拒绝旧远端/模糊配置，精确证据与 e2e 导入边界保持。adapters-final 的 sandbox opt-in skip 不计最终证据 |
| PR pending 修复 | [pending-pr-02](pending-pr-02/summary.json) 实际 provider poll 与反馈提交屏障2项通过；原 pending command/turn 时 PR 轮询不提前恢复等待合并或重建 Decision |
| 最终 L1 | [l1-final](l1-final/summary.json) 唯一一次固定 L1，通过7组，执行220.8秒；不运行 live/Full/完整L2 |
| 清理 | local-08 scenario.json 为 passed/usage_verified/cleanup_complete 全 true；真实 PR #72/#74 已关闭，#73/#75保持 merged；本次分支、owned进程树、Orca终端/worktree/setup与scratch按原资源清单清理 |

完整索引：[acceptance.json](acceptance.json)。最终实际命令：`go run ./cmd/test-feedback -level=L3 -suite=candidates -env=live '-scenario=candidates.local' -out docs/validation/candidate-native-acceptance/local-08`；耗时1103.4秒，顶层 TestCandidateLocal 非零命中，无 fail/skip。重跑生成新空 out 和当前 HEAD 的上述本机 manifest。

local-05 的 open PR 反馈旧状态等待错误已改为原 command ID 等待；local-06 的 worker PR URL 缺 owner 被产品正确拒绝，fixture 现明确逐字完整 URL，原连接恢复仍走原身份；local-07 暴露 pending 第三轮资源创建被旧 PR 轮询重置，已增加真实 provider 定向回归并在 local-08 修复重验。pending-pr-01 是测试夹具非法 Decision 状态跳转，已按规范前值修正，未放宽产品守卫。所有失败/原始 JSON/stderr/清理证据保留，不覆盖或当作通过。

#39 尚未执行的人工操作待办为空；本轮所需测试 PR 已由操作者精确合并。活动 Spec/Plan 保留至独立复审/合入，不声明协调已合并实现 PR。
