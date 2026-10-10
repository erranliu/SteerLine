# 发布结果补报实现 Plan

依据 [Spec](worker-publish-report.spec.md)，验收记录：[专属文档](../acceptance/worker-publish-report.md)。

1. 增加 publish.report 步骤/短模板和结果契约，绑定原发布输入/结果与候选；只在 publish 格式错误时排队一次补报。
2. 接入报告权限冻结、前后快照校验、失败保护、对账/恢复和显式继续，不创建第二份流程状态或重试发布副作用。
3. 增加控制台名称及原发布正文查询；扩展受控模型夹具，仅从所供正文整理发布报告。
4. 执行以下最小必要自动验收，登记证据和结果。无数据库迁移或安装现场操作。

## 测试计划

| 级别 / 验收项 | 场景与命令 | 环境 / 时点 | 通过条件 / 证据 |
| --- | --- | --- | --- |
| 定向 Go / [WPR-A1、A2](../acceptance/worker-publish-report.md) | `go run ./cmd/test-feedback -- ./internal/execution -run '^(TestWorkerStage|TestWorkerPrompt)' -count=1` | 隔离 SQLite + HTTP provider；实现后 | 原直接路径不补报；格式错误只补一次；源失败、未知、工作区变更、错误 PR 不被升级；恢复/重复结果不重投；runner summary/JSONL 登记专属记录 |
| 定向 Go / [WPR-A3](../acceptance/worker-publish-report.md) | `go run ./cmd/test-feedback -- ./internal/console -run '^(TestFinalStageQuery|TestWorkerStage|TestWorkerStatus)' -count=1` | 控制台夹具；实现后 | 发布报告名称及原发布正文查询正确；runner 证据登记专属记录 |
| integration / [WPR-A4](../acceptance/worker-publish-report.md) | `go run ./cmd/test-feedback -- ./tests/integration -run '^TestWorkerStageLocal(PublishReport|RoundTrip)$' -count=1` | 既有隔离 Git/home/SQLite/controlled app-server；定向规则通过后 | 三轮直接路径及四轮补报共用会话/Attempt，候选与远端一致，报告来源固定，重启不重投；runner 证据登记专属记录 |

不跑 full/L2/L3/live：本次业务转移由规则测试及受控原生接线验证，不修改双后端原生协议或权限实现；不依赖真实模型按概率触发格式错误。不新增人工等待步骤。
