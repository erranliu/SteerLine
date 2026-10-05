# #79 存储目录与消费者端口验证

依据：[Spec](../../spec/three-line-07-storage-ports.spec.md) · [Plan](../../spec/three-line-07-storage-ports.plan.md) · [迁移归属](ownership.md) · [测试映射](test-mapping.json)

## 结果

- 定向存储、execution、product、边界与 app wiring 组全部通过；无 skip。
- 最终 L1 通过：12 个 quick 组，431 个测试事件全部通过、0 失败、0 skip，执行 339.0 秒。摘要：[L1 summary](L1-20261005T043100Z/summary.json)。环境为 Windows/amd64，最终 L1 的短临时根为 `C:\s79l1`，每个 quick 组通过 runner 分配独立 home/temp；未启用原生验收开关。
- 全仓编译诊断 `go test -run '^$' ./...` 通过；该命令只编译，不计入行为测试证据。
- 未运行 Full、L2、L3 或 live。schema 14 和 checkpoint/runtime schema 18 均未更改。
- test-feedback 已清理每个 quick 组的隔离目录；几个人工建立的外层 TEMP 根仍留在系统临时目录。尝试删除这些根目录时，桌面环境文件操作策略拒绝了删除命令，因此没有继续绕过策略。

| 组 | 命中与结果 | 报告 |
| --- | --- | --- |
| `sqlite-01` | 11/11 通过；包含项目页/活动同事务、原子回滚、只读不建库/释放句柄和 conversation 写屏障子例 | [summary](sqlite-01-20261005T034918Z/summary.json) · [Go JSON](sqlite-01-20261005T034918Z/go-test.jsonl) |
| `execution-01` | 19/19 通过；MC1/MC7 与 ResultCAS 全部子例通过 | [summary](execution-01-20261005T040056Z/summary.json) · [Go JSON](execution-01-20261005T040056Z/go-test.jsonl) |
| `product-01` | 8/8 通过；501 候选超详情页排序通过 | [summary](product-01-20261005T040538Z/summary.json) · [Go JSON](product-01-20261005T040538Z/go-test.jsonl) |
| `storage-boundary-01` | 17/17 通过；TM04 与 TM07 storage/reconstruction 正负边界通过 | [summary](storage-boundary-01-20261005T042037Z/summary.json) · [Go JSON](storage-boundary-01-20261005T042037Z/go-test.jsonl) |
| `product-boundary-01` | 37/37 通过；CD67 core 全源码、Windows/Linux 依赖图与负向注入通过 | [summary](product-boundary-01-20261005T042043Z/summary.json) · [Go JSON](product-boundary-01-20261005T042043Z/go-test.jsonl) |
| `wiring-01` | 1/1 通过；真实 app store、窄端口和 Close 回调断言通过 | [summary](wiring-01-20261005T040545Z/summary.json) · [Go JSON](wiring-01-20261005T040545Z/go-test.jsonl) |
| `wiring-regression-01` | 11/11 通过；CD67 AdditionalImplementation lifecycle 子例通过 | [summary](wiring-regression-01-20261005T042735Z/summary.json) · [Go JSON](wiring-regression-01-20261005T042735Z/go-test.jsonl) |

最终 L1 的所有组 ID、父/子测试实际命中、各组通过数和原始 JSON 均在 [L1 summary](L1-20261005T043100Z/summary.json) 与同目录各组 `test.jsonl` 中。

## 修复和诊断轨迹

实施中保留了失败报告，不覆盖 `-out`：

1. 首次 `sqlite-01` 有 10/11 通过。回滚测试夹具在空库上读取 task graph，但未创建其所需的 native authority；已在事务前初始化当前 authority，再运行的 `sqlite-01` 为 11/11 通过。原报告：[首次 SQLite summary](sqlite-01-20261005T034628Z/summary.json)。
2. `execution-01` 的前两次诊断暴露白盒命令 fixture 的端口边界问题：包装后的 `CommandStore` 被错误要求同时实现任务命令写端口，之后测试 TaskService 又缺少固定输入和文档端口。已把 Reader、task-owned Input/文档端口与任务命令事务分开构造；最终 19/19 通过。原报告：[第一次](execution-01-20261005T034937Z/summary.json)、[第二次](execution-01-20261005T035305Z/summary.json)。
3. 第一次完整 L1 是修复前的诊断运行，`execution/reconstruction` 的 #76 守卫尚未允许读取纯 `storage/ports` 值、CD67 反向存储夹具仍放在旧 SQLite 路径，且 lifecycle 测试直接调用 core 答复时缺少 app 提供的 command ports。长 TEMP 根又让 Git worktree 在 Windows 报 `$GIT_DIR too big`。已允许 reconstruction 读取纯 storage ports（继续拒绝具体 SQLite）、迁移反向存储夹具、由 app 端测试 helper 注入 command read/write ports，并让 runner 的 quick 组使用独立短临时根；修复后的 core/product boundary 与 lifecycle 回归均通过，最终 L1 通过。两次诊断证据：[首次 L1](L1-20261005T040716Z/summary.json)、[旧路径反向夹具](product-boundary-01-20261005T041846Z/summary.json)。

具体测试选择、断言所有者、诊断报告和最终命中见 [test-mapping.json](test-mapping.json)；代码目录/类型/构造映射见 [ownership.md](ownership.md)。
