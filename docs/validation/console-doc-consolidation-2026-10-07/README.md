# 执行控制台与一期交付说明收敛

日期：2026-10-07（Asia/Shanghai）。源码基线：172cea6c，已包含当前控制台简化。只修改文档，不改变产品逻辑、任务身份、中心 DAG 或原始验收事实。

## 内容归属

| 原入口 | 承接位置 |
| --- | --- |
| 控制台规划约束：Windows 服务窗口、离线和停服确认 | docs/architecture.md 的生命周期边界 |
| 控制台规划约束：问题、传输事实、后端能力差异 | docs/product-design/decisions.md |
| 控制台规划约束：候选集合、放弃与唯一交付 | docs/product-design/data-model.md |
| 控制台布局、候选选择、草稿隔离及键位 | docs/product-design/execution-console.md |
| 会话采集、最终正文与窗口操作记录 | docs/product-design/conversation-events.md，保持原读模型和取证范围 |
| 一期产品边界、剩余任务与阶段关系 | 已有 product-vision、roadmap 与执行依赖，修复导航后删除平行说明 |
| 一期具体隔离、持续运行与重启门槛 | 已固定 #83/#42 的独立 Spec/Plan，保留原要求 |

删除 roadmap 下 console-constraints.md 和 phase-1-delivery.md。当前 TUI 已移除过程时间线和轮次折叠，不沿旧约束重新声明这些能力；拟议 WinUI 交互仍由自己的设计和未交付 Spec 管理。根 README 只保留控制台启动与设计入口，操作表由新的当前控制台文档维护。

历史 CC5 人工确认继续留在原 validation 记录，不再复制到长期设计。两份历史 Markdown 报告仅修正已删除入口的导航，其命令、commit 和验收事实不改写；历史 Canonical 路径保持。

## 最小检查

| 级别 / 场景 ID | 环境与命令 | 时点 | 通过条件 | 证据 |
| --- | --- | --- | --- | --- |
| 文档静态 / doc-links | Windows、Python 3：python scripts/check_doc_links.py | 吸收、删除与引用修正后 | 本地文件/目录导航零缺失 | checks.log |
| 文档静态 / task-identities | python scripts/roadmap_task_id.py check | 文档交付前 | 当前任务身份、名称与执行入口一致 | checks.log |
| 文档静态 / source-keys | 只读核对 model_keys.go、view.go、service_client.go | 操作表及生命周期说明完成后 | 当前三窗/原地反馈、F2/Enter、候选与详情键位、停服回执边界与源码一致 | checks.log |
| 文档静态 / whitespace、scope | git diff --check；核对本次差异范围 | 提交前 | 无格式错误，仅文档与检查输出，产品实现未变 | checks.log |

不运行 L1/L2/L3、Full 或模型/原生实验：没有实现、协议、测试登记或接线变更；原未闭合验收保持原状态。无需新增产品测试场景或环境能力。链接检查不验证历史章节锚点、外部网页或原试验可重放性。
