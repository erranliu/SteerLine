# 运行时测试进度 Plan

关联 [Spec](../spec/runtime-test-progress.spec.md)。Runner 来源为独立仓库 `aea8abe` 的公开单文件接口；本次不修改 Runner、安装环境或原 Task 18。

1. 发布调用方输入/快照类型及 capability/schema/字节核验，冻结原安排和初始对象。
2. 引入独立 candidate_progress 存储与进度观察，接入执行页；清除候选证据链，保留主干的独立 Git 历史。
3. 提供通用 PowerShell 入口及 Go 写入器，互通字节锁与原子更新；接入 SteerLine L1–L4 与业务项目既有结果判定。Plan 自动目标支持后续处理。
4. 同步文档、L3 注册、配置 fixture 和数据库 schema；在新现场验证并登记实际结果。

| 验收 | 测试及环境 | 执行时点与通过条件 |
| --- | --- | --- |
| A1/A3 | owner 定向 Go：runnerclient、testmanagement、app、sqlite；isolated | 实现后，原 Run/输入身份、字节/版本、字段严格性及存储 CAS 均通过 |
| A2 | runtimeprogress 混合 Go/PowerShell 多进程；isolated | 实现后，所有并发字段保留，重跑仅清零目标，未知字段拒绝 |
| A3 | `testing.runtime-progress`，L3 controlled | 提交前，真实产品服务/Git/SQLite，公开 Runner fixture；通过/重跑/错误与恢复/重启/原输入和业务 revision/清理完整 |
| A4 | Web build、client tests、历史 owner 回归 | 提交前，显示语义与独立历史功能均通过 |
| A5 | 业务项目 test-runtime-progress.ps1 与 test-managed-reports.ps1；controlled | 入口改造后，分开证明进度协议与原报告协议；不启动 Unity、不声称 L1–L4 业务全量通过 |
| 整体 | `l1.minimum`、`smoke.delivery`；isolated/controlled | 最终实现执行固定 L1/L2；零选择、跳过、超时及清理不明不通过 |

命令与结果登记在[专属验收](../testing/acceptance/runtime-test-progress.md)，每次使用新输出目录，首轮失败保留。Runner 自身真实远端 L3 资格以其原验收为准，不将本仓 controlled 联验当成新的远端业务执行证明。
