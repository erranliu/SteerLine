# Plan：本机客户端协议与历史查询

任务 ID：45；任务名称：本机客户端协议与历史查询；目标：`PHASE2`。唯一执行依据：[Spec](phase2-local-api.spec.md)。一个独立实现 PR 交付。

硬前置：#36，直接复用其状态、原会话反馈和 PR 查询接口；不等待多候选或全面 SQL 搬包。

## 实施顺序

1. 核对现有 Query/Command、ConversationQueryService 和图查询，列出执行 action/字段映射，提交公共 DTO schema 与错误样例。
2. 接入服务生命周期和回环 HTTP，实现发现/ACL/认证/实例校验，再接现有查询与命令。
3. 把 SSE 接到持久变化查询；验证游标、reset、断线原命令回读，以及 command_not_found 后原信封补交和迟到提交仲裁。
4. 提供同查询实现的离线 JSON 入口，测试只读打开、schema 拒绝和业务数据不变。
5. 用真实服务窗口完成 API1–API6；运行受影响 Go 回归和 Windows 认证/离线验收。

## 验证与交接

逐项填写本任务 Spec 验收表，在 docs/validation/phase2-local-api/ 保存构建、输入、身份、工具版本、实际结果和复现命令。先做定向检查，再运行受影响组件完整回归；Go 改动运行 go vet，UI 改动完成原生构建。模拟、跳过和未测项分别列出；没有真实平台证据不能宣称相应能力通过。完成 git diff --check，向后继交接固定接口与能力证据，不启动后继任务。
