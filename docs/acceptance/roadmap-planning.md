# Roadmap 文档与规划入口检查

本摘要记录规划文档的自动静态检查。下列原 PR #232 结果属于当时内容；后续修订分别登记，不追认为历史结果重跑。

## 原 PR #232 的规划入口检查

原范围为控制台文档收敛、并行开发指南、草稿/正式边界、RoadmapGuide 与任务身份规则。2026-10-07 冲突处理基于主线 ae32f300，保留其验证目录清理和摘要政策；未恢复旧报告目录，未改变产品逻辑、中心 DAG、身份 JSON 或原 Spec/Plan。

当前约定：规划先读 RoadmapGuide，草稿放 drafts/，可提前取得唯一 Task ID；正式准入沿用该 ID，纳入 execution-map 的受控流程才创建中心 Task/DAG，两边结果核对后可执行。正式表仍为 0 项且未核对实际 DAG，不宣称中心为空。

| 检查 | 命令 / 范围 | 结果与限制 |
| --- | --- | --- |
| 草稿/正式静态边界 | python -m unittest discover -s scripts -p test_roadmap_task_id.py -v | 本次 10/10 通过，0.152 秒；多份、嵌套、备选草稿和独立正式契约检查 |
| 身份、链接、差异 | python scripts/roadmap_task_id.py check；python scripts/check_doc_links.py；git diff --check | 本次 84 身份、13 草稿/0 正式有效；223 份 Markdown、1,363 个本地链接，0 缺失；差异通过。静态检查不证明 live DAG 同步 |
| 执行技能格式 | python -X utf8 <skill-creator>/scripts/quick_validate.py .agents/skills/roadmap-runner | 本次通过；只校验技能文件，不启动 session、worktree、heartbeat 或业务执行 |

原始检查输出：`C:/Users/lmas1/AppData/Local/Temp/steerline-pr232-conflicts-34aa3c8b6d0c47c5bf6f80f95602c568/checks.log`，不提交到仓库。本次未修改离线 runner 状态助手，其此前 18 项通过不能冒充新的 DAG/调度行为测试；未跑产品 L1/L2/L3、Full 或模型场景。

产品原生验收及历史原始证据见[验收入口](README.md)，各原结果仍绑定其原源码与环境。旧路径、命令、SHA 和未通过限制不被重写为当前通过。

## 2026-10-07 验收归属与草稿保留规则

方法：自动。范围：所有验收记录迁入 acceptance、原 #42/#53 转为人工组织活动、Guide/正式清单/runner 分类规则与草稿保留策略。只修订文档和技能指令，不修改执行代码、身份 JSON 或外部 SteerLine Task/DAG；草稿 11 项，正式成员仍 0 项且待同步。

| 检查项 | 状态 | 环境 / 命令 / 通过条件 | 结果与证据 |
| --- | --- | --- | --- |
| 文档导航 / doc-links | 已验收 | Windows/Python；python scripts/check_doc_links.py；本地导航无缺失 | 234 份 Markdown、1,469 个本地链接，0 errors；links.log |
| Task 身份与正式边界 / task-identities | 已验收 | 同上；python scripts/roadmap_task_id.py check；身份一致、正式表不混入人工活动 | 84 身份、11 草稿、0 正式；ids.log，不证明实际 DAG |
| 技能格式 / roadmap-runner | 已验收 | Python UTF-8；python -X utf8 <skill-creator>/scripts/quick_validate.py .agents/skills/roadmap-runner；格式有效 | Skill is valid；skill.log；不启动任务或 heartbeat |
| 差异格式 / whitespace | 已验收 | Git；git diff origin/main --check；无格式错误 | 通过；diff.log |
| 迁移完整性 / evidence-integrity | 已验收 | Python/Git；按 git diff -M 的迁移列表比较 origin/main 原 blob 与当前文件（Git 换行归一）；确认仅文档变更、身份 JSON 未变 | 20 份 JSON/JSONL 内容保持，终端检查输出；历史结果不重新标为当前验收 |

执行时点：修订及主线同步完成后、提交前。基线同步到 main `8e46537f`，两处文档冲突采用最新固定原生配置与统一验收入口；主线新增的控制台/L1/配置记录一并迁入，精选 JSON/JSONL 保持原内容。原始输出目录：`C:/Users/lmas1/AppData/Local/Temp/steerline-acceptance-policy-72f1431ca63141c89fd7f9b3923640f2/`。最近基线 JSON 原文件内容保持不变，历史通过范围没有扩大；新阶段活动的所有项仍待验收。

未选产品 L1/L2/L3/Full、模型/原生验收或新增环境：没有产品实现或状态助手代码修改，所需验证仅是上述静态检查。不把这些结果算作自动准入、实际 DAG 同步或阶段签收已验收。
