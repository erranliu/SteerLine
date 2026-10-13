# 客户端与 Go 服务边界

2026-10-13 已确认规划为[任务 #91](../spec/tui-http-client.spec.md)：保留 TUI、全部业务访问经 HTTP、删除 #44 WinUI。当前 consoleapp 注入纯 httpclient.Console，经 HTTP handler 调用服务端内部产品适配器；#44 WinUI 工程已删除。执行状态见[execution-map](../roadmap/execution-map.md)。

## 目标职责

TUI → 纯 HTTP 客户端/契约 → 服务端 HTTP handler → 内部产品适配器 → Go Task/Execution/原命令权威。SQLite、身份/版本、Operation 和唯一 TaskDelivery 保持。客户端不读业务库/checkpoint/inbox/私有日志，自己的草稿、偏好和原信封不是业务状态。

本机发现/凭证与必要 OS 生命周期观察独立于通用调用，退出客户端不停止服务/worker。离线显示最后观察/未发送输入并禁写，未有观察则明确不可查询；不回退直接读库或启动替代服务。恢复核验/原会话查看与停服证据已由 #91 补公开接口，原平台不支持的能力保持不可用。

协议现状见[本机客户端协议](local-client-protocol.md)，新范围/验收由 #91 Spec/Plan 固定。当前首版查询刷新，不引入 SSE/过程采集；连接区分服务/项目/实例/执行目标，路径属于服务或执行机器。远程认证/部署、WebUI 和主 Agent 后续重新规划。

旧三节点/Proposal/Publication/WinUI 页面不再是当前要求，见[退役说明](../archive/roadmap/README.md)。原[界面设计](planning-ui.md)及原型仅保留参考。源码接线不代替自动/人工验收，具体证据见任务专属记录。
