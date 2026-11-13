# 重构与维护的 SteerAgent 决策与 PR 合入评审 Plan

对应 [Spec](../spec/refactoring-agent-delivery.spec.md)。原交付同步方案的联验记录保留在[验收文档](../testing/acceptance/refactoring-agent-delivery.md)中。

1. 保留单一行动、预算、人工决策及原工作区持有者链。用现有 execute/1 的原始三级指令输入和已验证 Final 回执，删除维护结果文件初始化、封存和依赖检查。未核实的发布只处理原请求和可确认的原 PR。
2. 用现有只读 PR 查询绑定仓库、原分支和完整 head/base。在项目 Git 对象库获取固定提交差异，交付只读评审材料，按版本保存建议和原文。结束前再次核对版本，已交付 PR 的版本变化沿原行动重评。
3. 移除正文同步状态、同步指令和额外可写 Run。页面显示合入建议、版本及过期状态，保留原 PR 冲突续作与显式重评。
4. 依[测试指南](../testing.md)执行验收并记录原始证据。选定 ID 随[清单](refactoring-agent-delivery.tests.json)冻结。L1/L2 检查基础回归。go.http、go.storage 覆盖原有查询与行动持久化。go.runner-result 使用公开原生协议夹具验证文件材料、工作区复用及常规结果能力，维护使用无结果文件分支。L3 验证人工恢复、独立评审、原 PR 正文保持和冲突续作。真实 L4 使用已合入的固定 Runner 和隔离 GitHub 仓库，核实模型最终回执、两版独立评审与资源清理。
