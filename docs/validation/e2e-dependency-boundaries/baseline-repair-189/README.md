# #189 后基线修复验收（2026-10-02 / 03）

父分支基于 main a670e9d / #189。完整验收固定候选 **026621f**；末尾 **03f01dc** 仅强化一项历史轮询 fixture 的 SQLite 文件清理及 Plan，不改变生产代码或测试断言。L2 / Orca L3 分别在 9ac3852 / f05b9ff 上验证；后续生产增量只保留 Git 校验错误 cause，真实执行和原生证据语义不变。

## 最终结果

| 入口 | 结果 | 证据 |
|---|---|---|
| L1 固定最小入口 | PASS、complete=true，99 条含子测试结果通过，零 skip/fail；最终清理候选 03f01dc 上 61.50s | l1/summary.json；l1-fixed 保留 026621f 的 80.21s |
| Full 三 worker | **PASS、complete=true**，906 条含子测试结果通过，19 项登记 native opt-in/helper skip，零失败；核心全部 310 个顶层终态、全部 35 个唯一 Go 包完整；执行 2366.75s（约 39m27s），准备 9.84s | full/summary.json、full/results.json |
| 完整 L2 | PASS，3/3、coverage full-L2、cleanup_complete=true；631.4s | l2/summary.json |
| L3 delivery.orca-codex | PASS、cleanup_complete=true；264.0s | delivery.orca-codex/ |
| L3 recovery.orca-codex | PASS、cleanup_complete=true；400.0s | recovery.orca-codex/ |
| workspace / Orca evidence / parser / 依赖边界 | PASS，无 token | 本次命令记录 |

Full 的 906 个 pass 包含父子测试，不是 906 个互不重复场景；19 个 skip 为已登记的历史 helper 或原生 opt-in，L2/L3 场景无 skip。完整核心 295 pass + 15 skip，其他包顶层 279 pass + 4 skip。所有枚举、包终态和测试终态核对完成，无 panic、构建错误或遗漏。没有把失败运行的局部结果拼接成最终 Full。

L2 的 [SteerlineTest PR #43](https://github.com/erranliu/SteerlineTest/pull/43) / [#44](https://github.com/erranliu/SteerlineTest/pull/44) 均 MERGED。本次项目、任务、服务及原生资源由场景独立创建和清理；已完成 cleanup，owned Git refs 回收，模型仍为 gpt-6-luna / medium、完全访问。

成功原生累计 usage：L2 local **82,073**、L2 Orca **315,598**、L3 Orca delivery **248,280**、L3 Orca recovery **203,506**，合计 **849,457 tokens**。recovery 的 before / after-restart / final / cleanup 均为 203,506，重启和交付轮询没有新增回合或 usage。首轮失败的 L2 也保留：local 64,273（PR #42 MERGED）、Orca readiness 52,216，总 **116,489**；本次所有读取到的原生累计合计 **965,946**，不重复相加同一会话的中间/清理样本，不换算账单金额。

## 修复

- Composition 允许 Orca 的 workspace / host 写范围；完全访问在 Windows 仍经过 readiness / business 两回合。
- exec evidence 只剥离合法的原生 `// @exec` 调度参数行，继续核对完整原命令、call/source 身份、nonce + script hash、marker 和 exit code。正向及篡改拒绝单测通过；首轮真实 probe 本已执行成功但 pragma 未被识别，保留该失败。
- profile 选择与 claim 使用同一冻结 envelope digest；native workflow pin 固定整个 profile，合法 JSON 的 policy 变更也必须被拒绝。原会话失去 policy receipt 只有明确的 pending recovery seed 可以通过。
- 历史读取要求注入 source/managed host，缺失时返回错误；fixture 注入正确端口，消除 nil pointer panic。直接服务入口在任何效果前校验 native authority。
- Go JSON parser 支持当前工具链 build-output/build-fail，真实构建失败仍失败；完整包/测试计数核对保留。
- 修正配置 version 2、provider remote、显式 publisher、codec payload、固定目标/原始回执、通用显示合同和 fixture 端口；权限 admission 与只读目标 metadata 测试分开，未启用不具备 enforcement evidence 的远端执行。
- 异步 fixture 在原资源准备完成后才观察已受理 Dispatch；按 actor 顺序推进原准备/回执，等待真实条件，不重派或放宽断言。fixture 使用产品默认 30 秒命令预算，真实 4/10 分钟期限不变。hold 的原工具调用没有伪造完成结果。
- workspace Git 校验保留原命令错误 cause，避免把超时报成 base 移动。Windows fixture 锁的 delete-pending AccessDenied 在同一十秒窗口等待。

## 保留的诊断

full-first / full-second 是两次失败的诊断运行，不能当作验收通过。独立 combo 的三个组合本体通过，但历史轮询子测试在 Windows SQLite 临时文件清理时失败；该运行仍记录 FAIL。03f01dc 只对这个 fixture 自己的 database 文件在 store/collector 关闭后进行两秒有界清理，定向复核 PASS。Full 固定候选 026621f 已单次完整通过；不因这个纯清理增量重复支付真实模型或再跑约四十分钟 Full。

首轮组合命令采用 go test 默认十分钟总期限而中止；之后使用与 Full 一致的 -timeout=45m 复核，各场景原期限保持。所有原始目录及精确 source/build SHA、命令、耗时、选择、cleanup 与失败详见 runs.json / summary.json；归档不复制认证、数据库、原始 prompt 或 dispatch capability。

## 命令

```powershell
go run ./cmd/test-feedback -level=L1 -out=<fresh>
go run ./cmd/test-feedback -workers=3 -out=<fresh>
go run ./cmd/test-feedback -level=L2 -out=<fresh>
go run ./cmd/test-feedback -level=L3 -suite=delivery -env=live '-scenario=delivery.orca-codex' -out=<fresh>
go run ./cmd/test-feedback -level=L3 -suite=recovery -env=live '-scenario=recovery.orca-codex' -out=<fresh>
go test ./internal/steerline -run '^(TestCD65HistoryAndPreparation|TestCD65SourceCommitAndHostGate|TestCD64IntentReceiptAndRecoveryGate)$' -timeout=45m -count=1
go test ./internal/steerline -run '^TestCD65SourceCommitAndHostGate/active_and_historical_source_budgets_rotate$' -count=1
go test ./internal/steerline/workspace ./internal/steerline/orcaworker ./internal/testfeedback ./internal/testboundary -count=1
```

本地 L3 两项在上一轮已通过，本次完整 L2 覆盖本地 claim/交付，针对修改的 Orca 链路独立验证交付和恢复；没有宣称本轮重跑了全部六项 L3。

清理增量后的最后 L1（03f01dc）再次 PASS、complete=true，99 pass / 0 fail / 0 skip。working tree 当时只有待归档证据文件，不含实现修改。cleanup/results.json 保存历史轮询清理定向通过；不重复 Full 的说明见上。
