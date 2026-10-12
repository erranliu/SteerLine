# Orca 与 Codex 统一 worker 提示词

本页只归属 2026-10-10 用户直接授权的统一提示词修改。[Spec](../spec/orca-terminal-transport.spec.md) · [Plan](../spec/orca-terminal-transport.plan.md)。此前紧凑帧与 PR 验收是旧协议证据，不覆盖本次普通回复方案。

| ID | 自动验收项 | 结果与证据 |
| --- | --- | --- |
| OWP-A1 | 同一阶段的原 worker 文本不受后端改变；无结果帧/base64/nonce 追加 | 已通过。证据见下方本次记录 |
| OWP-A2 | adapter 普通回复采集、折行/空格、开始/idle/完整边界门槛、原终端恢复 | 已通过。证据见下方本次记录 |
| OWP-A3 | 新独立 SteerlineTest 环境的真实本机普通回复三阶段 | 已通过。证据见下方本次记录 |

人工项：普通会话显示可由用户后置检查，当前无人工先验门槛。未知 agent renderer 和远端原生启动准备不由本次本机样本证明。原生 CLI 屏幕不提供完整会话或原生 final 标识，结果来源为 terminal_observed，明确记录显示归一化及缺失边界限制。

## 本次执行记录

最终原生包：`C:/SteerlineDebug/dev-20261010-193346-a25eeedd-dirty`，源码基线 `a25eeedd` 加本次修改；由统一生成脚本 -Orca 创建，首次选择 live 后保持冻结。未读取/改写安装版数据库或配置，未更换安装版二进制。

- OWP-A1：受控 Codex、本机 Orca、environment 三场景通过，实际 TurnInput 与 StageInput.Prompt 相等；Orca 假 CLI 拒绝 SL_RESULT/reply_b64，额外原生输入文本与冻结值逐字比较。日志 `C:/SteerlineDebug/dev-20261010-191721-a25eeedd-dirty/evidence/stage-integration.log`，384.087s。worker 模板及 terminal_observed 正文存储/重放/来源门槛最终定向测试通过，`.../dev-20261010-193346-a25eeedd-dirty/evidence/worker-unit-final.log`，52.729s。
- OWP-A2：普通正文、中文、空格、JSON 折行、URI 重连；旧输出、错误输入、输入回显、工具输出、折叠/缺失/裁剪、未确认开始、非 idle、draft 与变化中回复均不结算；候选持久后重建 adapter 不重发。最终单元 `C:/SteerlineDebug/dev-20261010-192306-a25eeedd-dirty/evidence/adapter-unit-final2.log`，adapter 7.690s / 原语 0.900s。
- OWP-A3：最终包真实 implement.execute → implement.report → publish → await_merge，三轮均为普通回复，提示词与 frozen StageInput 完全相等，无传输指令。原 runtime `fb7953d3-29a6-4f81-b6b9-fa257bb98d04`、terminal `term_6bf27a2d-cfa0-4b19-a28d-6f2d2f765920`、incarnation `e43f28de-b7c9-4a66-a9ec-4ccd1940b87a`；run `ad07b7e1426dee6ea6789fa3f54efcd6`。源为 `orca.terminal.rendered-reply.v1`，身份 derived；不是原生 session/turn/final 接口回执。

[测试 PR #208](https://github.com/erranliu/SteerlineTest/pull/208)：专属 base `codex/debug/dev-20261010-193346-a25eeedd-dirty/base`、head `erranliu/sl-1-ad07b7e1426dee6ea6789fa3f54efcd6`、SHA `2028e1b9b6925c47a6b7aba5f3edbe2cdc55e9dc`。中心及独立 gh 查询核对一致，维持 draft/open；本次只验证三阶段/重启，不将未合并 PR 记为 TaskDelivery。

证据在最终包 evidence：`native-current.json`、`native-after-restart.json`、`native-pr.json`、`verification-summary.json`、`native-final-screen.json`、`native-final-terminal.json`、`native-final-stop.json`、`native-final-close.json`。运行/重启日志在 logs。重启保持同一终端、三轮输入和结果，没有新增投递。测试服务已停止，关闭原终端的回执只表示 PTY 停止，不扩大为所有子进程写入者不存在。

前一包 `dev-20261010-192306-a25eeedd-dirty` 三阶段到达测试 PR #207，但暴露 JSON 字符串物理行直接拼接会合并单词；这是本次修复前失败证据，未覆盖为通过。最终方案保留显示正文换行、仅重新连接 JSON 键及 URI，保留 CLI 缺少软/硬换行元数据的限制，并在新包重新验证。前一测试服务/原终端亦已停止。

当前普通回复采集只对 Codex 的公开终端渲染边界验证；其他 renderer 的 preflight 显式 unsupported。远端原生主机本次未重跑，environment 受控三阶段通过不代表远端主机启动或模型就绪已通过。

配置与控制台接线定向检查通过：`go test ./internal/app ./internal/console -run 'Terminal|Orca|CurrentExample' -count=1`；app 33.273s / console 0.878s。最终包 `evidence/config-console-unit.log`。
