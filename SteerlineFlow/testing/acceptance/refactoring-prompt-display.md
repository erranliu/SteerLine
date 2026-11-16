# 重构与维护两级提示词展示

2026-11-16，受测版本为 `d943e9673278` 加本次工作树差异。

详情页默认展示冻结的方向与补充要求，以及最新一条 Runner 执行指令。最近决策请求的内置规则、历次执行反馈、运行信息和历史记录可按需展开。新建页采用相同的两级标题，预览方向并说明执行提示词何时生成。

详情读模型从保存的决策请求读取阶段规则和适用策略原文，并使用现有工作流类型区分决策与执行记录。固定提示词正文、请求组装与运行时材料的 UTF-8 字节均未修改。

报告根目录为 `C:/Users/lmas1/.codex/visualizations/2026/11/16/01a1e337-faa3-7601-86cf-fe63559bcaa2`。

| 执行 | 结果与证据 |
| --- | --- |
| `npm --prefix web test -- src/features/refactoring-maintenance/refactoring-maintenance.test.tsx` | 11 项通过，覆盖两级正文、初始折叠、等待生成、历史执行与新建预览。 |
| `npm --prefix web run build` | TypeScript 检查和生产构建通过。 |
| `go test ./internal/product/query ./internal/refactoring -count=1` | 两个包通过，覆盖最新决策材料选取和执行指令投影。 |
| `go run ./cmd/test-feedback -level=L1 -out <目录>` | 通过，报告位于 `l1-maintenance-retry/summary.json`。 |
| `go run ./cmd/test-feedback -level=L2 -out <目录>` | 受控交付冒烟通过，报告位于 `l2-maintenance-retry/summary.json`。 |
| `go run ./cmd/test-feedback -level=L3 -suite=runtime -env=controlled -scenario runtime.refactoring-maintenance -out <目录>` | 通过，清理完整。Edge 验证桌面与 375 像素窄屏、历史记录与刷新，报告位于 `l3-maintenance-retry/summary.json`。 |

首次沙箱运行遇到 Windows 临时文件替换和测试 Git 仓库创建权限错误。上表记录在独立目录完成的复测。未部署，未调用真实模型。

提交 PR 时基于 `a7cc509b6c4d` 整合了后台启动核验。维护页 11 项测试、最新决策规则投影测试和 Web 生产构建复测通过。启动失败的原因和「带入新建」仍可直接查看，申请的起始提交移至「运行信息」。
