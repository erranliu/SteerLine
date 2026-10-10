# Plan：恢复 Worker 三轮流程与统一提示词

绑定 [Spec](worker-prompt-convergence.spec.md)。直接在当前聊天实现，不调用 roadmap-runner；结果登记[专属验收](../acceptance/worker-prompt-convergence.md)。

1. 在 prompt.go 集中三个短模板与结果契约入口，StageInput 冻结任务事实；首次/反馈/恢复/重试共用渲染。报告改为紧凑业务事实，原生权限仅保留控制元数据。
2. 去掉实现轮完整 final 的前置及报告 done 对源正文完整性的要求，保留原终态、失败、快照和候选核验；报告/发布仍核对其本轮正文。
3. supervised adapter 原样传递输入，worker_done 仅保存完成消息。worker-read 按原 Task 输入段、稳定消息身份和分页采集回复，保存来源与正文；补齐迟到结果对账时 envelope 的持久化，不重派。
4. 同步流程/codec 版本、受影响测试及当前接口文档；历史 payload 方案和验收作为历史保留。

## 测试计划

全部自动项归 [WPC-A1–A4](../acceptance/worker-prompt-convergence.md)。Windows 无模型，复用现有临时 Git/SQLite/原生协议夹具，不使用安装目录。日志保存本聊天工具输出及临时证据目录，摘要登记 acceptance；Plan 不记录实时结果。

| 自动项 | 级别/组与命令 | 时点与通过条件 | 证据登记 |
| --- | --- | --- | --- |
| WPC-A1 | 定向 Go：`go test ./internal/execution -run '^TestWorker(Prompt\|FlowPrompt\|WorkspaceInstruction)' -count=1`；adapter 的 TestSupervisedResultPromptIsUnchanged | 步骤 1 后；固定任务引用保留，后端策略/身份不改变模板，附加数据不生成新模板，adapter 不追加文字 | acceptance WPC-A1、工具输出 |
| WPC-A2 | 定向 Go：`go test ./internal/execution -run '^TestWorkerStage' -count=1 -timeout=4m -v` | 步骤 2 后；三轮、缺执行正文可报告并核验候选、源失败不能发布、未知/报告重试/反馈/重开/提交失败规则通过 | acceptance WPC-A2、工具输出；增量只重跑修改场景 |
| WPC-A3 | 定向 Go：`go test ./internal/execution/adapters/orcaremote ./internal/execution/adapters/orca -count=1 -timeout=2m` | 步骤 3 后；原完成和回复分离，分页、迟到、原来源/消息绑定、裁剪及重启/冻结读取通过 | acceptance WPC-A3、工具输出 |
| WPC-A4 | 定向 integration：`go test ./tests/integration -run '^Test(WorkerStage\|SupervisedDevelopmentFlow)' -count=1 -timeout=5m`；`go test ./internal/app ./internal/console -run '^Test(Supervised\|WorkerStage)' -count=1`；`go test ./internal/testkit/boundary/... ./tests/e2e/internal/controlled ./tests/e2e/internal/evidence ./tests/e2e/internal/testenv -count=1`；`go run ./cmd/test-feedback -level=L3 -suite=delivery -env=controlled '-scenario=delivery.basic'`；`go vet` 受影响包、`go build ./cmd/steerline`、文档链接、diff 检查 | 交付前；公共领取/local 三轮推进及消费接线、supervised adapter/app 定向通过，既有退役场景保留 skip，不计三轮覆盖；源码可构建、导航无错误 | acceptance WPC-A4、临时证据日志与 runner summary.json |

表格中的正则竖线仅为 Markdown 转义，命令使用普通 `|`。不默认 Full、完整 L2/L3或 live：本次恢复已有设计，结果传输不新增外部接口或模型任务，隔离定向回归足以验证改动。没有本任务新增人工项；既有真实交付待验记录不因此算作通过。
