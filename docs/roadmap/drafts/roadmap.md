# SteerLine 当前 Roadmap 草稿

本轮按[RoadmapGuide](../RoadmapGuide.md)选定本页，仅规划交付 `TUI → HTTP API → Go 后端`。2026-10-13 用户确认范围，实际状态/依赖/验收汇总以[execution-map](../execution-map.md)为准，规划确认不表示已启动实现。

本轮无待执行任务。#91 已经独立复审修复、最小自动复验及真实 [PR #299](https://github.com/erranliu/SteerLine/pull/299) 合并核验，2026-10-13 从草稿任务表移除。保留能力摘要及永久编号；[Spec](../../spec/tui-http-client.spec.md)、[Plan](../../spec/tui-http-client.plan.md)、[验收](../../acceptance/tui-http-client.md)可追溯，TH-M1 剩余人工判断继续在正式清单登记。

已交付的业务消费只经 HTTP，不访问业务库或宿主命令文件回退；退出客户端不停止服务/worker。查询刷新沿现有节奏，不引入 SSE 或完整过程采集。

共享 codec/服务/console 构造由 #91 统一修改；实际独立 SteerlineTest 包、复审修复、验证版本/失败和原身份见验收，人工窗口剩余项待安排。后续新增工程范围须重新确认，不从本页已交付摘要恢复派发。

旧 #43/#46–#52/#40/#41 不再是待重定队列，编号保留且不复用。#44 规格/结果归档、实现删除由 #91 交付；#45 为已有能力。历史见[归档说明](../../archive/roadmap/README.md)。WebUI、SSE、主 Agent、远程部署不拆任务或设前置，后续按真实需求重新规划。一期独立未验仍由 execution-map/acceptance 保留。
