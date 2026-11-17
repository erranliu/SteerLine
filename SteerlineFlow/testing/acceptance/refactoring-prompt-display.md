# 重构与维护指令展示

2026-11-16，受测版本为 `d943e9673278` 加本次工作树差异。

当时的详情页默认展示冻结的维护方向、补充要求和最新一条执行指令。最近的决策指令、历次执行反馈、运行信息和历史记录可按需展开。新建页预览维护方向并说明执行指令何时生成。当前展示见[产品设计](../../../docs/product-design/refactoring-maintenance.md#行动详情)。

详情读模型从保存的决策请求读取阶段规则和适用策略原文，并使用现有工作流类型区分决策与执行记录。固定提示词正文、请求组装与运行时材料的 UTF-8 字节均未修改。

报告根目录为 `C:/Users/lmas1/.codex/visualizations/2026/11/16/01a1e337-faa3-7601-86cf-fe63559bcaa2`。

| 执行 | 结果与证据 |
| --- | --- |
| `npm --prefix web test -- src/features/refactoring-maintenance/refactoring-maintenance.test.tsx` | 11 项通过，覆盖维护方向与执行指令正文、初始折叠、等待生成、历史执行与新建预览。 |
| `npm --prefix web run build` | TypeScript 检查和生产构建通过。 |
| `go test ./internal/product/query ./internal/refactoring -count=1` | 两个包通过，覆盖最新决策材料选取和执行指令投影。 |
| `go run ./cmd/test-feedback -level=L1 -out <目录>` | 通过，报告位于 `l1-maintenance-retry/summary.json`。 |
| `go run ./cmd/test-feedback -level=L2 -out <目录>` | 受控交付冒烟通过，报告位于 `l2-maintenance-retry/summary.json`。 |
| `go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled -scenario runtime.refactoring-maintenance -out <目录>` | 通过，清理完整。Edge 验证桌面与 375 像素窄屏、历史记录与刷新，报告位于 `l3-maintenance-retry/summary.json`。 |

首次沙箱运行遇到 Windows 临时文件替换和测试 Git 仓库创建权限错误。上表记录在独立目录完成的复测。未部署，未调用真实模型。

提交 PR 时基于 `a7cc509b6c4d` 整合了后台启动核验。维护页 11 项测试、最新决策规则投影测试和 Web 生产构建复测通过。启动失败的原因和「带入新建」仍可直接查看，申请的起始提交移至「运行信息」。

## 2026-11-17 名称与默认展示

受测版本为 `db4615a3740f` 加本次工作树差异。执行对话顶部直接展示维护方向和补充要求，时间线直接展示执行指令和执行反馈。每条决策指令默认折叠，展开后可看短指令、阶段规则和适用策略。固定提示词资产、文档内引用的规则正文及执行接线与基线比对无差异。

- `npm --prefix web test -- src/features/refactoring-maintenance/refactoring-maintenance.test.tsx` 的 13 项测试和 `npm --prefix web run build` 通过。定向测试覆盖默认显示、单条决策指令展开、原文换行及启动失败时的补充要求。
- `go run ./cmd/test-feedback -level=L1` 通过。独立进程复测 L2 `smoke.delivery` 通过，报告位于 `C:/Users/lmas1/.codex/visualizations/2026/11/17/01a1e81b-1eed-78a1-81f6-79ea668f46f5/refactoring-terms-l2-2ea19ceb8ee940ce8f31f68a25b89f16/summary.json`。
- 独立进程复测 L3 `runtime.refactoring-maintenance` 通过。Edge 验证新建表单、详情默认展示、决策指令按条展开及刷新后恢复折叠，并检查 375 像素视口无横向溢出。报告位于 `C:/Users/lmas1/.codex/visualizations/2026/11/17/01a1e81b-1eed-78a1-81f6-79ea668f46f5/refactoring-terms-l3-28325d9fac7a4666939d2f503f6b82aa/L3/summary.json`。
