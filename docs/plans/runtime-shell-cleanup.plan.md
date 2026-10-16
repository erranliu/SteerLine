# 旧执行记录与空壳字段清理

2026-10-17，用户直接授权当前聊天实现并提 PR；普通开发，不占 Task ID、不改 roadmap。代码基线 `2fb07ba1`，包含环境检查启动配置与报告刷新修复。范围与实时结果见[专属验收](../acceptance/runtime-shell-cleanup.md)。

## 实施范围

1. 核对全部 Go 源码（包括平台文件、build tag 和测试）的符号引用；删除三个无调用函数 `workflowSummary`、`executionDetails`、`executionSessionFacts`。
2. 删除 `reconstruction.Runtime` 无生产填充/读取的 WorkflowRuns、Executions、Attempts、Decisions、Operations，以及 RunView 无消费者的 ManualStop、IdentityMapDigest、DomainAttemptNumber、WorkflowConfigDigest、StepResultPath、WorkspaceIntent。删除失去全部消费者的旧记录类型、私有解码路径及其专属辅助类型。
3. 保留 TaskDelivery、Task/DAG、ServiceCheckpoint、当前 RunView/CandidateView 与产品 DTO、问题/输入、活动/会话和宿主进程能力；同名 TaskRecord.IdentityMapDigest 的拒绝旧身份规则保留。只删除已断开的引用闭包，不扩大为所有无写入展示字段清理。
4. 校准架构、数据模型、Spec/验收导航与测试入口；按现有验收记录准确区分环境检查实现、公开契约、固定版本本机 full_access 正例和剩余原生/人工范围。历史 Spec/Plan 和原始验收事实不改写。

不移动模型归属，不合并读取链，不改变数据库/schema、公开 HTTP/CLI、原请求、交付、停止、恢复或授权行为。内部 Runtime 不是持久格式：存储读写 ServiceCheckpoint，再由 taskrunner.Project 组合公开事实。[Worker 生命周期](../product-design/worker-lifecycle.md) 的 W/E/V 规则及实现缺口不变。

## 测试计划

| 自动验收项 | 级别/组 | 环境、命令与时点 | 通过条件 | 证据位置 |
| --- | --- | --- | --- | --- |
| [RSC-A01](../acceptance/runtime-shell-cleanup.md#rsc-a01) | 全源码引用审计 | 删除前后对全仓 Go 源码检索精确符号及结构字段，核对平台文件、测试和 ServiceCheckpoint 持久化入口 | 删除对象无有效消费者；保留当前读取、业务与宿主边界；守卫中的旧类型禁用名单继续保留 | 专属验收删除/保留清单与审计结果 |
| [RSC-A02](../acceptance/runtime-shell-cleanup.md#rsc-a02) | 默认测试编译、固定 L1 | 清理后 Windows 隔离 fixture；`go run ./cmd/test-feedback -build-only`；`go run ./cmd/test-feedback -level=L1` | 编译 complete/passed；L1 非零匹配、complete/passed、零失败/跳过，包含跨视图、checkpoint/存储与分层边界 | runner 新临时目录的 summary.json；编译日志 go-test.jsonl、L1 分组 test.jsonl，登记于专属验收 |
| [RSC-A03](../acceptance/runtime-shell-cleanup.md#rsc-a03) | 文档/差异静态检查 | 文档同步后 `python scripts/check_doc_links.py`；`git diff --check`；提 PR 前自查完整 diff | 链接及差异无错误；状态表述与已有证据版本/范围相符 | 专属验收 |

本次没有新增运行行为或外部执行边界，复用现有验证，不新增仅镜像删除清单的测试。L2/L3/L4、Full、Web 测试、真实模型和调试包不选：产品/前端契约及执行流程未改，固定 L1 与默认测试编译覆盖本次类型删除风险。无需人工验收，不使用安装环境。
