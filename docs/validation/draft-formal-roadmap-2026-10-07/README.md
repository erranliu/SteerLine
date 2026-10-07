# 草稿与正式 DAG 执行入口分离

日期：2026-10-07（Asia/Shanghai）。本次只修改文档、身份静态检查与执行技能入口；没有读取/清空中心数据库，没有取消任务或 worker，没有启动 session、heartbeat 或执行流程。

roadmap.md 保留 13 项草稿候选和已有编号/Spec/Plan，展示剩余拟议前置；已交付持续 worker 与本机候选作为基线摘要，#36/#39 不继续作为草稿待完成依赖。完整历史依赖和固定契约不改写。

execution-map.md 的正式条目已按要求清空，静态计数为 0；文件明确同步状态待核对，不宣称实际中心 DAG 为空。它只接受真实 DAG 准入/查询回执核对过的任务，日常执行从此选择，空表或不同步时不能从草稿或旧 run 自动补入。

身份检查独立校验草稿与正式文件，草稿允许未完成文档，正式表必须有完整独立 Spec/Plan；二者不要求相同集合或版本链接。该静态工具不校验实时 DAG。roadmap-runner 的入口、提示、运行示例和 UI 提示同步指向正式表，保留离线状态助手的用途和原权限/审核门槛。

## 最小验证

| 范围 / 场景 | 环境与命令 | 时点 / 通过条件 | 证据 |
| --- | --- | --- | --- |
| 草稿/正式静态边界 | Python 3：python -m unittest discover -s scripts -p test_roadmap_task_id.py -v | 修改后，7 项临时目录用例通过：空正式表不接纳草稿、未完成草稿、独立正式成员/文档版本、缺契约/错误身份/重复条目拒绝 | checks.log |
| 身份与导航 | python scripts/roadmap_task_id.py check；python scripts/check_doc_links.py | 文档修正后，身份有效；13 草稿/0 正式，链接无缺失；输出明确不验证 live DAG | checks.log |
| 技能与离线助手 | python -X utf8 <skill-creator>/scripts/quick_validate.py .agents/skills/roadmap-runner；python -m unittest discover -s .agents/skills/roadmap-runner/scripts -p test_state.py -v | 入口修正后，skill 格式有效，18 项原离线状态测试通过；不调用业务服务或平台 | checks.log |
| 差异与保留数据 | git diff --check；核对 Task 身份 JSON 和原 Spec/Plan 未修改 | 提交前，无格式错误；不重分配编号，不改正式契约或原证据 | checks.log |

不跑产品 L1/L2/L3、Full、模型或原生场景：本次没有修改产品执行逻辑、测试环境、codec 或状态格式。技能检查使用 Python UTF-8 模式，避免 Windows 默认 GBK 读取中文 frontmatter；未改外部验证器。

二期自动审核和服务自动同步未在本次实现；现有受控任务创建与回执可用于正式准入，具体新审核协议仍需正式 Spec/Plan。文档重置不作为中心 DAG 操作回执，原已启动工作和历史数据保持。
