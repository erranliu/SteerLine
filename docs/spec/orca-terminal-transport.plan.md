# Plan：Orca 单终端统一 worker 提示词

依据[Spec](orca-terminal-transport.spec.md)与用户最新明确要求直接实施，不进入 roadmap。[本次专属验收](../acceptance/orca-unified-worker-prompts.md)。

1. 删除 worker 的 CompactTerminalReply 分支和共享 workerreturn 帧协议；同一阶段按统一模板冻结原始输入，不按后端添加指令。
2. Orca 准备阶段固定本轮原提示词、摘要和发送前屏幕基线，投递一次并保存 request/start 事实。状态 codec 更新，不兼容旧扫描记录。
3. adapter 读取原终端 Codex 渲染边界，核对完整输入回显与最终 assistant 区段；处理显示折行、普通正文和 JSON。开始已确认、两次稳定 idle、无 draft/裁剪/缺失边界后冻结结果；未知 renderer 显式阻断，不依靠模型编码或标记。
4. 正文以 terminal_observed 来源进入共用阶段结果与会话存储，保留 derived 身份和显示归一化限制；业务 JSON、候选 SHA、PR 与交付仍由共用 worker 判断。
5. 更新假 CLI 为普通 Codex 终端屏幕，受控三阶段核对真正投递的文本；生成新的独立 Orca 调试包验证真实回复、恢复和发布阶段，不修改安装目录。

## 测试计划

只选择本次影响范围的定向单元与受控集成、一个本机真实 Orca 调试包；不跑 full/L2 全套，远端路由和原生主机模型准备不在本次变更范围，使用同一适配逻辑的受控 environment 场景验证。人工查看可后置，不作为当前自动项通过依据。

| 级别/场景 | 自动项 | 环境、命令、时点 | 通过条件、证据登记 |
| --- | --- | --- | --- |
| 定向单元 rendered-reply | [OWP-A1/A2](../acceptance/orca-unified-worker-prompts.md) | 隔离临时夹具；`go test ./internal/execution/adapters/orcatransport -count=1` 与 `go test ./internal/execution -run 'WorkerPrompt|DeclaredFinal' -count=1`；实现后 | 正文/中文/空格/折行保留，回显/历史/工具/缺失边界/未开始/非 idle 不完成，原输入未修改；日志保存 debug evidence，验收页登记 |
| 配置与控制台接线 | [OWP-A1/A2](../acceptance/orca-unified-worker-prompts.md) | 既有隔离夹具；`go test ./internal/app ./internal/console -run 'Terminal\|Orca\|CurrentExample' -count=1`；实现后 | 当前目录/codec/配置及显示接线通过，旧路由拒绝；config-console-unit.log 登记验收 |
| 受控三阶段与重启 | [OWP-A1/A2](../acceptance/orca-unified-worker-prompts.md) | 既有受控进程/Git；`go test ./tests/integration -run 'WorkerStage(LocalRoundTrip|OrcaTerminalRoundTrip|OrcaEnvironmentRoundTrip)$' -count=1 -timeout=8m`；单元通过后 | 两路径实际投递文本等于 StageInput.Prompt，普通回复推进相同三阶段，原 session 持续、重启不重发；stage-integration.log 登记验收 |
| 本机原生单终端 | [OWP-A3](../acceptance/orca-unified-worker-prompts.md) | `scripts/new-debug-release.ps1 -Orca -OrcaExecutable <固定CLI>` 创建新包；prepare → 注册本包 repo → 专属 base 推送 → run/manual start_task → screen/snapshot → 停止/清理；最终代码后 | 无 SL_RESULT/base64/nonce 指令，真实普通回复完成 implement/report/publish 并绑定专属 PR；开始/屏幕/idle、失败、结果和清理保存包 evidence，验收页登记 |

若公开渲染边界不满足真实采集，则修复 adapter 或明确保留未通过项，不能退回注入提示词。人工项：用户可查看普通会话与结果显示；不阻塞本次自动验证。
