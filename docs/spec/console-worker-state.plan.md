# Plan：Worker 原始状态与控制台精简

绑定 [Spec](console-worker-state.spec.md)，结果登记[验收](../acceptance/console-worker-state.md)。直接在当前聊天实现，不修改 roadmap。

1. 在 model 定义仅含原字段的 Activity 数据并复用原查询的执行/资源映射。worker commit 在同一请求附加状态变化，成功后才通知；领取、交付复用同一数据构造器写入各自事务。覆盖普通执行、反馈、恢复、停止及终态，不新建状态表或事件框架。
2. RecentActivity 保留最近五条状态变化和十条其他活动，同快照绑定、原字节预算与类型部分索引。
3. 控制台集中原值/固定中文/颜色映射，更新列表、概览、活动、窄屏和上下文提示。长原值换行，完整诊断在详情；问题、草稿、回执及视图身份规则保持。
4. 更新受影响契约断言，补状态事务/重放/活动读取和显示最小回归，同步设计与验收文档。

## 测试计划

全部项归 [CSC-A1–A4](../acceptance/console-worker-state.md)。Windows，现有隔离 Git/SQLite/console 夹具，无模型、无安装目录。工具日志保存本次临时证据目录；摘要按 ID 登记 acceptance。

| 自动项 | 级别/组与命令 | 时点、通过条件 | 证据位置 |
| --- | --- | --- | --- |
| CSC-A1 | 定向 model/reconstruction：`go test ./internal/execution/model ./internal/execution/reconstruction -count=1`；execution 的 TestWorkerStateActivityCommitAndRollback、TestWorkerCommitRollsBackOnStaleRecordAndCheckpointFailure、TestWorkerStageTransitions | 步骤 1 后；原字段、三轮与恢复记录、同事务/回滚、不变状态不重复写、查询映射不改变 | console-model.log、execution.log、acceptance |
| CSC-A2 | 定向 SQLite：`go test ./internal/storage/sqlite -run '^Test(PS1RecentActivity\|TM05Activity)' -count=1`；TestWorkerFlowProviderDeliveryRejectsWrongPRAndClosedUnmerged、TestWorkerFlowClaimNeedsNoProjectSteps | 步骤 1–2 后；同快照/原身份、工具噪声下仍保留五条变化、领取与真实交付重放不重复 | activity.log、execution.log、acceptance |
| CSC-A3 | console 包回归：`go test ./internal/console ./internal/console/activityrender -count=1` | 步骤 3 后；原值/字段固定映射、未知值/长值/窄屏、终态/问题下变化可读，反馈/模态/焦点/迟到/离线规则保持 | console-model.log、console-final.log、acceptance |
| CSC-A4 | 受影响 query/app 定向、boundary：`go test ./internal/product/query ./internal/testkit/boundary/... -count=1`；`go vet` 受影响包，`go build ./cmd/steerline`；`python scripts/check_doc_links.py`、`git diff --check` | 交付前；公开活动接线、架构边界、构建、导航与格式通过 | boundary.log、vet.log、static.log、acceptance |

表格正则竖线仅为 Markdown 转义，实际命令用单个竖线。无需新增驱动/环境能力；不选择 Full、完整 L2/L3/live，因为状态与展示由现有无模型事务和渲染夹具验证。没有本项新增人工窗口，也不把本次回归算作已有真实模型交付验收。
