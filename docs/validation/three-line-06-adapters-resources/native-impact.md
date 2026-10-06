# #78 Native impact 与 #39 证据复用

基线为 `4288c43b05287554de0b542e9eeb90d366fe063f`。本任务移包和真实 consumer/factory 接线不修改执行协议、原生参数、权限、资源身份、PR 提交或停止能力。#76 已迁入的 process facts 是 `internal/execution/process` 的唯一事实实现；原 processhost OS 启动/文件功能并入同一包，重复的旧实现删除。

| 入口/行为 | 本次实际变化 | 原生影响与证据决定 |
| --- | --- | --- |
| Codex app-server / session launcher | 唯一 codec 移至 `adapters/protocol/codexapp`，app 调用 Codex adapter 的真实新路径 | JSON-RPC/WebSocket 帧、handshake、原 thread/turn/session/process identity、恢复原请求、native command/permission 参数均不变。复用 #39 [native wiring](../candidate-native-acceptance/native-wiring-final/summary.json) 与最终 [本机候选结果](../candidate-native-acceptance/local-08/summary.json)；本次另跑 TM06 Codex/protocol 定向 parents 和真实 app factory 子进程验证。 |
| 本机 Orca terminal | adapter 与 app registration 移至新包 | terminal/runtime/agent/incarnation/Dispatch/worktree binding、stop/cleanup 意图与确认语义不变。复用 #39 native wiring 与 local-08 原 terminal/session evidence；本次跑固定绑定、替代 incarnation 和 agent-detection 定向测试。 |
| Orca remote 存量代码 | 代码路径和四个无消费者 verify JSON fixture 路径迁移；拒绝保护仍在原 adapter | 没有远端资格、权限或 real admission 变化；不开展真实 remote 操作，也不声称认证。当前只跑 codec/version/preflight/identity/config refusal parents 与 Windows/Linux compile/dependency graph。 |
| Process Host 与 process facts | `Host`、受控输出、原子文件写入和替换移入已有 `execution/process`；OS `StartManaged`/identity facts 复用 #76 唯一实现 | Windows 仍使用原 suspended start、Job Object/KILL_ON_JOB_CLOSE 与 process-group flags；Linux 仍使用原 Setpgid/Pdeathsig；generation、文件权限与缺失身份诊断保持。Linux 旧重复 processhost 对 malformed `/proc/stat` 曾当作不存在；统一采用 #76 facts 的 unknown/error，避免把未核实身份当作已退出。该拒绝语义由既有唯一 facts 提供，未改原生启动/停止参数；Linux 本次只有源码/依赖图检查，没有原生运行证据。`TestTM06ResourceWiring` 对隔离本机子进程、文件及真实 lifecycle factory 做定向核验。复用 #39 [同一 PID/creation/executable handle 证据](../candidate-native-acceptance/local-08/summary.json)。 |
| Git workspace 与 PR publication | package/import/default factory 路径迁移 | 不改命令参数、仓库/PR provider、PR URL/base/head/merge identity、发布责任、资源创建或清理能力。复用 #39 CN2/CN5 evidence；本次跑原 workspace、PR identity/merge receipt 与 unsupported-provider 拒绝代表。 |
| launcher、OS tags 与 L1 runner | 测试分组 package path 更新；移动的 adapter OS-tagged files 与资源路径按真实目录编译 | launcher argv/embed assets 与配置格式未改变；当前 App runner 的唯一 process-host 与 session-launcher 调用已改线。静态 source scan 覆盖所有 OS/build tags；L1 `TestCD67CoreBoundary` 执行 Windows/Linux 生产传递图。 |

## 复用证据

- #39 的原生 wiring 和候选会话/资源证据：`docs/validation/candidate-native-acceptance/native-wiring-final/summary.json`、`local-08/summary.json`。它们覆盖本地 Codex/本机 Orca 原 session、process tree、binding、候选交付和清理；本次移动没有改变这些值、命令或权限。
- #39 的固定 L1 参考：`docs/validation/candidate-native-acceptance/l1-final/summary.json`。#78 仍按本 Plan 对当前新构造路径实际运行一次 L1，并保存为独立证据，不把历史报告当作本次 L1。
- 本次定向证据计划见 [Plan](../../archive/spec/completed/three-line-06-adapters-resources.plan.md)；实际 run/pass/skip、Go 版本、命令、耗时与输出应以同目录 `README.md` 和各自 test-feedback `summary.json` 为准。

未选择 Full/L2/L3/live：纯目录移动没有变化到模型接入、权限、原生协议、PR 网络/provider 或真实后端能力；第一阶段 remote remains refusal-only。只在隔离临时目录用本地子进程验证资源 Host，不替换用户/现场 executable 或配置。
