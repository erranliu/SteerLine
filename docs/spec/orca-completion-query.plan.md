# Plan：Orca 普通终端完成补查询

日期：2026-10-13。[Spec](orca-completion-query.spec.md) · [专属验收](../acceptance/orca-completion-query.md)。不维护实时结果。

## 实施步骤

1. 使用 scripts/new-debug-release.ps1 创建 C:/SteerlineDebug 独立包。扩展既有生产原生探针：支持固定 environment/worktree，先以生产启动器相同的 READY 初始化建立原主代理及 provider 基线，再在同一普通终端两次短回合中忽略状态通知，以原请求回执、基线和查询事实核验完成；保存脱敏证据，核验并清理自己的终端。探针编译当前源码的生产适配器，不以包内旧二进制代表本次实现。不能使用安装版数据库/配置/认证构造现场。
2. 复用现有 NativeProof/Turn，取消 WorkingAt 对查询及结束受理的额外门槛；保留原请求 accepted/started、原实例、基线新旧和正文核验，补必要字段解码。封套语义变化升级 codec，不新增旧记录兼容。
3. 将独立查询与订阅读取分开：订阅故障不阻断查询；查询尝试时间持久化，30 秒重试、新 done 提示及单在途共用现有观察器。不引入独立调度框架或修改 worker。
4. 在现有隔离夹具扩展最小反例、无通知/故障恢复，以及 report 未消费 working 的阶段场景；另覆盖 report 在途服务停止、原代理离线完成、订阅断开、恢复结算及第二次重启不重复。保持下一输入既有确认门槛。运行以下最小测试后执行步骤 1 的真实探针。
5. 更新被替代的文档入口，登记验收结果及证据；按用户追加授权独立复审、直接修复、复核最终差异，在最新主干上提交并创建 PR；不合并 PR、不操作安装环境。

## 测试计划

覆盖专属验收 [Q-A01–Q-A03](../acceptance/orca-completion-query.md)。新增测试/参数在实施步骤 1、4 落实后执行；零匹配、skip 和缺证据不计通过。

| 级别/场景 | 验收项 | 环境、命令、时点 | 通过条件、证据登记 |
| --- | --- | --- | --- |
| L1 定向 / Q-RULE | Q-A01 | 隔离夹具、可控时钟；实现及复审修复后：`go run ./cmd/test-feedback -- ./internal/execution/adapters/orcatransport ./internal/execution/adapters/protocol/orca -run '^Test(OrcaCompletionQuery(RecoveryClosesFailedRead)?\|OrcaNativeState(Gate\|Lifecycle\|Binding\|Identity\|Stream)\|OrcaNativeFinalRetriesMissingBodyAndQueryFailure\|OrcaObserverDeliversNativeFinalWithoutScreen\|Terminal(NativeFinalRecoveryNeverReplaysInput\|PreviousReplyCannotCompleteNewTurn\|UnknownSubmissionOnlyObservesOriginal)\|RecoveryRejectsProviderReplacementInsideOriginalTerminal)$' -count=1 -timeout=2m` | 无 working/无 done 通知可查询结算；旧记录/仅 accepted/替换/额外边界/初始化/工具正文/中断/截断不结算；缺记录可恢复，已观测冲突经过 waiting 和重建仍拒绝；恢复首读失败释放订阅。原文不变，结果只一次。runner summary/日志路径登记 Q-A01 |
| 受控集成 / Q-FLOW | Q-A02 | 现有隔离 Git/SQLite/进程夹具；实现后按下方命令分别执行 CompletionQuery、NativeStateGate 的本机/environment 子场景，每条限时 5m | 本机/environment 共用三阶段；report 无 working 仍推进，缺正文不投递下一输入，原 publish 旧报告不提前结算；输入/结果各一次。runner summary/日志登记 Q-A02 |
| 受控集成 / Q-RESTART | Q-A02 | 同一隔离夹具；复审修复后按下方命令分别执行 CompletionQueryRestart 的本机/environment 子场景，每条限时 5m | 本机/environment report 在途重启且通知断开；原 process/session/attempt 不变，离线新 done 一次结算、WorkingAt=0；第二次重启不重复结果/输入，下一输入等待既有确认。runner summary/日志登记 Q-A02 |
| 原生最小 / Q-LIVE | Q-A03 | 新生成包，真实本机及 e68f4edf-8a83-42c2-a9da-6d404d3aa3d8；实现和自动测试后：`scripts/test-orca-state-transitions.ps1 -PackageRoot <新包> -OrcaExecutable <固定CLI> -QueryOnly [-Environment <固定environment> -Worktree <本次专属原目标>]` | 两次只读输入各一次；忽略全部状态通知；查询保留旧基线并取得本轮新 done/完整正文，无伪造 working；原资源/provider 不变，终端清理确认。新包 evidence/native-production-lifecycle.json 及探针 runner summary 登记 Q-A03 |
| 静态 / Q-DIFF | Q-A01–Q-A03 | 最后 `git diff --check`，文档本地链接检查及相关包编译 | 文档、命令和实现一致；验收页记录结果和限制 |

Q-FLOW / Q-RESTART 的精确 PowerShell 命令（六条独立入口，可并行执行）：

```powershell
foreach ($completionScenario in @('CompletionQuery', 'NativeStateGate', 'CompletionQueryRestart')) {
    foreach ($completionHost in @('orca', 'orca-environment')) {
        $completionPattern = '^TestWorkerStageOrca{0}$/^{1}$' -f $completionScenario, $completionHost
        $completionReviewOutput = Join-Path $env:TEMP ('steerline-test-feedback/orca-completion-review-' + [guid]::NewGuid().ToString())
        go run ./cmd/test-feedback --out $completionReviewOutput -- ./tests/integration -run $completionPattern -count=1 -timeout=5m
        if ($LASTEXITCODE -ne 0) { throw "Failed: $completionPattern" }
    }
}
```

不跑 full、完整 L2/L3 或真实 PR 发布：本次只变完成观察和核验，受控阶段验证共享业务接线，原生短回合验证真实字段。无独立人工验收项；无法运行原生样本时 Q-A03 明确未通过，不能将受控测试代替真机能力。
