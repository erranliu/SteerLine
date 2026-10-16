# 历史阶段推进边界：TUI 经 HTTP 接入 Go 后端

> 本文记录 2026-10-13 的阶段决策，已被后续 Web 前端交付和 TUI 退役取代。当前状态以[Spec 索引](../../spec/README.md)、[执行清单](../../roadmap/execution-map.md)及[TUI 退役验收](../../acceptance/tui-retirement.md)为准。

2026-10-13 用户确认的唯一成果已由 #91 经 [PR #299](https://github.com/erranliu/SteerLine/pull/299) 实现、独立复审修复、自动复验及合并核验交付：保留 TUI，业务查询/命令统一经 HTTP 到 Go 后端。[主草稿](../../roadmap/drafts/roadmap.md)已移除完成任务行，暂无待执行任务；[Spec](../../spec/tui-http-client.spec.md)、[Plan](../../spec/tui-http-client.plan.md)与[验收](../../acceptance/tui-http-client.md)保留范围/证据，状态归[execution-map](../../roadmap/execution-map.md)。

Go Task/Execution/原命令与 SQLite 保持权威，HTTP 适配产品契约。TUI 保留键位/布局，补缺口后切换消费；服务端内部产品适配器保留。删除 #44 工程/专属配置由 #91 实施，原历史与未验不改称通过。

先跑通本机，查询刷新沿原节奏。连接地址、项目/实例和执行目标区分，路径属于服务/执行机器；实际远程监听/认证、多用户、WebUI、SSE、规划 Agent 和后续业务不纳入本轮。旧三节点不是当前门槛，仅在[历史归档](../../archive/roadmap/README.md)追溯。

无未完成硬前置，复用 #45；一期独立人工签收和原平台未验保留，不作为无关客户端改造前置。自动项全部映射最小测试，失败不能后置；TH-M1 可自动通过后集中到任务签收前，不建阶段验收 Task 或重复平台。
