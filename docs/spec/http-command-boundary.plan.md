# Plan：HTTP 服务端命令边界收敛

任务 ID：92；任务名称：HTTP 服务端命令边界收敛。

依据 [Spec](http-command-boundary.spec.md)，结果只登记[专属验收](../acceptance/http-command-boundary.md)。范围于2026-10-14经用户确认纳入[execution-map](../roadmap/execution-map.md)；本Plan安排后续实施，当前未启动。调查代码/规范版本为 `5a24abec3c10d184a6e57962e154854bc6011a9f` 的 RoadmapGuide/SpecGuide 与当前源码；未提交的其他架构修订仅辅助解释，不作为运行规格版本。

## 实施与整合

1. 核对 Server 全部产品调用、commands 的映射/登记/authority查找、openNative 和 stopReceipt。保留当前认证、调用身份、原请求与大小边界；将下面测试表列为本项行为基准，不复制旧生产实现作第二 oracle。
2. 在 httpapi 定义消费端口，按查询、命令提交/对账、原生查看实际需要分组；Prepare/Server接收这些能力，不再要求具体产品Client。保持 app 的具体组装，用实际 product/client.Client 直接满足端口，不新建通用 forwarding facade。查询页大小常量的其他清理不混入本项。
3. 建立同包命名登记/对账组件和私有 requestRecord 读写 owner。组件接收固定值、小记录端口与正式authority端口，输出原 wire response/错误；它不接收 Server/http请求或 writer。保留跨动作共同仲裁、原映射意图绑定及原锁范围。正常命令与原生查看共用必要登记基础，各自保留补交/至多一次规则。
4. handler 按原路由调用新owner；删除原重复记录/对账逻辑。HTTP Ready/auth、JSON上限/状态码留在handler，中心资格留在原产品应用。仅修改 app/execution_service_wiring.go 的endpoint构造段；不改业务提交和 #93 的客户端构造。
5. 新增 `TestHTTPCommandBoundary` 与 `TestHTTPCommandRegistry`（当前不存在，实施时补齐）的小型记录/authority stub，直接覆盖 owner能力隔离、同ID竞争、未知/读失败及副作用计数；现有真实HTTP测试保留为必要回归。结构检查验证构造/参数及旧重复路径删除，不把同包文件分开宣称为编译隔离。
6. 执行测试表与一次固定L1，在本项验收登记真实源码/dirty、构建、完整选择/命令/结果和临时证据。更新本项受影响架构/并行指南。实施交付后按真实复审/合并结果同步execution-map；本次纳入不代表已实施。

## 开工、共享文件与集成

无新增任务硬前置。#92/#93 对公共 wire/API 只读，禁止互改实现；若公共契约必须改变，先重新协调两项Spec。app 的HTTPendpoint段归#92、consoleapp构造归#93。Worker三层重构已由用户告知在另一worktree实施；本项只修改app文件中的HTTP endpoint段，不触及adapter/可靠性/actor组装或共享内部契约。保持公开DTO/产品客户端调用，后合入者在最终候选复验表中HC92-A3实际app接线。公共fixtures、httpfs/ACL和runner冻结，用本包私有stub补缺口。任务文档/验收只由本项维护；共享架构文档后合入者保留兄弟的新说明并核对组合，不创建文档代管任务。

## 测试计划

以下从仓库根目录运行。新增测试未存在，步骤5须先实现并确保实际枚举，不以零匹配通过。每行独立调用runner，其默认新目录为系统临时 `steerline-test-feedback/<运行时间>/`；记录行标识、summary.json及原始Go日志/stderr的绝对路径到专属验收“执行记录”。

| 验收项 | 级别/组或场景、环境 | 命令 | 时点与通过条件 | 证据位置 |
| --- | --- | --- | --- | --- |
| [HC92-A1](../acceptance/http-command-boundary.md#自动验收计划) | 定向owner / 新增 TestHTTPCommandBoundary；Windows普通Go，stub无SQL | `go run ./cmd/test-feedback -- ./internal/product/httpapi -run '^TestHTTPCommandBoundary$' -count=1` | 提取后、PR前；能力隔离/单一实现，真实测试数>0、无skip | runner目录；HC92-A1 / boundary |
| [HC92-A2](../acceptance/http-command-boundary.md#自动验收计划) | 定向owner / 新增 TestHTTPCommandRegistry与原登记反例；私有临时记录+authority stub | `go run ./cmd/test-feedback -- ./internal/product/httpapi -run '^(TestHTTPCommandRegistry|TestAPIAuthoritativeControlBinding|TestAPIAuthoritativeSQLiteBinding|TestAPIFirstRegistrationBindsMappedControlIntent|TestAPIFormalConsumerWinsRegistrationWindow|TestAPIFormalControlContentCollisionIsIdentityConflict|TestAPIOriginalCommandReceipt|TestAPIReconnectAndUnsupported)$' -count=1` | 接线后、PR前；原绑定/冲突/未知/补交及持久拒绝正确，副作用次数符合Spec | runner目录；HC92-A2 / registry |
| [HC92-A3](../acceptance/http-command-boundary.md#自动验收计划) | 定向HTTP / 既有API和真实app接线；隔离发现/记录/SQLite | `go run ./cmd/test-feedback -- ./internal/product/httpapi -run '^(TestTUIHTTPRoutes|TestAPIServiceLifecycle|TestAPIIdentityAndAuthentication|TestAPIRequestRecord.*)$' -count=1`；`go run ./cmd/test-feedback -- ./tests/integration -run '^(TestAPIConfiguredServiceDiscardsTaskThroughOwnedWriter|TestAPIOriginalCommandReceipt)$' -count=1` | 接线后及有共享构造变化的最终合入候选；原窗口调用一次、停服回执、大小/认证与业务owner保持 | 两个runner目录；HC92-A3 / transport、assembly |
| 必要回归，不替代上述验收 | 固定L1，隔离fixture | `go run ./cmd/test-feedback -level=L1` | PR前一次；固定范围passed/complete及边界通过 | L1 runner目录；回归记录 |

不选择Full、L2/L3/live：本项不修改worker/provider/交付或平台协议，HTTP与真实app的定向入口覆盖实际改动。无新增人工项/参与窗口；原人工项保留原任务归属。
