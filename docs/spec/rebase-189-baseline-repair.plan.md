# #189 后基线修复

恢复 Orca 完全访问在 composition 中的配置与 readiness 判定；历史读取缺少注入宿主时返回明确错误，测试注入真实受控宿主。修正重构后 fixture 的公开字段、原始回执、PR remote 身份和显示合同，保留原身份与拒绝变更断言。Go JSON 报告支持当前工具链的构建事件，真实构建失败仍失败。先定向验证，必要时按完整入口新暴露的问题继续修复。

## 测试计划

| 级别 | 组/场景 | 环境 | 命令 | 时点/通过条件 | 证据 |
|---|---|---|---|---|---|
| L1 定向 | composition/full-access、历史、原会话/回执、console、parser | 本机、无 token | `go test ./internal/steerlineapp ./internal/steerline/console ./internal/testfeedback ./cmd/steerline ./internal/steerline/integration -count=1`；核心按失败测试精确选择 | 修复后；保留拒绝和不重复派发断言 | baseline-repair-189/定向记录 |
| L1 | 固定最小入口 | 本机、无 token | `go run ./cmd/test-feedback -level=L1 -out=<fresh>` | 定向后，全部通过、报告完整 | baseline-repair-189/l1/ |
| Full | 完整确定性 Go | 本机、三 worker | `go run ./cmd/test-feedback -workers=3 -out=<fresh>` | 全部枚举测试/包完整、零失败；新失败先定位修复再新运行 | baseline-repair-189/full/ |
| L2 | smoke.delivery、smoke.local-codex、smoke.orca-codex | gated；完全访问，luna/medium，真实 SteerlineTest PR | `go run ./cmd/test-feedback -level=L2 -out=<fresh>` | 校验恢复后；门槛通过再真实双链路，PR 合并、usage/清理通过 | baseline-repair-189/l2/ |
| L3 | delivery.orca-codex、recovery.orca-codex | live、完全访问，luna/medium | 分别 `go run ./cmd/test-feedback -level=L3 -suite=<组> -env=live '-scenario=<ID>' -out=<fresh>` | Orca 定向后；两回合准备、原执行交付/重启、usage/清理通过 | baseline-repair-189/各场景/ |

本次选择 L1/Full 为用户明确要求，L2 和 Orca L3 覆盖修改的原生入口；不重复已通过且本次不变的本地 L3。证据根 `docs/validation/e2e-dependency-boundaries/`。每次真实运行独立现场、失败不自动重试。

Full 新暴露的问题包括重构后只读 metadata probe 与权限 admission 混用、异步 fixture 在派发前耗尽轮询次数，以及原配置的 raw policy 变更未进入 immutable pin。修复 fixture 的层级和条件等待；nativeWorkflowPin 固定整个原 profile，定向 `TestNativeClaimAllowsManualParallelButKeepsAutoSlot` 必须拒绝合法 JSON 的 policy 变更。真实 Orca probe 已原生执行成功但 exec pragma 未被 evidence 识别；只接受合法 `// @exec` 调度参数行，保留完整原命令、工具结果、身份、marker/exit 验证。新增 pragma 正向与篡改拒绝单测，`go test ./internal/steerline/orcaworker -count=1` 后重新使用独立现场运行完整 L2 与受影响 Orca L3，保留首轮失败与 usage。

异步测试驱动按真实 actor 顺序 drain 后恢复原任务，派发受理后才开始观测次数预算；重启时停止并重新绑定测试 actor 消息通道。Windows fixture 锁删除中的 AccessDenied 按同一十秒锁窗口等待，不创建新锁路径、不放宽生产权限。对应 core ER1/ER3/ER4/原终端恢复定向与 `go test ./internal/steerlineintegration -run '^TestCD65HistoryAndPreparation$' -count=1`，必须保持原 Dispatch 和清理断言。

组合回归暴露 fixture 过早为只有资源的 Session 注册 observer，占据 actor 后阻止首次 Task/Dispatch。测试驱动只为已受理 Dispatch 恢复 observer，保持生产 actor 原准备顺序。增加组合入口 `TestCD64IntentReceiptAndRecoveryGate` / `TestCD65HistoryAndPreparation` / `TestCD65SourceCommitAndHostGate` 定向复核，之后再运行完整 L1/Full，保留失败运行证据。

Windows 完整环境的多个 Git 校验共享准备阶段的命令预算；测试 fixture 明确使用产品默认 30 秒，而真实 4/10 分钟期限不变。workspace 校验保留原命令错误原因。多个组合用例总运行超过 `go test` 默认十分钟，定向组合入口使用与 Full 一致的 `-timeout=45m`，各场景自身期限不变，原十分钟超时日志保留。
