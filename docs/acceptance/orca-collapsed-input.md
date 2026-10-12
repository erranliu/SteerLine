# Orca 折叠输入回显的回复采集验收

仅归属 2026-10-12 用户授权的本次修复。[Spec](../spec/orca-collapsed-input.spec.md) · [Plan](../spec/orca-collapsed-input.plan.md)。

| ID | 自动验收项 | 结果与证据 |
| --- | --- | --- |
| OCI-A1 | 折叠提示词及前置工具历史不阻断完整最终回复，实际投递文本不改变 | 通过。adapter-unit.log、collapsed-stage-integration.log、native-parser-probe.log |
| OCI-A2 | 原身份/发送/开始/旧完成基线/稳定 idle 门槛保留，候选恢复不重发 | 通过。adapter-unit.log、unknown-submission-unit.log；原生发送/idle/稳定 screen 见 native-render-*.json |
| OCI-A3 | 本机/environment 受控三阶段及重启，显式启动和业务输入不重复 | 通过。collapsed-stage-integration.log；本机 61.04s / environment 59.96s |

无人工先验门槛；真实 PR 交付、远端真机、安装版 105 恢复不在本次验证范围。

## 2026-10-12 执行记录

源码基线 `d54b51f74b67123ea07014686f616a99ffd95c09` 加本次修改；由统一生成脚本 -Orca 创建 `C:/SteerlineDebug/dev-20261012-104925-d54b51f7-dirty`，所有日志、公开终端样本及临时探针源码在该包 `evidence/`。生成后新增的未知投递断言另有定向日志；生产实现没有再次修改。原生只读探针使用包内新项目，不启动包内业务服务、任务或真实 PR，没有读取安装版配置、数据库或认证。

- 定向单元：`go test ./internal/execution/adapters/orcatransport ./internal/execution/adapters/protocol/orca -count=1 -v`；adapter 9.420s / 原语 0.499s。折叠输入和前置工具尾部通过，原正文/JSON/空格/中文处理保留；旧完成区段及区段外变化、实例变化、未开始、非 idle、draft、变化中正文、裁剪拒绝；折叠输入下持久化候选重建不重发。日志 `adapter-unit.log`。
- 追加未知发送回执断言：`go test ./internal/execution/adapters/orcatransport -run '^TestTerminalUnknownSubmissionOnlyObservesOriginal$' -count=1 -v`，0.640s；即使已有完整可见回复，也不能替代未知发送/开始事实。日志 `unknown-submission-unit.log`。
- 受控三阶段：`go test ./tests/integration -run '^TestWorkerStageOrcaCollapsedInputRoundTrip$' -count=1 -timeout=8m -v`，总计 125.637s。本机/environment 两路显式 agent_launch 各一次；每轮屏幕回显省略，实际投递仍逐字等于 StageInput.Prompt，三轮成功进入 await_merge，重启不重发，尚未交付。日志 `collapsed-stage-integration.log`。
- 原生样本：Orca runtime `fb7953d3-29a6-4f81-b6b9-fa257bb98d04`，terminal `term_6315789c-2bee-4a13-a9ee-41c780dbf6de`，incarnation `ade3bcde-f3c1-4208-9fc3-6d9a6f348a98`，Codex 0.162.1 / gpt-6-luna high。只读渲染探针请求 `6b267cd2-049e-4485-9c59-eed59d6cdda1` 的回执含 input_accepted/turn_started；screen 用户行以 `PRs…` 省略，工具输出折叠，最终 22 行正文和完成行完整可见。生产 renderedReply 对两份稳定 screen 均解析成功，正文以 `Probe complete: 42.` 结束，没有混入工具输出或 UI Tip。完成摘要 `1782e1556255ae63acde27de8f234ba93d7797bc286c27a314ee9d41b9ccff2c`；证据 `native-render-send.json`、`native-render-idle.json`、`native-render-screen.json`、`native-render-screen-stable.json`、`native-parser-probe.log`。临时 Go probe 源码归档为 `native_probe_test.go`，不纳入生产或默认测试。
- 前置多行长文本探针只有 input_accepted，未观察到 turn_started，仍显示 Pasted Content；保留 `native-send.json` / `native-screen.json`，未补 Enter、未重复投递，关闭自己的原终端后才开展独立单行探针。这是既有投递/开始能力的限制，不声明长文本投递已修复。

本次仅验证本机原生渲染样本与受控三阶段，不声明真实业务三阶段、真实 PR 或远端真机通过。探针终端关闭回执仅证明本次 PTY 已关闭，不扩大为所有子进程写入者退出。执行目录版本、状态 codec、固定 profile 和数据库结构保持原值，未更新安装环境。

提 PR 前重放主干 `ee47850c`（#293）。该主干增量只涉及 sandbox 验收驱动、调试模板及文档；Orca adapter、原语和本次集成夹具内容与已验证版本一致，未重复执行原生模型探针。以上原生证据仍按实际生成基线 `d54b51f7` 标注。
