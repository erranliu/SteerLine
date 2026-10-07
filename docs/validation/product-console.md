# #231 三窗与内嵌反馈验收摘要

2026-10-07 第一批产品精简，绑定[Spec](../spec/product-simplification-01-console.spec.md)与[Plan](../spec/product-simplification-01-console.plan.md)。三窗、当前候选/Attempt 最新 10 条活动、原地反馈、完整业务最终回复与只保存草稿/原请求的 UI 偏好已实现；第二、三批不因此完成。

| 范围 | 原结果 |
| --- | --- |
| PS1-01–04、隐藏编辑器保护 | console/query/client 定向通过 |
| 最新 10 条、同事务、来源/准备轮隔离 | SQLite 定向通过 |
| 真实 app/SQLite/受控 worker 接线 | 两方案/两候选、切换草稿隔离、同一候选续跑通过 |
| PS1-05 Windows PTY | 80×24 终端的中文两行、F8、候选切换及 F2 原身份绑定通过；完整最终回复可滚到末尾 |
| L1 | 最终报告 passed/complete；此前超时/skip 不计通过 |
| 合并前质量修复 | console/query/client 109 项、SQLite 4 项，无 fail/skip；构建、vet 与文档静态检查通过 |

PTY 证明真实 VT/UTF-8 解析和程序渲染，不证明物理 IME、桌面截图或远端原生能力。后续布局修复使用定向回归，复用的 L1/受控接线/PTY 保持其原采样源码，没有追认为修复后全部重跑。早期 integration/候选准备、未刷新问题的续跑及 L1 超时均保留为原失败。

精确报告、修复与退役映射见[原记录](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/product-simplification-01-console/README.md)及[最终检查](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/product-simplification-01-console/final-checks.json)；当前行为维护在[产品设计](../product-design/README.md)。
