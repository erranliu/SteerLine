# 活动中心放弃活动 Plan

对应 [Spec](../spec/activity-abandonment.spec.md) 和[测试选择](activity-abandonment.tests.json)。

1. 在项目 SQLite 保存按来源和记录 ID 定位的归档事实。沿现有命令信封和回执接入放弃。对需要停止的来源，在原业务锁与保存边界内持久提交归档和原停止意图，由原业务继续观察和结算。
2. 将归档纳入活动的 SQL 预筛、游标、计数、正式请求关联、摘要和 Web 缓存更新。抽屉及独立页共用确认交互。
3. 六个来源的原页面查询对应记录的归档状态，退出当前展示，保留精确历史和原有核实/准入入口。

| 验收 | 验证安排 |
| --- | --- |
| AA-01、AA-02 | L1/L2 基线，`go.http`、`go.storage` 验证范围、命令与读取契约，`runtime.async-activities` 覆盖抽屉和独立页，定向活动用例验证归档状态、原链接和分页。 |
| AA-03、AA-05 | `web.unit` 覆盖多消费者和缓存行为，定向验证原业务入口及续作草稿。 |
| AA-04 | `go.environment`、`go.roadmap` 保护原停止链，定向运行应用、续作建议、维护、存储测试验证归档/停止一致性。 |

在实现稳定后按[测试指南](../testing.md)运行固定基线与受影响验证。当前工作结果见[专属验收](../testing/acceptance/activity-abandonment.md)。
