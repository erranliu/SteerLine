# 执行隔离规划与共享查询边界交接

日期：2026-09-30。复核基线：`origin/main@78c1872`，承接 PR #162 远程 Orca 持续 worker 和 PR #164 公共契约归包。#54 已交付；当前仅规划 #55–#61。01 两份文档保持主干内容，PR 划分与剩余硬前置不变。

## 修订内容

| 范围 | 交接要求 |
| --- | --- |
| 类型与依赖 | execution 是 #54 底层身份/动作类型的唯一所有者；contracts 记录复用 execution/question/identity 类型。contracts 和 execution 不定义两份相同值 |
| #55/#56 | workspace/publication 可用纯 execution 类型及 contracts 记录，但不导入中心/SQL 写层或业务事务 |
| #57/#58 | 本机 Orca/Codex 与新接通 remote 均迁其既有生产消费者；配置/协议分别归实现，remote/workspace 路径不混为中心本地路径 |
| #59/#60/#61 | 纯策略值/匹配归 execution；contracts 冻结记录引用 execution。保持此单向依赖，不设计循环 |
| 查询展示 | query.Service 使用四类只读端口；sqlite.Reader 采样原始 ProjectRecords，中心 projectReadRecords 组合成 ProjectView；不直接把 Reader 当完整 ProjectReader |
| roadmap | 新增 execution、contracts、query、sqlite 与 orca_remote_worker 的角色及单向依赖；#54 行为/Spec 不回写，作为已交付 #55/#56 前置 |

当前 QueryService/DTO facade 别名指向单一实现。后续只沿实际消费者收口，不为清理所有别名而重做存储和查询。命令与 worker 写事务继续由中心拥有。

## 验证范围

检查七组活动任务身份/配对及两份规划表、任务依赖无环、链接/JSON/格式和 Plan 当前测试声明；确认 01 两份文档与 #54 已合入契约未修改。只读核对 `contracts → execution`、query/sqlite ProjectView/端口及远程 Orca API 对照和规划的依赖方向。

检查通过：七组活动 Spec/Plan 配对及 DAG、273 个文档链接、44 个当前 Go 测试声明、JSON 与空白/围栏；Roadmap task ID 检查通过。`go list` 验证 execution 无项目包依赖；contracts → execution/task/taskview，query → contracts/task/taskview/taskgraph，没有 sqlite/平台依赖；diff 检查通过。

本次不实施迁移、不运行 Go 测试或远程真机认证。各后续 PR 自行验收新策略字段/严格解码；remote 回归不等于双机认证。
