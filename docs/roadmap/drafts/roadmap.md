# SteerLine 当前 Roadmap 草稿

本轮按[RoadmapGuide](../RoadmapGuide.md)选定本页，仅规划交付 `TUI → HTTP API → Go 后端`。2026-10-13 用户确认范围，实际状态/依赖/验收汇总以[execution-map](../execution-map.md)为准，规划确认不表示已启动实现。

| 任务 ID | 任务名称 | 成果 / 所有者 | 前置 | 执行规格 | 专属验收 |
| --- | --- | --- | --- | --- | --- |
| 91 | TUI 经 HTTP 接入 Go 后端 | 公共 HTTP 契约/客户端、Go 接线、TUI 消费由本任务统一修改；补必要接口、保留 TUI、删除 #44 WinUI | 无未完成硬前置；复用已交付 #45 与当前 TUI | [Spec](../../spec/tui-http-client.spec.md) · [Plan](../../spec/tui-http-client.plan.md) | [验收](../../acceptance/tui-http-client.md)：TH-A1–A6 自动，TH-M1 人工可后置 |

先固定接口缺口、原请求和停服/离线行为，再换客户端。业务只经 HTTP，不访问业务库或宿主命令文件回退；退出客户端不停止服务/worker。查询刷新沿现有节奏，不引入 SSE 或完整过程采集。

一个任务、一份 Spec/Plan、一个实现 PR，共享 codec/服务/console 构造由该任务拥有。最小验证与新增驱动见 Plan，人工窗口待安排；先生成独立 SteerlineTest 包并显式 simulate，自动失败不能后置。

旧 #43/#46–#52/#40/#41 不再是待重定队列，编号保留且不复用。#44 规格/结果归档、实现删除由 #91 交付；#45 为已有能力。历史见[归档说明](../../archive/roadmap/README.md)。WebUI、SSE、主 Agent、远程部署不拆任务或设前置，后续按真实需求重新规划。一期独立未验仍由 execution-map/acceptance 保留。
