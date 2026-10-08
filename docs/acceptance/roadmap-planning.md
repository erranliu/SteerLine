# 当前规划文档检查

本页记录当前文档维护的静态检查；历史PR和前序规划检查见[归档](archive/completed/roadmap-planning.md)。产品/任务验收仍在各自专属记录，静态检查不证明实际依赖交付或产品验收通过。

## 2026-10-07 acceptance归档

范围：历史验收与旧二期方案分层归档，保留当前待验/规划入口，修复导航；不修改产品、Task/DAG或原验收结果。

| 检查项 | 方法 / 命令 | 通过条件 | 状态 |
| --- | --- | --- | --- |
| 文档导航 | 自动；python scripts/check_doc_links.py | 本地链接无缺失 | 已检查，通过 |
| 身份/清单 | 自动；python scripts/roadmap_task_id.py check | 任务身份及3项正式规划条目保持 | 已检查，通过 |
| 差异格式 | 自动；git diff --check | 无格式错误 | 已检查，通过 |
| 证据完整性 | 自动；移动前后SHA-256比较 | 20份JSON/JSONL字节一致，历史失败/限制不重标 | 已检查，20/20一致 |

移动36个文件：已结算记录及精选证据30个、旧方案6个。原始迁移清单/摘要定位：C:/Users/lmas1/AppData/Local/Temp/steerline-acceptance-archive-7f9_rxhh/migration.json。原始测试数据仍留各次临时目录或固定Git历史，不重新运行。

此次仅文档归档，不选择产品L1/L2/L3/Full、模型、原生UI或DAG运行验证。当前待验、#83/#45/#44及等待原型的#49/#50计划保持待验；旧二期方案进入superseded，不算通过。

检查结果：254份Markdown、1569条本地导航、0错误；84身份、10草稿、3正式规划行，身份一致。3行仍为待中心准入规划，未验证live DAG。归档后acceptance根目录保留17个当前文件。

## PR主分支同步

同步origin/main 5d69bfbc，保留主线全部自动验收项的Plan映射规则及原产品/测试实现。新增#238后完整回归、第三批资源/CLI证据归入completed，#40/#41验收草案归入延期参考；原状态与JSON/JSONL不重标。初次归档统计/检查是同步前结果，最终范围交付前再核对。证据完整性记录：C:/Users/lmas1/AppData/Local/Temp/steerline-roadmap-pr-sync-zoqmwfit/evidence-integrity.json。

主分支同步后最终静态检查：259份Markdown、1642条本地链接、0错误；84身份、10草稿、3正式规划行；git diff origin/main --check通过。40份JSON/JSONL的工作树字节摘要保持，PR差异仅docs/。这些结果不计为产品验收或中心准入。

## 2026-10-08 Codex 开发流程简化

范围：项目规则、RoadmapGuide、roadmap-runner 技能/提示及相关导航统一为 Codex 直接开发。用户确认的 execution-map 管理任务与依赖，启动固定文档版本；保留独立复审、必要验证、真实合并、验收同步及防重复恢复。#83/#45待执行，#44等待#45，#49/#50留草稿。SteerLine 产品中心契约与业务实现不变，历史失败和所有未验项保留。

| 检查项 | 命令 | 实际结果 |
| --- | --- | --- |
| 文档导航 | python scripts/check_doc_links.py | 259份Markdown，1647条本地导航，0错误 |
| 身份/清单 | python scripts/roadmap_task_id.py check | 84身份、10草稿、3正式行，next ID 84；通过 |
| 技能元数据 | python -X utf8 C:/Users/lmas1/.codex/skills/.system/skill-creator/scripts/quick_validate.py .agents/skills/roadmap-runner | Skill is valid；默认GBK读取失败后以UTF-8入口复核通过 |
| 离线状态助手 | python .agents/skills/roadmap-runner/scripts/test_state.py -v | 18/18通过；串行、审核失效、人工后置、防重复与完成停用约束保持 |
| 差异格式 | git diff --check | 通过 |

检查输出见本聊天工具记录；只验证文档/技能和离线日志助手，不计为#83/#45/#44产品验收。本轮未创建执行聊天、worktree、heartbeat或PR，未运行产品L1/L2/L3/Full及模型/原生验收。

随后用户要求提PR：同步 origin/main 17229133，解决 Spec 索引冲突并保留主干新增的会话恢复文档。复核结果：264份Markdown、1666条本地导航、0错误；84身份、10草稿、3正式行；技能元数据、18项离线状态测试及 git diff origin/main --check 全部通过。仅发布本次流程修改PR，开发任务仍未启动。
