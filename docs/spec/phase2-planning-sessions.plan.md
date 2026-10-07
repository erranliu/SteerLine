# Plan：Codex 规划会话服务

任务 ID：47；任务名称：Codex 规划会话服务；目标：`PHASE2`。唯一执行依据：[Spec](phase2-planning-sessions.spec.md)。一个独立实现 PR 交付。

硬前置：#43、#46。全部按原任务交付核验，不能按接口里程碑提前执行。

## 实施顺序

1. 在 #43 的固定版本证据和 #46 guard 上实现 app-server 专用适配；服务配置显式核验权限与工具集合。
2. 接会话/轮次命令及规范事件、正文存储，建立请求到外部 thread/turn 的持久身份。
3. 把受控草稿工具接到 #46，验证 session、turn、generation 和 file_id；人工产品命令不进入 agent 工具集。
4. 实现服务停止与未知轮次对账，分别注入发送丢回执、工具落盘后、终态前故障；检查不重发不确定动作。
5. 在真实 Windows 完成 PS1–PS6，与 #48 并行开发但共用接线串行集成；未有 #48 时使用 #46 publish owner 夹具，后合入方回归已交付消费者，真实跨组件验收由 [二期验收活动](../acceptance/phase2.md) 承担。

## 验证与交接

按本任务验收编号逐项记录于 docs/acceptance/phase2-planning-sessions.md；保存数据库/文件/Git/外部身份和前后状态的最小摘要，原始产物放临时目录，不以最终文本代替应用事实。先跑领域定向和故障测试，再跑受影响 Go 全量回归、go vet 与 git diff --check，服从所引用共同 Spec 的测试门槛。需要真机的项单列环境和未测项。公共接口变化同时更新消费者 Spec、schema 和契约样例，不通过添加旧协议兼容掩盖不一致。

## 本次文档修订测试计划

本次只调整验收归属与导航，不实施上述能力。正式准入前按实际源码补齐 PS1–PS6 的最小实现测试命令与环境；不把本次静态结果记为产品验收。

| 级别 / 场景 | 环境 / 命令 | 时点 / 通过条件 | 证据 |
| --- | --- | --- | --- |
| 文档静态 / doc-links、task-identities、whitespace | Windows、Python/Git；python scripts/check_doc_links.py；python scripts/roadmap_task_id.py check；git diff --check | 本次修订交付前；导航存在、身份一致、无差异格式错误 | [规划文档验收](../acceptance/roadmap-planning.md)与临时检查输出 |

未选产品 L1/L2/L3/Full：此次没有实现、测试环境或场景改动。
