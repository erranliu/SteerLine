# Plan：Codex 规划主 agent 能力验证

任务 ID：43；任务名称：Codex 规划主 agent 能力验证；目标：`PHASE2`。唯一执行依据：[Spec](phase2-agent-capabilities.spec.md)。一个独立实现 PR 交付。

硬前置：无。

## 实施顺序

1. 建立隔离探测目录、专用配置和只读哨兵，记录 CLI/协议 schema；先枚举需要禁止的工具和权限覆盖来源。
2. 实现一个最小 stdio 客户端及受控 Markdown 写样例，依次执行新建、恢复、流式和审批路径；不复用用户生产 thread。
3. 运行权限负向矩阵及断线/响应丢失试验；对每个动作保存原请求、thread/turn、进程与文件前后证据。
4. 形成通过/失败能力报告和复现说明，只有 AC1–AC5 全通过才交付下游。

## 验证与交接

逐项填写本任务 Spec 验收表，在 docs/validation/phase2-agent-capabilities/ 保存构建、输入、身份、工具版本、实际结果和复现命令。先做定向检查，再运行受影响组件完整回归；Go 改动运行 go vet，UI 改动完成原生构建。模拟、跳过和未测项分别列出；没有真实平台证据不能宣称相应能力通过。完成 git diff --check，向后继交接固定接口与能力证据，不启动后继任务。
