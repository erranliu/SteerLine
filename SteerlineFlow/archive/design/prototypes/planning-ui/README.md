# 规划界面原型（历史演示）

打开同目录 [index.html](index.html) 即可浏览，无构建步骤、外部脚本或服务依赖。HTML 按整理前字节原样迁入，演示任务、文档、命令、回执和路径均是内嵌快照；不读取实际仓库，不调用 Runner、Codex、Git、PR 或产品 API。

## 演示范围

| 页面 | 可讨论的交互 |
| --- | --- |
| Roadmap 草稿 | Markdown 解析、内部代号/依赖确认、Spec/Plan 生成与审阅、文档合并及入图编号 |
| DAG 与执行 | 派发开关、独立候选、反馈、模拟断线及原会话定位 |
| 长期文档与版本蓝图 | 目录导航、只读正文、更新与待合入预览 |
| 验收与调试环境 | 人工事项、独立版本环境、指南及结果历史 |
| 测试、Bug 与体验版本 | 测试记录筛选及复验、模拟修复与 PR、版本创建/删除/重建 |
| 执行设置 | 机器、Agent、模型与推理强度的联动选择 |

页面中的能力开关、成功状态和自动动作只控制模拟。旧命令、门槛及流程文案不作为当前操作要求；断线时保留演示状态，刷新恢复初始数据。Roadmap 和测试管理的部分生产能力后来已交付，原型的“待接入/未实现”提示也只是旧版本文案。

## 当前依据与历史来源

- 当前界面及工程使用：[Web README](../../../../../web/README.md)。
- Roadmap 真实业务：[产品设计](../../../../../docs/product-design/roadmap-promotion.md) · [Spec](../../../../spec/roadmap-promotion.spec.md) · [Plan](../../../../plans/roadmap-promotion.plan.md) · [验收](../../../../testing/acceptance/roadmap-promotion.md)。
- 只读 Git 测试历史：[测试记录 Spec](../../../../spec/test-records.spec.md) · [Plan](../../../../plans/test-records.plan.md) · [验收](../../../../testing/acceptance/git-test-records.md)。
- 早期页面设计与自动/人工事项：[历史方案](../../planning-ui.md)。
- 测试分层及证据规则：[通用测试架构规范](../../../../../SteerlineProjectGuideDoc/testing-architecture.md)。
- 整理前的完整演示路线、各轮记录及原文读取方法：[归档来源](../../README.md#第三批整理来源)。

本原型保留作历史视觉参考。实际产品改动在当前 Web 工程和对应 Spec/Plan 中维护；需要新交互探索时明确新的范围与版本，避免把此快照继续更新成另一份产品规则。
