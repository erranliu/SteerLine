# 原生异步问答最小设计

普通开发任务，不纳入 roadmap。验收见 [专属记录](../testing/acceptance/native-async-questions.md)。

Decision 区分 `sync` 与 `async`。同步问答阻塞当前执行；Codex `agentMessage.delivery=async` 的结构化 `questions` 立即保存原问题、全部选项和原 session/turn/item/input command 身份，执行保持运行。控制台显示“运行中 · 有待回答问题”，仍可打开回答草稿。问题身份和内容保持不可变；API 根据 WorkflowRun 当前状态派生 `blocks_progress`，不新增 Workflow 状态。

异步答案先通过原 Decision 命令事务保存。原轮次活动时，用 `turn/steer` 和严格 `expectedTurnId` 投递，带稳定输入身份；不改变业务步骤和轮次计数。送达未知时只核对原 journal/公开 thread history 的输入身份及正文，禁止自动重发或改投其他轮次。

原轮次结束后保留未回答问题，允许原流程执行报告以明确业务结果；报告不重复提问。有效 `needs_input` 保留原 Decision 和选项并进入 `waiting_input`；答案在其他步骤活动时只保存，不注入报告。明确答案需要继续时，绑定原问题及来源步骤启动后续轮次。有效且经成果核验的 `done` 使未使用的异步问题 `superseded`，表示问题已失效；不从自由正文推断完成或阻塞。同步工具原请求回复方式保持原有契约。

固定 `publication_policy` 随模型步骤提供，避免让用户回答已冻结的执行配置。实施不修改安装目录、不迁移旧数据、不新增通用消息总线。
