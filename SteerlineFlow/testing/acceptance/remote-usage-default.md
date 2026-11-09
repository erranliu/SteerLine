# 远端用量默认采集

日期：2026-11-09。用户要求取消逐日期目录授权，采集默认启用。普通开发，不进入 roadmap；未修改安装配置、进程或原候选。

## 范围与固定来源

SteerLine 基线 `221812ec058ec7dd608e447f8aeaeafb8587e376` 加本轮 dirty 修改；Runner 基线 `023dbd482f077008856ce7d278f864477cdddc21` 加本轮 dirty 修改，独立工作区 `C:\Project\SteerRunner-worktrees\default-remote-usage-20261109`，分支 `codex/default-remote-usage`。

原安装 Task 101、候选 `f3469a283116448ca21ff93a873d59f8` 对应 Runner Run `4c7f45fd3159454087c998cbe4e4952f`。只读查询确认策略仅允许 `sessions/2026/11/06`，实际来源需要同一配对环境/主机上的 `sessions/2026/11/09`，返回 `REMOTE_LOG_ACCESS_NOT_CONFIGURED`。该诊断不表示现场已经恢复。

新策略仅含 `enabled`，缺省 true。删除逐目录 grants、授权需求投影和表单。Runner 按原 Run 绑定核实来源后自动读取最小 session 叶目录；原主机身份、会话匹配、路径/链接校验、读取上限、实例许可及临时访问回收保留。旧授权字段拒绝，不增加迁移或兼容。无提示词正文/调用接线改动。

## 自动验收与结果

报告根：`C:\SteerlineDebug\wt-ae18-remote-default\`。以下结果分别判定，不把子集或构建成功当作完整通过。

| ID | 方法及通过条件 | 结果与证据 |
| --- | --- | --- |
| RUD-A01 | Runner policy/API/schema 与原采集/回收定向测试；跨日期默认允许，关闭及不可信身份/根拒绝 | 已通过；本轮 policy/collector/API/schema 断言已包含在最终 Runner 完整 L1，`runner-l1-recheck-20261109T044318903Z/l1.jsonl`；初始与扩展定向的失败边界见补充记录 |
| RUD-A02 | SteerLine 完整 L1，非零/零意外skip | 已通过，`test-results/L1/20261109T043708604Z/summary.json` |
| RUD-A03 | SteerLine 完整 L2，原候选核心交付与清理完成 | 已通过，`test-results/L2/20261109T043817922Z/summary.json` |
| RUD-A04 | `web.unit`、`go.http`；采集开关保存/待重启、有效策略只读、界面无目录授权、原HTTP回归 | 已通过，`test-results/L4/web-20261109T044149974Z/summary.json`、`test-results/L4/http-20261109T044147966Z/summary.json`；`npm --prefix web run build` 通过；应用侧只读/用量定向 `diagnostics/app-usage-20261109T044333364Z/summary.json` 通过 |
| RUD-A05 | Runner 完整 L1，原生框架和产品集合通过 | 已验收，最终 `runner-l1-recheck-20261109T044318903Z/summary.json` 为 passed（998.4s），751 个 Go 顶层测试、21 个扩展测试及 native preflight 编译/零匹配保护通过；12 个既有 helper/live opt-in 的默认 skip 按 Runner 正式规则排除，未执行 live。首次框架访问拒绝失败仍保留于 `runner-l1-20261109T043614910Z/summary.json`，详情见下文 |
| RUD-A06 | `usage.remote-codex` / live，固定新Runner/新home/冻结execute请求；默认读取、重启不重复、关闭保留、最终停止清理 | 待验收，本轮尚未执行真实模型调用；旧grant方案的历史通过不继承 |

新真实入口已更新为默认采集，不再显式grant或获取授权需求。安装部署、原候选用量回填和人工体验均未执行；本轮自动测试使用独立临时现场。

## 定向执行与失败保留

- 扩展 Runner 定向：`go test ./internal/usage/... ./internal/contracts ./internal/resources ./internal/cli ./internal/execution/adapters/protocol/orca ./internal/storage -run 'Usage|RunUsage' -count=1 -json`，证据 `runner-usage-20261109T043903461Z/`。88 项通过，7 项明确要求 live opt-in 的历史诊断未执行；storage 因本次删除旧授权需求专属测试后遗漏 strings import 编译失败，原失败保留，不能称整条命令通过。
- 修正该 import 后，`go test ./internal/storage -run Usage -count=1 -json` 通过，证据 `runner-storage-20261109T044008678Z/`。没有修改采集行为来绕过失败。
- 真实入口已用 `go test -tags=runneracceptance -c -o <新目录>/app.test.exe ./internal/app` 编译通过，证据 `diagnostics/compile-live-20261109T044234404Z/build.log`。这只证明构建，不表示执行 live 验收。
- 首次 Runner 完整 L1 的实际框架错误是记录中的文件替换 `Move(..., overwrite=true)` 报访问被拒绝，未形成预期 L2 原生 Git 前置结果。首次基线定向调用缺少 framework 的 live-catalog 初始化而退出，保留于 `runner-baseline-config-20261109T043901581Z/`；补齐原依赖后框架在干净基线上通过。第二次完整 L1 使用新目录并已完整通过，不覆盖这些证据。

## 匹配构建

`./build.ps1 -Web -RunnerRepo C:\Project\SteerRunner-worktrees\default-remote-usage-20261109 -TimingPath C:\SteerlineDebug\wt-ae18-remote-default\diagnostics\build-20261109T044855822Z.json` 已通过，生成当前工作区 `bin/steerline.exe`、`bin/steerline-web.exe` 和独立 Runner 工作区 `bin/steerrunner.exe`。仅构建，未部署。新 Runner 的 `capabilities --offline --json` 返回 `run_usage.remote_log_access_default=verified_session`；查询未启动服务。
