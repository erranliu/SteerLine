# Orca 延迟启动状态同步修复

仅归属本聊天对 SteerlineTest 105 启动后仍显示“启动中”的直接修复，不进入 roadmap。

## 现场与范围

WorkflowRun `027baa0b696c2caaf9d075e65b3ff584` 的原 Dispatch 为 `ctx_6bc0b03922b7`。首次派发留下 `turn_start_unobserved` 回执；在原终端补发一次 Enter 后，公开 `worker-show` 已报告 `activity=working`、`liveStatus=fresh` 和原进程身份，但服务仍保留 pending 输入。原服务直到北京时间 2026-10-09 15:56:33 收到完成消息时才补记开始，15:56:37 收尾，随后进入 `waiting_pr_merge`。本次不提交运行数据库或原业务提示。

修复使原 worker 的新鲜活动证据在完成消息之前推进启动状态。核对 Run、Task、Dispatch、runtime、terminal、incarnation、worktree；要求精确原 worker、连接有效、未退出，以及输入意图之后的 `agent_status` 活动证据。仅输入已接受、空闲、过期证据或缺少活动字段均不证明启动。记录开始时同步持久化原 turn 的 Started 标记，不重新派发、不补发业务文本。

## 测试计划与验收

| ID | 级别 / 场景 | 环境、命令、时点 | 通过条件 | 证据位置 / 结果 |
| --- | --- | --- | --- | --- |
| ODS-A1 | 定向 Go / 延迟活动与 observer | Windows，无模型；实现后 `go test ./internal/execution/adapters/orca -run 'TestOrca(DelayedStartRequiresFreshOriginalActivity\|ObserverRecordsDelayedStartBeforeWorkerDone\|AcceptedUnobservedStartPreservesWorker\|UnobservedStartRequiresDurableAcceptance)$' -count=1` | 初始空闲不推进；随后 working 在无完成消息时发出唯一开始事件并保存启动事实；旧证据、错误身份、不可达不成为开始证明；不重发或关闭 worker | 本聊天终端输出；通过，2.200s |
| ODS-A2 | 定向 Go / adapter 与控制台回归 | Windows，无模型；实现后 `go test ./internal/execution/adapters/orca ./internal/console -count=1` | adapter 启动、等待输入、结算及控制台 pending/active 状态映射通过 | 本聊天终端输出；通过，adapter 6.214s、console 3.703s |
| ODS-A3 | L3 controlled / `recovery.orca-startup.unobserved` | Windows，无模型；实现后 `go run ./cmd/test-feedback -level=L3 -suite=recovery -env=controlled '-scenario=recovery.orca-startup.unobserved'` | 已接受未观察到开始的原 Dispatch 正常结算，重启不重派，清理通过 | 通过；场景 144.80s，summary 与 scenario 均为 passed=true、cleanup_complete=true；报告 `C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T080220.796155500Z/summary.json` |
| ODS-A4 | 定向 Go / 中心开始回执 | Windows，无模型；实现后 `go test ./internal/execution -run '^(TestAppServerStartedEventCommitsOperationReceipt\|Test.*Worker.*Receipt.*\|TestReleasedWorkerStartConvergesAcrossRestart)$' -count=1` | 原输入开始事实提交后推进中心状态；原回执和失败释放恢复不被改变 | 本聊天终端输出；通过，49.687s |

未选择 Full、L2 或其他 L3 组：改动限于本机 Orca 延迟开始事实及其持久化，不改变任务内容、发布或合并契约。ODS-A3 验证启动异常恢复接线；延迟活动先于完成的具体行为由 ODS-A1 验证，不将受控场景说成真实模型验收。

安装目录二进制和正在运行的服务未更新；安装版仍需正常构建安装后才会使用源码修复。105 现场已经恢复并到达待合并，本次不重跑原任务。

文档链接检查通过（286 文件、1778 本地导航链接、0 错误）；差异空白检查通过。

## 单独 PR 提交前复验

2026-10-09 将本修复单独更新到最新主干 `ac9114f74c09f1bbf9d0ed3927b816d0bdf0c726`。提交范围仅 observer 的延迟开始事实、原身份活动校验、对应回归及本验收记录/导航；structured 方案和探测草稿未纳入。

