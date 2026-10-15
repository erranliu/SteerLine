# 当前阶段共同约束：TUI / HTTP / Go 服务

> 2026-10-15：TUI 专属客户端/交互已退役，本文保留原范围与历史事实；共享 HTTP、服务和进程身份要求继续有效。当前范围见[TUI 退役](../spec/tui-retirement.spec.md)。


2026-10-13 用户确认当前规划仅到[任务 #91](tui-http-client.spec.md)交付。旧三节点及 WinUI 页面退出活动范围；配套[Plan](tui-http-client.plan.md)/[验收](../acceptance/tui-http-client.md)，实际范围归[execution-map](../roadmap/execution-map.md)。本页不另立任务。

Go 服务核验 Task/WorkflowRun/Execution/Attempt/Decision/Command/Operation/唯一 TaskDelivery，SQLite与原命令权威保持。TUI 全部业务读取/操作经 HTTP，纯产品值/HTTP 契约/客户端不依赖具体业务实现；服务端仍用内部产品适配器。偏好、草稿、原信封与连接发现不是业务权威。

保持本机发现/认证/实例核验，不放开浏览器 Origin/CORS/远程监听。命令绑定原 request_id、精确目标/版本，提交前保存信封，丢回执先查，权威 command_not_found 才补原请求；未知副作用只对账。关闭 TUI 不停服务/worker，离线保留观察/输入并禁写，不回退业务库/inbox/私有日志。

补恢复核验、原会话查看与停服证据；final/有界正文/缺口保持。首版查询刷新，不以 SSE、过程采集、未来规划域为前置。地址/项目/实例/执行目标区分，为远程留边界但不交付远程部署；无旧协议兼容或离线查询进程。

#44 删除仍待 #91 实施，历史/未验不标通过；#45 基线/限制按[原记录](../acceptance/phase2-local-api.md)核对。旧编号保留不派发，后续重新规划。

任务独立 Spec/Plan/acceptance，自动验收映射最小入口/环境/命令/证据。人工少量可后置，失败/关键前置不豁免。本次只作规划静态检查，产品验证由实施后执行。
