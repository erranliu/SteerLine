# Orca 保存连接配置验收

普通配置调整，2026-10-13，关联[执行方案说明](../product-design/execution-profiles.md)。本机和远端各自保存 v2 runtime pair，执行方案引用保存环境，不将认证放入仓库。

| ID | 验收项 | 结果与证据 |
| --- | --- | --- |
| OPC-A1 | 四套安装方案及四套项目示例分别引用正确的本机/远端连接，保存认证与用户提供的 pair 一致 | 已通过；`config-verification.json`，不含认证正文 |
| OPC-A2 | 两个保存环境均能进行 CLI status 和生产原生状态认证查询 | 已通过；两份 `status-<environment-id>.json` 和 `native-state-check.jsonl`；两端均 ready/connected，`native.QueryWorktreeAgents` 均成功 |
| OPC-A3 | 新建 Orca 调试包默认绑定保存的本机连接，Windows PowerShell 5 正确解析中文名称 | 已通过；生成包 manifest/execution.json 为 `name:asus笔记本`；Windows PowerShell 5 AST 解析并按字符值断言默认名称通过 |

独立包由 `scripts/new-debug-release.ps1 -Orca -OrcaExecutable <固定CLI>` 生成，现场为 `C:/SteerlineDebug/dev-20261013-114947-da325443-dirty/`，证据位于包内 `evidence/`。没有从安装环境复制配置、数据库或认证准备调试现场；原生查询直接使用用户的 Orca 保存环境。

本机 Orca 1.4.224，runtime `fb7953d3-29a6-4f81-b6b9-fa257bb98d04`；远端 Orca 1.4.223，runtime `95d30a41-2f86-43ae-ac0d-9c304b5f1f90`。CLI 查询命令为 `orca status --environment <保存环境ID> --json`；原生查询调用生产 `native.QueryWorktreeAgents(ctx, environment, runtime)`，核对固定 runtime，不输出查询正文。

安装配置更新由用户明确授权；保留远端原环境 ID。101 仍为 open/version 1，105 仍为 in_progress/version 2，服务未重启，任务未启动或重置。新安装执行目录需要服务重启后加载。此次仅验收配置和认证访问，未运行完整业务流程、模型投递或真实 PR 交付测试。
