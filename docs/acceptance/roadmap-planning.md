# 当前规划文档检查

本页记录当前文档维护的静态检查；历史PR和前序规划检查见[归档](archive/completed/roadmap-planning.md)。产品/任务验收仍在各自专属记录，静态检查不证明中心DAG同步或执行资格。

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
