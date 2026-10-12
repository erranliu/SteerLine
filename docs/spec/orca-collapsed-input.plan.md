# Plan：Orca 折叠输入回显的回复采集

依据[Spec](orca-collapsed-input.spec.md)，当前聊天直接实施，不进入 roadmap。[任务专属验收](../acceptance/orca-collapsed-input.md)。

1. 删除 renderedReply 的原提示词参数和回显全文比较；用户消息标记只用于结果分界，保留正文完整性检查及现有完成区段基线算法。
2. 补充 105 形态的 parser 回归、旧完成区段及区段外屏幕变化拒绝、替换终端拒绝；候选恢复使用折叠输入夹具。保留既有未开始、非 idle、draft、变化中正文及裁剪回归。
3. 在受控 Orca CLI 夹具中分别记录实际原始输入与折叠显示，覆盖本机及 environment 的显式启动、implement/report/publish 和重启不重发。
4. 使用 scripts/new-debug-release.ps1 创建 C:/SteerlineDebug 下的新 Orca 包。每个探针只用一个原生 Codex 终端，只发只读探针；保存原请求/开始回执、idle、真实 screen，通过生产 parser 读取结果，关闭本次终端。未开始的探针只记录事实并关闭本次原终端，不补 Enter；其他独立探针使用新终端。探针不启动业务任务或真实 PR，不读取安装版配置/数据库。
5. 登记最小验证，提交并提 PR。不更新安装环境，不自动恢复安装版 105。

## 测试计划

证据根目录为本次生成包 evidence，实时结果只登记在验收页。

| 级别/场景 ID | 自动验收项 | 环境、命令、时点 | 通过条件及证据登记 |
| --- | --- | --- | --- |
| 定向单元 rendered-reply/lifecycle | [OCI-A1/A2](../acceptance/orca-collapsed-input.md) | 既有隔离夹具；实现后运行 `go test ./internal/execution/adapters/orcatransport ./internal/execution/adapters/protocol/orca -count=1 -v` | 折叠输入及前置工具历史可采集；旧回复、未开始、非 idle、draft、变化中正文、裁剪和实例变化拒绝；保存候选后重建 adapter 不重发；adapter-unit.log |
| 受控三阶段及重启 | [OCI-A3](../acceptance/orca-collapsed-input.md) | 独立临时 Git/进程夹具；单元通过后运行 `go test ./tests/integration -run '^TestWorkerStageOrcaCollapsedInputRoundTrip$' -count=1 -timeout=8m -v` | 本机/environment 两路实际输入逐字等于 StageInput.Prompt；三轮折叠显示仍完成，启动一次、业务输入各一次，重启不重发，await_merge 无提前交付；collapsed-stage-integration.log |
| 独立原生 screen 探针 | [OCI-A1/A2](../acceptance/orca-collapsed-input.md) | 生成脚本 -Orca 创建新包后，在包项目中用公开 terminal create/send/wait/read/close 运行只读长提示词；真实 screen 交给临时 Go probe 调用生产 renderedReply | 原请求已接受/开始、精确同一终端且 idle；实际省略回显下最终正文解析通过；native-*.json、native-parser-probe.log 与探针源文件；验收页记录实际覆盖 |

不跑 full、L2 或 L3 全套：共享 worker、任务状态机、数据库、PR 交付和主机路由未修改。真实三阶段/真实 PR 与远端真机不列为本次最小门槛；environment 的显示行为由受控场景覆盖，原生样本只证明本机公开渲染形态。无人工先验门槛。
