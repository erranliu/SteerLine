# 重构方向模板与界面统一

重构与维护复用项目维护的面板样式和共享控件。内置与安装级自定义方向共用目录、完整原文预览和启动版本校验。每次行动冻结所选方向，单次目标和范围通过“补充要求”传入。[产品设计](../../../docs/product-design/refactoring-maintenance.md#维护方向) · [安装说明](../../../docs/integration/install-deploy.md#自定义重构方向)

## 提示词差异

公共规则原段内联了五个方向的标识与指导语。五句指导语移到同名独立资产，各加显示标题与句号。公共规则保留原段的切片与提前结束要求，在段首改用以下正文：

> 只读决策以本次上下文的 direction_prompt 完整正文为方向依据，requirements 提供本次补充要求。方向和补充要求均受本文件的权限、范围、验证与交付规则约束。

| 正式资产 | UTF-8/LF 字节（修改前 → 修改后） | 迁移的方向正文 |
| --- | --- | --- |
| [当时的公共规则](https://github.com/erranliu/SteerLine/blob/371ef2d6939eec90557c17506d38f39c94de324c/internal/prompts/materials/maintenance.md) | 4955 → 4905 | 读取实际选择的方向及单次补充要求 |
| [简化设计](../../../internal/prompts/materials/refactoring-directions/simplify.md) | 0 → 56 | 简化不必要的抽象与控制流。 |
| [清理冗余](../../../internal/prompts/materials/refactoring-directions/cleanup.md) | 0 → 59 | 清理已证实无用或重复的代码。 |
| [理顺职责](../../../internal/prompts/materials/refactoring-directions/boundaries.md) | 0 → 47 | 理顺模块职责与依赖。 |
| [提升可维护性](../../../internal/prompts/materials/refactoring-directions/readability.md) | 0 → 68 | 改善命名、错误处理和复杂表达。 |
| [整理文档与测试](../../../internal/prompts/materials/refactoring-directions/docs-tests.md) | 0 → 68 | 修复文档、测试与实现不一致。 |

实际只读短提示为 251 → 251 字节，可写短提示为 547 → 547 字节。预览与只读决策读取同一正文，以启动时校验的版本冻结到行动记录。只读 Runner 的 `maintenance/context.json` 携带完整 `direction_prompt`，可写初始化文件使用决策生成的实施指令。独立审核批准最终正文、字节守卫与文件接线，并复核安装目录边界、Windows 文件名匹配和可写初始化限额相关修复。

## 验证

受测基线为 `99aa0cc9` 加本次工作树差异。报告根为 `C:\SteerlineDebug\wt-5151-refactoring-directions\`。

- L1 和 L2 正式验证通过，报告位于 `test-results/L1-34424464` 与 `test-results/L2-e199799e`。后续变更影响的提示词、维护文件通道、目录行为和测试清单通过定向复验，报告位于 `diagnostics/owners-92c0f637`。
- 维护页面与项目切换的 Vitest 共 25 项通过，Web 生产构建通过。Edge 代表视口为 1440 和 375 CSS 像素，已检查完整原文预览及换行。截图位于 `test-results/L3-7567e839/scenarios/runtime.refactoring-maintenance/browser/`。
- L3 `runtime.refactoring-maintenance` 通过，报告位于 `test-results/L3-7567e839`。公开命令拒绝过期预览。浏览器启动自定义方向后，测试改写安装模板，后续只读决策材料与历史详情仍保留原正文。两轮完成后，冲突续作更新原 PR 与报告，刷新未产生新命令，受控资源清理通过。
- L4 `go.http` 通过，报告位于 `test-results/go-http-5e0d1370`。文档链接检查与 `git diff --check` 通过。

默认沙箱内的后端定向尝试遇到临时 Git 写权限与本机测试 HTTP 服务限制；同组测试在上述独立调试目录复验通过。

本次使用公开 Runner 和 PR 平台夹具验证交接。未运行真实模型或实际发布 PR，未部署安装版。
