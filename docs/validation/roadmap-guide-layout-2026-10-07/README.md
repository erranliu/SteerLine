# RoadmapGuide 与草稿目录

日期：2026-10-07（Asia/Shanghai）。本次只整理规划入口、草稿位置与静态检查，不创建/取消中心任务、worker、session 或 heartbeat。

## 结构与维护

- docs/roadmap/RoadmapGuide.md 是每次规划必读入口，索引项目规则、产品愿景、整体/并行架构、正式清单、Spec 索引及测试方法；不索引单份草稿。原 readme.md 的有效流程和规则归入指南，readme 仅提供发现入口。
- 现有三个规划文件进入 drafts/，可继续按主题扩展；具体草稿按人的本轮目标选择，目录枚举不是待执行队列。草稿的 Spec/Plan 链接按新位置修正。
- 身份检查独立扫描 drafts/ 内 Markdown 文件，支持不同名称、子主题、同一预留身份的备选草稿，并从各文件自身目录解析契约链接。正式成员不依赖某一特定草稿，也不借草稿补契约。
- 项目 AGENTS、文档入口、并行指南及执行技能指向 RoadmapGuide。日常执行仍只消费已核对 DAG 的 execution-map，正式条目保持 0，同步状态未核对；没有操作实际中心 DAG。
- 历史 Markdown 导航按移动位置修复，历史命令、Canonical 路径、commit/摘要、身份 JSON 和原始数据不改写。现有 Spec/Plan 执行要求不变。

## 最小验证

| 范围 / 场景 | 命令与环境 | 时点 / 通过条件 | 证据 |
| --- | --- | --- | --- |
| 草稿/正式边界 | Python 3：python -m unittest discover -s scripts -p test_roadmap_task_id.py -v | 修改后，10 项通过；多份任意名称、嵌套相对路径、备选草稿及空正式表独立验证 | checks.log |
| 身份与导航 | python scripts/roadmap_task_id.py check；python scripts/check_doc_links.py | 迁移与引用修正后，原身份有效，13 草稿/0 正式，导航零缺失 | checks.log |
| 指南范围与保留数据 | 核对指南链接、变更文件及正式条目 | 提交前，指南不直接索引 drafts/ 内文件，旧根级草稿移走；原 Spec/Plan 与身份 JSON 未改 | checks.log |
| 技能入口 | python -X utf8 <skill-creator>/scripts/quick_validate.py .agents/skills/roadmap-runner | 入口修正后，格式有效；不调用执行/自动化工具 | checks.log |
| 差异格式 | git diff --check | 提交前，无新增空白错误 | checks.log |

不运行产品 L1/L2/L3、Full、模型或原生场景：产品代码、协议、执行环境和数据库格式未变。只运行改动的离线检查工具和技能验证，不重复未改的离线状态助手测试。目录调整不视为服务自动同步、真实 DAG 准入或业务完成证明。
