# 当前阶段验收入口

> 2026-10-15：TUI 专属客户端/交互已退役，本文保留原范围与历史事实；共享 HTTP、服务和进程身份要求继续有效。当前范围见[TUI 退役](../spec/tui-retirement.spec.md)。


2026-10-13 用户确认 roadmap 仅到 TUI 经 HTTP 接入 Go 后端。唯一工程任务 #91，所有自动/人工行为与结果归[专属验收](tui-http-client.md)，不另立阶段 Task 或重复完整链验收。

TH-A1–A6 自动通过；独立复审发现均已修复，最后修复复核无新增问题，[PR #299](https://github.com/erranliu/SteerLine/pull/299) 已合并，#91 实现完成。用户委托的独立 SteerlineTest live/ConPTY 终端操作已验；TH-M1 剩余原生查看窗口渲染及桌面输入法/字体体验判断待人工验收，可后置到任务签收前。固定版本、合并事实、失败修复和证据见[专属验收](tui-http-client.md)，范围/入口见[Spec](../spec/tui-http-client.spec.md)/[Plan](../spec/tui-http-client.plan.md)，汇总归[execution-map](../roadmap/execution-map.md)。

旧三节点退出当前要求，见[归档](archive/superseded)和[退役说明](../archive/roadmap/README.md)。#44 实现已由 #91 删除，旧人工要求退出签收不等于通过；#45 仍见[原记录](phase2-local-api.md)。