`go test ./internal/execution/adapters/orca ./internal/console -count=1` 通过（5.184s / 2.812s）。文档检查通过（291 文件、1813 本地导航链接、0 错误），PR 差异空白检查通过。上表 ODS-A1–A4 的原始结果属于前述基线，ODS-A3 没有在新主干重复执行。

包含 `TestReleasedWorkerStartConvergesAcrossRestart` 的 ODS-A4 最新复验失败：`failed_start_test.go:59` 报 `worker service-stop behavior differs from the frozen execution`。将唯一生产改动文件恢复为主干版本后单独执行该测试，同样失败（13.119s）；其 neutral lifecycle fixture 与当前冻结停服声明不一致，作为主干已有阻塞记录，不扩入本状态同步修复。

其余中心测试 `go test ./internal/execution -run '^(TestAppServerStartedEventCommitsOperationReceipt|Test.*Worker.*Receipt.*)$' -count=1` 通过（44.518s），实际命中 4 项：开始事件提交、ER4 超时/重启/丢回执、回执失败保留意图、提交回滚。最新 ODS-A4 仅这 4 项复验通过，不能标整组通过。

## 首次 Enter 未生效调查

检查安装版 Orca 1.4.223 的 `resources/app.asar` 中 `out/main/index.js` 及持久化 mutation receipt。SteerLine 调用原生 `orchestration.workerStart`；提示提交和 Enter 由 Orca 内部负责。原请求 `f625ce84-4d8d-4e69-81e9-19951bbdd5af` 的已保存回执只有 input_accepted，没有 turn_started。

安装代码中，dispatch-preamble 路径经 `sendTerminalAgentPrompt` 和 `writeTerminalAgentPrompt` 提交；后者在 `ptyController.write(pty, '\r', inputKind)` 返回成功之后才构造 input_accepted。因此现有证据不支持“根本没有调用 Enter 写入”。写入接口的成功不等于 TUI 已消费该按键。

发现明确的时序缺口：此路径的选项不包含 composerReady；其 render gate 依据显示光标的转义序列及静默计时放行，而不核对完整粘贴草稿是否处理完毕。Codex 配置的 submitRetryDelayMs=1200 只在 composerReady 路径使用，worker-start 的 dispatch-preamble 路径不会进入该延迟重试。另一次补发 Enter 后原草稿立即执行，与首次提交过早或未被消费的解释相符。

上述时序缺口已由安装代码确认，但本次原始日志不含首次 Enter 在 PTY/TUI 内的逐键投递与消费时刻，不能宣称已证明具体丢失层或完整根因。Orca 提交时序尚未修改或独立复现；前面的源码修复仅解决 SteerLine 在真实启动之后未推进状态的问题。现有 live 测试驱动的粘贴稳定后补 Enter 属于测试侧适配，不构成产品端修复。

## 文档复核：原生 structured worker

按用户要求复核 Orca 1.4.223 的版本匹配 orchestration 指南、worker-start 帮助及[官方指南](https://github.com/stablyai/orca/blob/main/skill-guides/orchestration.md)。worker-start 是组合就绪、输入投递和资源所有权的正式入口；有无真实 terminal 均应通过原生 worker-show/read 和消息接口管理。运行类型遵循用户的新 agent 设置，没有本次调用的模式覆盖参数。

Orca 提供实际 structured session 的任务投递路径，由 host send/settlement 接口投递消息并读取 accepted/pending/queued 结果，而不是通过 PTY 粘贴和回车提交。这是与首次 Enter 时序分开的原生执行路线；不能将[Chat UI 视图](https://github.com/stablyai/orca/blob/main/docs/site/content/docs/agents/native-chat.mdx)等同于无 PTY 的 structured worker。

安装版实现的 worker-start 已解析 `--terminal orca_session_id:<id>`，验证原 chat 所属 workspace，并在原 chat 上投递新的 Dispatch；原 session 的发送路径使用 structured host。与此同时 SteerLine 的 `worker_start.go` 仍拒绝 mode != terminal，并要求 worker-show 返回真实 terminal；仓库原 Spec 关于尚无 structured 同会话接续能力的限制需要按现有平台重新核实，不能继续仅凭旧说明断言 Orca 没有这条路线。

本轮仅完成文档与安装代码核对，没有切换用户全局模式、启用 SteerLine structured 支持或执行 structured 多轮/重启/清理验收；因此不宣称稳定性实测通过。后续应优先核验并接入原生 structured worker 完整流程，而不是把人工补 Enter 或猜测延时作为正式投递协议。
