# 验证：Orca 本机原生状态通道

2026-10-14，归属普通开发验证“本机 Orca 与远程 Orca 的连接差异”。本次只验证能力，不实现新的本机观察模式，不更新安装版程序、执行方案或任务状态。

## 环境与范围

- Orca 1.4.224、Codex CLI 0.159.2，Windows 本机。
- 先运行 `scripts/new-debug-release.ps1 -Orca -OrcaEnvironment '' -OrcaExecutable <固定 CLI>`，成功生成 `C:\SteerlineDebug\dev-20261014-123419-ca6590b0-dirty`，不启动包的业务服务，不发布 Git 分支或 PR。
- 首次生成被旧测试驱动引用已删除的 ControlPayload 字段阻断；修正 driver/candidate.go 的过期候选弃用分支及字段后，重新生成成功。产品执行逻辑未改变。`go test ./tests/e2e/internal/driver -run '^$'` 编译通过。
- 本机 CLI 与命名管道均不指定 environment 或 pairing code，不保存本机远程配对项。命名管道从本机运行时元数据发现，使用本机 authToken；令牌仅在内存中使用，不写入证据或执行方案。
- 测试 TUI 使用隔离 CODEX_HOME，只复制用户登录认证和 Orca 状态钩子；仅复用用户已批准的、内容不变的 Orca hook 信任摘要，最终使用的钩子仅来自 Orca，未复制用户全局模型配置或安装版认证。model=gpt-6-luna、effort=medium、sandbox=read-only；初始化 READY，随后两轮只回复标记、不使用工具或改变文件。

## 自动验证项与结果

| ID / 场景 | 验证方式 | 实际结果与证据 |
| --- | --- | --- |
| LNC-A01 / 本机命名管道查询 | 用与原生 CLI 相同的 NDJSON/authToken 请求格式直接调用 status.get、session.tabs.list、worktree.ps；核对 request/runtime ID | 通过。三项均 ok=true；没有远程主机登记或配对。`evidence/local-transport-probe.cjs`、`local-transport-result.json` |
| LNC-A02 / 原始主代理两轮状态 | 原终端各投递一次；每轮确认原生 input_accepted/turn_started，查询捕获 working→done，核对新时间、唯一 pane、terminal incarnation 和 provider session，并取得本轮最终原文 | 通过。LOCAL_PIPE_A / B 分别观察 17 / 14 个样本；两轮最终原文准确、身份保持。`local-agent-roundtrip.cjs`、`local-agent-roundtrip.json`、两轮 metadata/agents JSON |
| LNC-A03 / 当前生产解码器兼容性 | `STEERLINE_ORCA_LOCAL_PIPE_EVIDENCE=<本包 evidence> go test ./internal/execution/adapters/protocol/orca -run '^TestOrcaLocalPipeNativeEvidence$' -count=1 -v` | 通过。ParseEnvelope、DecodeOriginalState、DecodeOriginalAgent 直接读取真实命名管道响应，保留原身份、主代理 done 和最终原文；两个子用例均执行、无 skip。`production-decoders.log` |
| LNC-A04 / 无配对本机订阅边界 | 命名管道调用 session.tabs.subscribe；另探测本机 WebSocket 是否接受普通本机 owner 请求 | 命名管道明确返回 method_not_supported：requires a streaming transport。本机 WebSocket 普通 owner 请求被 4001/Invalid e2ee_hello 拒绝；这只证明该请求格式不能直接复用，不能据此声称不存在其他可用的本机流式认证方式。本次未验证出无需配对的订阅通道。`local-transport-result.json`、`local-ws-probe.cjs`、`local-ws-result.json` |
| LNC-A05 / 资源与写入边界 | 关闭本次三个测试终端；最终 terminal list、fixture git status | 通过，当前测试工作区终端清单为空、Git 无变化。`terminal-close-native.json`、`terminal-list-after-cleanup.json`；前两个试验终端关闭均返回 ptyKilled=true |

全部证据及探针脚本位于上述新包的 `evidence/`。包目录是私有测试材料，含隔离登录认证，不能整体发布。

## 结论与未覆盖范围

本机状态查询提供了现有主代理判定所需的数据，当前生产解码器可共用；本机与远端的连接、认证和观察方式需要区分。不能只删除本机方案 environment：现有生产 StateFeed 仍要求订阅，而本机命名管道不支持该订阅方法。

若采用本机查询模式，后续需明确查询时点/频率、短回合的开始/结束证明、断连与恢复规则，再接入当前 lifecycle；若要求两端都用推送订阅，需要先找到并验证 Orca 的本机流式接口。本次两轮样本不等于业务三阶段、真实 PR 交付、重启恢复、极短回合或故障窗口验收，也不证明用户报告的 Orca 自配对 bug。

测试期间没有把本机加入远程主机清单，没有修改 C:\SteerlineRuntime。生成后的原生测试会话使用自己的 fixture；补齐状态钩子后启动了新的终端，不把第一轮缺钩子的空状态当成能力通过。

附加编译检查：`go test ./tests/e2e/internal/testenv -run ^$` 仍被既有 `candidates.go:408` 的 WorkflowRunRecord.Abandonment 引用阻断，证据为 testenv-compile.log；本次未扩展修复旧候选场景。该失败不计通过，也不以两轮本机样本宣称完整 E2E 已通过。
