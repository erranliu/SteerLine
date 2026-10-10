# Orca 终端通信收敛：能力前置验证

本记录仅归属用户直接授权的 Orca adapter 收敛工作，不进入 roadmap。2026-10-10 用户确认：共用 SteerLine worker 上层流程；Orca 移除 worker-start 和 Run/Task/Dispatch/coordinator/preamble/worker_done；底层只准备会话、投递、观察、停止和清理。先验证无需 Dispatch 的原回合结果读取，能力不足时另行确认共用结果通道，不先删除当前入口交付一条只有输入的路径。

当前状态：单终端 CLI 实现已接线；用户 2026-10-10 要求直接实施、不执行验证。新版输入/结果传输未验收。下方 OTT-A1–A4 为先前公开能力探针历史，不能当作新版稳定性证明。HTTP 与文件/辅助终端方案已撤回。

## 环境与最小入口

源基线 `4da3dcb6`，未提交内容仅已有探针/验收记录。运行 `scripts/new-debug-release.ps1` 生成 `C:/SteerlineDebug/dev-20261010-145443-4da3dcb6-dirty/`，随后 `bin/debug-helper.exe debug prepare --simulate`。simulate 是包的产品方案，下面的独立原生 Orca 探针未启动产品任务或服务，不将它冒充产品交付验收。

CLI/runtime 1.4.224；runtime `fb7953d3-29a6-4f81-b6b9-fa257bb98d04`。独立 repo `88428ccb-5550-4484-9ead-591d01583842`。测试 terminal `term_557e8fe0-04ad-4142-b380-f0ef965be57a`，incarnation `83bb12fd-2b94-44e1-9da6-14694f8cdcbb`。

实际入口为本包 `evidence/session-read-probe.py`，执行快照与全部原生回执在 `evidence/session-read-probe/`。仅调用 terminal show/wait/read/send；两轮203字符提示各要求计算不同算式并返回带随机nonce的紧凑JSON文本，禁止工具、文件、Git和原生编排。submit观察30s，随后只读屏幕/stream、观察idle并核对同一incarnation。屏幕中的正确计算回复仅用于证明模型处理过输入，探针明确不把它转换成生产完成/规范final。

本次按现有默认可用代理准备夹具；代理名是CLI参数值。探针不添加代理专用参数，不解析代理私有日志，不修改权限/更新设置，没有创建任何 Orca Run/Task/Dispatch。

## 验收明细

| ID | 门槛 | 结果与证据 |
| --- | --- | --- |
| OTT-A1 | 独立环境、原runtime/terminal/incarnation和两轮输入绑定 | 通过。两轮原request分别2a63dfb4-e751-4247-8921-4270d86a3e03、b5f66f56-ddd8-40ea-b6b4-04b8a99b2552；回执input_accepted+turn_started，原processIncarnation一致。result.json及send/binding回执 |
| OTT-A2 | 无Dispatch的普通终端能处理短输入并继续第二轮 | 通过本次两轮有限样本：不同nonce和计算回复均可见，idle观察通过。screen/stream/idle回执；不宣称一般启动稳定性或同会话记忆恢复 |
| OTT-A3 | 无Dispatch公开接口提供原回合完成事件与可验证完整正文 | 未通过能力门槛。terminal read只有tail/source/cursor/裁剪与状态事实，show没有原生turn/final绑定，wait只有exit/tui-idle；结果记录canonical_turn_completed和canonical_final_reply为null。CLI command schema中完整worker-read要求dispatch，没有独立session/turn全文入口。未实现adapter |
| OTT-A4 | 清理自建terminal，不改安装环境/业务任务 | 通过。terminal-close.json确认ptyKilled=true；final-terminals.json为空。独立包保留 |

## 公开资料的核对范围

本机agent-context、CLI help及版本匹配技能是1.4.224的接口证据。[上游CLI指南](https://github.com/stablyai/orca/blob/main/skill-guides/orca-cli.md)与[会话CLI缺口issue #24392](https://github.com/stablyai/orca/issues/24392)用于交叉核对，后者主要讨论已有structured会话，不能等同于本次PTY探针。[nativeChat.readSession/subscribe代码](https://github.com/stablyai/orca/blob/main/src/main/runtime/rpc/methods/native-chat.ts)证明底层存在会话读取RPC，不证明本机CLI已经暴露该能力、它可绑定本探针terminal/请求，或已经实测恢复。

结论限定为：当前公开CLI路径尚未核实符合SteerLine原回合完成/完整回复要求的接口；不能推广为Orca所有API均无此能力。没有调用未文档化RPC、读取Orca数据库或代理日志来绕过这个门槛。

## 当时的后续决策（已由当前单终端方案取代）

已向用户提出结果通道选择：SteerLine共用worker回传契约，或者保持公开会话接口门槛并继续验证/等待平台能力。共用回传属于上层worker契约调整，须明确它与当前统一短模板“无需文件回传”的规则及Codex原生完成证据如何结合；不能在Orca adapter偷偷追加说明或重新创建Task/Dispatch。下一阶段Spec/Plan和最小测试计划以用户选定的结果通道为准。本页不把尚未选定或实现的通道登记为验收通过。

## 当前单终端实施与待验明细

最新基线 `03df9d14`；[Spec](../spec/orca-terminal-transport.spec.md) / [Plan](../spec/orca-terminal-transport.plan.md)。只使用同一 agent 终端、本机 CLI 和 environment 路由；先保存原游标、原样发短输入、沿每轮固定来源读有界结果。旧 supervised 及本次 HTTP/文件回传原型删除。长文本、全文和 native final 不属于新方案承诺。

| ID | 当前自动项 | 状态 |
| --- | --- | --- |
| OTT-A5 | 紧凑帧、原 Command、回显/歧义/超限边界 | 已实现，未运行验证 |
| OTT-A6 | 原终端身份、每轮游标持久化、一次投递与只读对账 | 已实现，未运行验证 |
| OTT-A7 | 本机/远端相同 CLI 端口、连续阶段与恢复 | 已实现，未运行验证 |
| OTT-A8 | 共用 worker 接线、旧 codec/回调字段拒绝、结构化 PR URL | 已实现，未运行验证 |
| OTT-A9 | 真机本机和远端单终端短输入/紧凑结果闭环 | 未运行，后续安排 |

实施过程中之前的 HTTP 本机三轮与 CLI 材料传输探针均为已撤回方案的实验。它们不覆盖本表；辅助终端实验结束时已有 ptyKilled 清理回执。远端模型拒绝/网络回调超时只说明当时环境，不能转记为新单终端路径的通过或失败。本次不运行测试、探针、完整编译验证或真实 PR 调试，不修改安装运行环境。
