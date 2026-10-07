# Plan：移除 Beads 旧执行路径

任务 ID：12；任务名称：移除 Beads 旧执行路径；目标：`REFACTOR`。状态：实现与验证完成；[#44](https://github.com/erranliu/SteerLine/pull/44) 已提交审查，待合入。唯一执行依据：[Spec](beads-path-removal.spec.md)。硬前置：[#11](test-feedback.spec.md) 已交付；一个实现 PR 交付。

## 实施顺序

1. 基于 #11 最终版本取得本任务基线，列出 Beads 主动调用、配置分支、旧整段 worker/结果路径、测试 fixture 与当前原生消费者。检查 `beads.go`、`config.go`、`service.go`、`completion.go`、`minimal_step.go`、`recovery.go`，按调用关系确定删除面。
2. 先把旧 fixture 中保护当前行为的断言移到原生最小现场，登记原测试到新测试的对应关系；PR 合并事实、未知副作用和独立恢复不能退化为只测返回值。
3. 统一所有入口对显式 native 的要求，删除 Beads 客户端和旧配置执行分支。把原生仍使用的上下文参数换成已有原生身份；删除不再可达的旧 start/finish/completion 执行及 fake 分支。
4. 复核旧状态检测仍在任何初始化/外部动作之前。用包含哨兵文件与外部动作计数的 fixture 验证拒绝不修改现场；保留当前格式恢复与独立资源占用。
5. 更新当前配置、文档和受影响规格，核对 #6 不再引用可执行的过渡 Beads 接口。报告放在 `docs/validation/beads-path-removal/`，向 #13 交接已收敛的原生代码和测试。

## 验证与完成条件

按 LR1–LR6 逐项验收。已运行受影响配置、原生工作流、决策/恢复、PR 和旧状态拒绝用例；最终完整 suite、静态检查、Windows/Linux 构建记录见 [验证结果](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/beads-path-removal/results.md)。Live Codex、Orca、PR-provider 和可见窗口开关未启用，保持为独立真实平台验收门槛。

对每个删除的测试说明删除的是失效产品路径还是重复断言；当前仍有意义的行为必须给出承接位置。比较耗时并解释变动，不通过跳过测试或减少恢复约束达成清理。

## 测试计划（2026-10-07 导航维护）

本次仅将已清理的证据导航改为固定 Git 快照，不改原实现契约或追认历史通过。最小范围为文档静态 `doc-links`、`task-identities`、`whitespace`：当前 Windows/Git/Python 3 环境，交付前执行 `python scripts/check_doc_links.py`、`python scripts/roadmap_task_id.py check`、`git diff --check`；通过条件为无缺失本地链接、任务身份一致及无空白错误。固定历史链接的目标另用 `git cat-file --batch-check` 核对存在；结果见[本次清理验证](../../../acceptance/history.md#测试计划与本次结果)。不选择 L1/L2/L3、Full 或原生模型组；导航变更不需要这些验证，不新增场景或环境能力。
