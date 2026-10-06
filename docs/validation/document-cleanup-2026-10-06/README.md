# 文档归档与当前入口检查

日期：2026-10-06（Asia/Shanghai）。核对源码基线：`75474dce`，三线最终清理 PR227 已合入。此次只整理文档、导航及源码定位，不修改产品实现、任务身份、固定契约或原始 JSON/日志证据。

## 整理范围

- 任务表移除已交付的 #76–#82，三线目标 roadmap 归档；#73–#82 的完整历史规划保留在[归档](../../archive/roadmap/three-line-modularization.md)。
- 125 份已交付规格、配套计划和阶段约束移入 `docs/archive/spec/completed/`。其中有 60 份任务 `.spec.md` 和 57 份配套 Plan；三份历史架构审计 Spec 原本没有独立 Plan，不补造历史实施计划。
- 旧中心步骤流程、远端探针和旧操作指南单独归档。归档不表示未测远端、故障场景或平台限制已通过。
- 活动表一致保留 #40–#53，共 14 项；三个尚未闭合的定向验收入口保留在[当前 Spec 索引](../../spec/README.md)。全部 83 个任务身份记录保持原样。
- 移动仅适配 Markdown 导航。历史 Canonical Spec 路径、commit、摘要、命令及原始回执保留原记录；固定 Git 契约仍从其原 commit 读取原路径。

## 最小验证

| 范围 / ID | 环境和命令 | 通过条件 | 结果 |
| --- | --- | --- | --- |
| 文档静态 / doc-links | Windows、Python 3；`python docs/validation/document-cleanup-2026-10-06/check_docs.py` | 本地 Markdown 导航目标存在；忽略代码示例里的伪链接；活动表一致，已交付 Spec/Plan 成对归档 | pass，最终计数见下方 |
| 文档静态 / task-identities | 当前 Git 仓库；`python scripts/roadmap_task_id.py check` | 身份、名称、Spec/Plan 和两份规划表一致 | pass：83 identity files，next ID 83 |
| 文档静态 / whitespace | 当前 Git 仓库；`git diff --check` | 无新增空白错误 | pass |
| 文档静态 / evidence-integrity | 通过 `git cat-file --filters HEAD:<原路径>` 还原 Git checkout 字节，与移动后的非 Markdown 文件和 `task-ids/` 比较 | 原始探针、JSON、图片、日志和身份不因移动改变 | pass，最终计数见下方 |

验证在移动、入口修正和失效导航修复后执行。未选择 L1/L2/L3、Full 或真实模型场景：实现、协议、测试登记和执行接线没有改变，本次检查只需要文档静态校验。无需新增产品测试场景或测试环境。

导航检查验证文件和目录是否存在；历史章节锚点、已固定 commit 中的源码布局、外部网页与原试验可重放性不在此次校验范围。

最终检查：360 份 Markdown、2323 个本地导航链接，0 个缺失目标；三份活动表一致。400 份非 Markdown 证据及任务身份与原路径的 Git checkout 字节逐字节一致，0 差异。Windows checkout 的 CRLF 与 Git blob 的 LF 差异通过原路径过滤器处理；不改写现有文件。
