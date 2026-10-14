# Spec：HTTP 服务端命令边界收敛

任务 ID：92；任务名称：HTTP 服务端命令边界收敛。

状态：2026-10-14 草稿规划，未纳入执行、未实施。配套 [Plan](http-command-boundary.plan.md)、[专属验收](../acceptance/http-command-boundary.md)、[本轮草稿](../roadmap/drafts/roadmap.md)。调查基线 `5a24abec3c10d184a6e57962e154854bc6011a9f`；实际启动另固定已提交 Spec/Plan 版本及最新主干。

## 问题与成果

[endpoint.go](../../internal/product/httpapi/endpoint.go) 的 Server 持有具体 product/client.Client；[commands.go](../../internal/product/httpapi/commands.go) 混合 HTTP 解码、信封登记、动作映射和 authority 对账；[console_routes.go](../../internal/product/httpapi/console_routes.go) 的原生查看另有登记路径。新增 handler 或原请求处理需要同时理解这些职责。

Server 改为消费查询、命令和原生查看所需的小端口。提取同包的命令登记/对账 owner，接收固定信封、认证所得调用身份及必要应用/记录端口，返回已有 CommandResponse 或类型化错误；不接收 Server、ResponseWriter、Request、具体产品 Client 或数据库连接。HTTP owner 保留认证、路由、JSON 上限及状态码映射。现有 product/client.Client 通过结构满足端口，app 继续组装实际应用。

这是明确职责与可独立验证的提取，不宣称同包内编译隔离。原路径随提取删除，不增加通用命令框架或一层逐方法转发 client。

## 边界与保持条件

- 使用现有 [HTTP 契约](../product-design/local-client-protocol.md) 和 httpcontract/product/api 值。公开 DTO、路由、action、状态码、能力、请求格式和持久记录格式保持；本轮不改 schema、权限或业务资格。
- 同一项目跨动作共用原登记仲裁及锁范围。保存完整原信封与实际映射意图后才派发；同 ID 同内容返回原记录，异内容冲突。不能因提取普通命令/原生查看而分裂 ID 命名空间或并发仲裁。
- 保留 control/SQLite authority 的按动作查找顺序、精确身份/版本核对、已有外部正式命令 ID 冲突、登记窗口竞争和持久 rejection。读取失败不是 command_not_found；unknown 或已有接受摘要且 authority 丢失不能重放。
- 原生查看调用前持久标记 unknown，重复请求不再次开窗；与普通业务命令的补交规则分别实现。停服在现有窗口等待原 applied 回执。文件聚合大小、结果摘要限额、原子发布、ACL、发现与凭证安全保持。
- 应用端口仍由 Go 业务 owner 核验资格/转换/提交；登记 owner 不决定 Task、Execution 或 TaskDelivery 状态，不读取业务文件回退。

## 修改归属与并行

本项拥有 httpapi 生产代码、所属测试和 `app/execution_service_wiring.go` 中 HTTP endpoint 的必要参数接线。#93 拥有 httpclient/consoleapp，二者消费现有协议，可并行。本项不修改 #94/#95/#96 的 execution 用例。

共享 httpcontract/product/api、httpfs 格式/ACL实现、测试公共 helper、runner 和存储提交接口冻结；确需改变时先重新核定相关任务范围，不将签名变化留给后合入者猜测。依赖守卫新增本项专用规则/测试，保留既有禁用规则。

无新增任务硬前置，复用已交付 #91/#45。一个任务、一个实现 PR；当前仅草稿，不因编号启动执行。

## 最小验收

| ID | 自动通过条件 |
| --- | --- |
| HC92-A1 | handler 经消费端口调用；登记/对账 owner 可用小型记录/authority stub 验证，不需要完整 app、真实 SQL 或 HTTP 对象；旧重复实现删除 |
| HC92-A2 | 重复/异内容/跨动作、先登记后派发、正式消费者竞争、错实例/目标、持久 rejection、authority 读取失败与 unknown 对账保持；可补交者仅用原 ID/原信封 |
| HC92-A3 | 原生查看至多一次调用、停服原回执、记录大小/原子性及认证保持；真实应用 HTTP 接线仍到原命令 owner |

无新增人工项；旧 #91 人工体验不转为本项前置。具体命令、环境与结果归专属验收，Plan 覆盖全部自动项。
