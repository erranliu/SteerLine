# #77 产品查询与控制台所有权

本分支基于合入 #76 后的 `main`：#219 merge `4288c43b`，当前基线 `a5fa7fc2`；分支 `codex/product-console-77`。共享接线已按用户确认的建议顺序，在 #76 实际接口落定后实施。

| 当前所有者 | 文件与接口 | 职责 |
| --- | --- | --- |
| `internal/product/api` | `pages.go`, `graph.go`, `conversation.go`, `commands.go` 等 | 可序列化页面、过滤条件、原请求/结果与错误摘要；不导出存储或 actor 记录 |
| `internal/product/client` | `ports.go`, `client.go` | 唯一 Go 产品客户端；Reader、TaskCommands、ControlPort、DecisionPort 和资源关闭 |
| `internal/product/query` | `ports.go`, `tasks.go`, `conversation.go`, `pages.go`, `project_page.go` | 注入式只读查询与 API 投影；SQLite 项目概览仍由同一 `ReadProjectView` 事务提供 |
| `internal/product/query/taskgraph` | `model.go`, `project.go`, `page.go` | 产品图过滤、分页游标与 GraphPage 投影；不持有共享存储快照类型 |
| `internal/storage/ports` | `task_graph.go` 的 `TaskGraphSnapshot` | SQLite 与执行/产品读取共享的一次采样值；不含产品过滤、游标或页面 |
| `internal/console` | 原 `internal/steerline/console` 全包 | UI 只消费 `product/api`；输入、草稿、历史、焦点、阻塞与停止行为保留 |
| `internal/steerlineapp` | `client.go`, `client_task_commands.go`, `client_discard_commands.go`, `console.go` | 唯一产品工厂和内部类型到产品 DTO 的边缘映射；写命令仍委派给 execution |
| CLI | `cmd/steerline/main.go`, `task_commands.go` | 使用产品 client/API；对候选重读并核对 execution/session/revision 后再发命令 |
| `internal/execution/reconstruction` | #76 所有 | actor 恢复重建的唯一所有者；#77 不保留 Runtime/Run/旧 query alias |

## 与 #78 的共享文件归属

本 PR 只修改 runner 的 `quickRulePackages` 产品条目，以及 `quickBoundaryAssigned` 中产品图纯所有权子例。`quickOwnerCases` 中适配器/协议/资源 package、TM06ResourceWiring/TM06AdapterBoundary 选择及具体 adapter/resource 前缀分类由 #78 拥有；合并时保留双方条目，不整文件覆盖。`coreboundary` 的 product/api 纯 DTO 规则、`internal/console` 根及闭包/反例归 #77；适配器/资源路径规则归 #78；共同 `Check` 容器合并时按实际接口保留双方检查，不放宽或新增通用白名单。

app 所有权为：`client.go` 与 `console.go` 的产品构造/DTO 转换归 #77；`directory.go`、`app.go` 的 backend/resource factory 和 `config.go` 的 resource import 路径归 #78。`storage/ports.TaskGraphSnapshot` 由 #77 唯一定义并切换 SQLite、persistence、execution 与 query 消费；#78 不复制其值或方法。后续如真实方法签名冲突，先记录具体字段/方法，再按协调顺序集成。

SQLite `Reader.TaskGraphSnapshot`、事务内 `TaskGraphSnapshotTx`、`persistence.Connection` 与 `storage/ports.TaskGraphSnapshot` 是唯一图快照接线。产品 `query.Service` 通过注入 `Store` 读取；原 SQL 和 `ReadProjectView` 同事务行为没有复制。产品 API 不携带 `JournalCursor` 扫描状态，执行日志采集状态由 #76 的 execution 命名空间持有。

关键消费者与断言：

- API/client/query/taskgraph/storage DTO 与页面：`product-02`，包含 `TestTM05ProductDTO`、`TestTM05OriginalRequest`、`TestTM05PageProjection`、规范活动身份、游标/能力和图快照组。
- 终端交互、Unicode、候选与草稿行为：`console-02`，完整 `internal/console` 包测试。
- 单事务项目读取与查询 profile 绑定：`app-query-02`、`sqlite-snapshot-01`。
- 全 OS/build-tag 源扫描和 Windows/Linux 依赖图：`boundary-01` 的 `TestCD67CoreBoundary`；实现位于 `internal/coreboundary`，精确选择位于 `cmd/test-feedback/scoped.go`。
- 真实 app/client/SQLite/受控后端命令路径：`wiring-02` 的 `TestCandidateConsoleWiring`，最终状态在报告完成后记录。

所有验收命令、环境、时点、通过条件和证据目录见 [实现计划](../../archive/spec/completed/three-line-05-product-console.plan.md) 与本目录 [证据索引](README.md)。

## 协调集成后的唯一所有者

基线更新到 #78 PR220 merge `fdb9ad75b1c5797503af9ced198732c62c9968dc`。共享 cmd/app/guard 容器由协调聊天唯一修复合并，保留产品与适配资源双方实际接口。adapter 的旧 taskview 纯值例外随该旧包删除；只保留现存 contracts→product/api 路径。

`query.ActivityReader.ReadActivityView` 消费 SQLite 的一只读事务，连接结构契约声明同一方法；app reader 委派 `query.Service.ReadActivity`，无第二次独立 Query/Events 采样、无无消费者 ActivityEvents 转发。新 SQL provider 复用原有 QuerySnapshot 和有界 event SQL；此方法由 #79 按存储边界继续承接，不能改回两个采样。

活动格式化唯一位于 `internal/console/activityrender`，只接受产品 API 并依赖纯字符宽度库；console 和 ServiceHost 直接调用，query/console 无复制或转发。它是 Spec 允许的独立纯 UI 视图，不是新的业务/客户端/SQL 白名单。守卫对 query 全标签生产 imports、新 console adapter 反例及该视图传递闭包保留禁用检查。实际 missing task 错误在 app 映射成公开 UnknownRecord，并保留原任务 error cause。
