# HTTP / Go 服务边界（历史阶段约束）

> 2026-10-15：TUI 专属客户端/交互已退役，本文保留原范围与历史事实；共享 HTTP、服务和进程身份要求继续有效。当前范围见[TUI 退役](../spec/tui-retirement.spec.md)。


2026-10-13 阶段范围和任务 #91 的 TUI/HTTP 接入已交付，之后 TUI 已退役。本文保留原设计边界；当前交互入口见[Web Spec](web-frontend.spec.md)和[TUI 退役验收](../acceptance/tui-retirement.md)，原范围见[任务验收](../acceptance/tui-http-client.md)。

Go 服务核验 Task/WorkflowRun/Execution/Attempt/Decision/Command/Operation/唯一 TaskDelivery，SQLite与原命令权威保持。当时的 TUI 通过 HTTP 读取/操作，纯产品值/HTTP 契约/客户端不依赖具体业务实现；当前 Web 继续使用该服务边界。偏好、草稿、原信封与连接发现不是业务权威。

保持本机发现/认证/实例核验，不放开浏览器 Origin/CORS/远程监听。命令绑定原 request_id、精确目标/版本，提交前保存信封，丢回执先查，权威 command_not_found 才补原请求；未知副作用只对账。关闭 Web 不停服务/worker，离线保留观察/输入并禁写，不回退业务库/inbox/私有日志。

补恢复核验、原会话查看与停服证据；final/有界正文/缺口保持。首版查询刷新，不以 SSE、过程采集、未来规划域为前置。地址/项目/实例/执行目标区分，为远程留边界但不交付远程部署；无旧协议兼容或离线查询进程。

#44 实现删除由 #91 完成；历史/未验不标通过；#45 基线/限制按[原记录](../acceptance/phase2-local-api.md)核对。旧编号保留不派发，后续范围以当前 Spec 和验收为准。

任务独立 Spec/Plan/acceptance，自动验收映射最小入口/环境/命令/证据。人工少量可后置，失败/关键前置不豁免。本次只作规划静态检查，产品验证由实施后执行。
