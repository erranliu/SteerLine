# Roadmap 文档与规划入口检查

本摘要记录控制台文档收敛、并行开发指南、草稿/正式边界、RoadmapGuide 与任务身份规则的最小检查。2026-10-07 冲突处理基于主线 ae32f300，保留其验证目录清理和摘要政策；不恢复旧报告目录，不改变产品逻辑、中心 DAG、身份 JSON 或原 Spec/Plan。

当前约定：规划先读 RoadmapGuide，草稿放 drafts/，可提前取得唯一 Task ID；正式准入沿用该 ID，纳入 execution-map 的受控流程才创建中心 Task/DAG，两边结果核对后可执行。正式表仍为 0 项且未核对实际 DAG，不宣称中心为空。

| 检查 | 命令 / 范围 | 结果与限制 |
| --- | --- | --- |
| 草稿/正式静态边界 | python -m unittest discover -s scripts -p test_roadmap_task_id.py -v | 本次 10/10 通过，0.152 秒；多份、嵌套、备选草稿和独立正式契约检查 |
| 身份、链接、差异 | python scripts/roadmap_task_id.py check；python scripts/check_doc_links.py；git diff --check | 本次 84 身份、13 草稿/0 正式有效；223 份 Markdown、1,363 个本地链接，0 缺失；差异通过。静态检查不证明 live DAG 同步 |
| 执行技能格式 | python -X utf8 <skill-creator>/scripts/quick_validate.py .agents/skills/roadmap-runner | 本次通过；只校验技能文件，不启动 session、worktree、heartbeat 或业务执行 |

原始检查输出：`C:/Users/lmas1/AppData/Local/Temp/steerline-pr232-conflicts-34aa3c8b6d0c47c5bf6f80f95602c568/checks.log`，不提交到仓库。本次未修改离线 runner 状态助手，其此前 18 项通过不能冒充新的 DAG/调度行为测试；未跑产品 L1/L2/L3、Full 或模型场景。

产品原生验收及历史原始证据见[验收入口](README.md)，各原结果仍绑定其原源码与环境。旧路径、命令、SHA 和未通过限制不被重写为当前通过。
