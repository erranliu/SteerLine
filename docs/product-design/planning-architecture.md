# 客户端与 Go 服务边界

当前 Web 通过独立 Web 服务和 Go 应用目录访问原项目 HTTP API。项目服务保留 Task/Execution 与原命令权威，应用目录不直接查询数据库。原 #91 TUI/consoleapp 交付已随[TUI 退役](../spec/tui-retirement.spec.md)删除；历史事实见原验收。

## 目标职责

Web → 应用目录/HTTP 客户端 → 项目 HTTP handler → 内部产品适配器 → Go Task/Execution/原命令权威。SQLite、身份/版本、Operation 和唯一 TaskDelivery 保持。客户端不读业务库/checkpoint/inbox/私有日志，自己的草稿、偏好和原信封不是业务状态。

本机发现/凭证与必要 OS 生命周期观察独立于通用调用，退出客户端不停止服务/worker。离线显示最后观察/未发送输入并禁写，未有观察则明确不可查询；不回退直接读库或启动替代服务。恢复核验/原会话查看与停服证据已由 #91 补公开接口，原平台不支持的能力保持不可用。

协议现状见[本机客户端协议](local-client-protocol.md)，Web 范围/验收由独立 Web Spec/Plan 固定。当前首版查询刷新，不引入 SSE/过程采集；连接区分服务/项目/实例/执行目标，路径属于服务或执行机器。远程认证/部署和主 Agent 后续重新规划。

旧三节点/Proposal/Publication/WinUI 页面不再是当前要求，见[退役说明](../archive/roadmap/README.md)。早期[规划界面设计](../archive/design/planning-ui.md)及原型仅保留历史参考。源码接线不代替自动/人工验收，具体证据见任务专属记录。
