# 第六批文档精简与提示词审阅

2026-11-06 整理[项目准备提示词](../../../docs/product-design/project-onboarding-prompt.md)、[Activity](../../../docs/product-design/execution-observability.md#流程与状态活动)和[Roadmap](../../../docs/product-design/roadmap-promotion.md)。本记录保存本次差异与验证，不是运行能力或安装验收。

## 来源与保留范围

精简前已提交原文可用 `git show 5b8e08bc53e3789b2fe0ab736260a26eaf7be1c7:<路径>` 读取：

- docs/product-design/project-onboarding-prompt.md。
- docs/product-design/activity.md。
- docs/product-design/roadmap-promotion.md。

Activity 保留公开变化、原身份/游标、近期预算、显示顺序及只读预览规则；移除已退役 run/console 用法，Runner 采样细节按原版本追溯。[原 Plan](../../plans/execution-observability.plan.md)及[验收](../../testing/acceptance/execution-observability.md)保持原受测范围。

Roadmap 合并静态准备说明，补 roadmap_prepare_static，并区分解析与文档记录目标；原静态/动态材料、未知请求、发布及事务入图约束保留。证据仍为[转正验收](../../testing/acceptance/roadmap-promotion.md)、[初始化 Spec](../../spec/roadmap-static-initialization.spec.md)及[初始化验收](../../testing/acceptance/roadmap-static-initialization.md)。

第三批记录的提示词字节/SHA 是当时快照，本次以以下新正文为准；原始记录不改成当前版本。

## 提示词实际差异

业务原因：旧模板声称远端 Plan 更新自动生成测试安排，与当前项目维护显式登记、后台处理的实现冲突。仅替换这一处行为说明，不扩大准备 Agent 的修改、提交或执行授权，保留模板独立使用所需的其余规则。依据见[显式准备验收](../../testing/acceptance/project-maintenance-preparation.md)及[维护 worker](../../../internal/app/maintenance_worker.go)。

度量范围为 Windows 工作区四反引号 text 块内部的原始 UTF-8 字节（含原 CRLF），不含围栏或外围说明。Git 原文使用 LF，恢复其 69 个 CRLF 后可复核修改前 SHA；本次没有统一改换行。

| 项目 | 修改前 | 修改后 |
| --- | --- | --- |
| 复制正文 | 7542 bytes | 7613 bytes（+71） |
| SHA-256 | ef8eb1db4182556b82e4707fb1a8cc6639e74c591f409fbdd7485da6eaa13b12 | e18d16c36a050a1fac8d4dcb11af89e765b9dd2e7144ccbc8f8d65252bdba2fc |
| CRLF 换行 | 69 | 69 |

以下为完整变化行；逐字节核对确认复制正文除此替换无其他修改：

```diff
--- before/copy-prompt
+++ after/copy-prompt
@@ -42 +42 @@
-与后续统一维护首版衔接时，testing 角色固定为 SteerlineFlow/testing.md，不另建同内容测试规范。维护测试管理所需的 SteerlineFlow/testing/test-list.json；有实际 Plan 时在测试计划里说明最小必要入口，SteerLine在远端Plan更新后自动生成系统测试安排；不要求创建、应用或提交旁置.tests.json。没有测试时如实记录、清单可为空；没有实际任务时不创建示范 Plan/安排。服务采用配置远端目标分支的已发布 commit，本地领先提交或未合入准备 PR 仍须报告待发布。
+与后续统一维护首版衔接时，testing 角色固定为 SteerlineFlow/testing.md，不另建同内容测试规范。维护测试管理所需的 SteerlineFlow/testing/test-list.json；有实际 Plan 时在测试计划里说明最小必要入口，用户在项目维护显式准备后，SteerLine按需生成系统测试安排；查询、轮询和任务启动不隐式登记生成；不要求创建、应用或提交旁置.tests.json。没有测试时如实记录、清单可为空；没有实际任务时不创建示范 Plan/安排。服务采用配置远端目标分支的已发布 commit，本地领先提交或未合入准备 PR 仍须报告待发布。
```

## 验证与独立审核

- `go test ./internal/prompts -count=1 -v` 通过，全部黄金正文和规则副本检查一致；本检查验证原生产模板未受影响，不代替对复制正文的独立审核。
- `python scripts/check_doc_links.py`：706 份 Markdown、4,363 条本地链接，零错误；`python scripts/roadmap_task_id.py check` 和 `git -c core.safecrlf=false diff --check` 通过。
- 定向静态核对：28 条锚点引用无误，docs/ 全部 25 份 Markdown 被两级索引直接覆盖，三份固定 Git 原文可读取；原 Spec/Plan/验收/roadmap 仅有前批导航变更，原型字节未变。
- 修改前两级索引及三份正文的 48 个不同引用目标均保留在当前文档或本记录。
- 复制正文逐字节仅有上列替换，保留 69 个 CRLF；生产提示词、组装/接线、规则资产、writer、黄金样本与 CI 无修改。本次为文档模板纠错，按测试指南不启动产品 L1/L2、模型或真实 PR 验收。
- 独立审核：`/root/sixth_batch_doc_review` 只读审阅，未参与编写；2026-11-06 批准本批文档及复制提示词变更，无需修复的问题。审核核对了实际正文差异、7,542→7,613 字节与 SHA、显式准备实现、模板自足性及授权边界、Activity 保留规则、Roadmap 目标/原请求/入图约束、索引与历史入口；并独立通过链接与差异检查。

本次修改用户复制的 Markdown 模板，不改 internal/prompts、生产组装/接线、正式协议/writer、黄金样本或 CI；不执行模型、PR、安装或远端保护配置。产品添加/核验/纳管仍为拟议能力，文档整理不使其交付。
