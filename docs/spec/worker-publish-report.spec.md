# 发布结果补报

普通开发，用户直接授权。配套 [Plan](worker-publish-report.plan.md)、[验收](../acceptance/worker-publish-report.md)。本次调整覆盖原分步流程/提示词收敛文档中“publish 不增加报告轮”的限制。

`publish` 返回合法结构化结果时沿用直接处理：done 核验实际 PR 后等待合并，needs_input 等输入，blocked 按已知阻断/未知外部结果处理。已停止、来源核验且工作区可核实的 publish 缺结果或格式不合约时，在同一 worker/session/Attempt 内自动排队一次 `publish.report`；原生终态/正文来源或工作区未知仍先对账，不据此启动新输入。

补报绑定原 publish CommandID/TurnID、冻结 candidate SHA、原发布快照和正文/失败事实；仅整理已有结果，不 push、不创建/修改 PR、不改代码/commit、不重试原命令、不新增检查。沿用已冻结的原生报告权限能力，发送前后比较工作区内容/HEAD/绑定 PR，不能声称完整拦截外部写入。

补报不再自动重试。其结果仍要求严格 JSON，done 必须有实际 PR URL；中心核验 repository/base/head/候选 SHA 后才绑定 PR。补报不得覆盖原发布失败，原失败只可进入只读对账/真实合并交付核验。合法 needs_input/blocked 保留对应状态。既有恢复人工继续门槛、原输入 first-wins、原子提交和唯一 TaskDelivery 保持。

控制台展示 publish.report 为“发布报告”，报告阶段正文默认查询原 publish 回合。安装环境不更新或修补，103 的既有状态不改写。

自动验收项见专属验收记录。本次无独立人工验收门槛。

当前模型输入按 [提示词精简](worker-prompt-convergence.spec.md#2026-10-10-当前提示词精简)裁剪：补报正常利用原会话，不重复正文/快照/候选；内部来源和候选绑定保持。publish 要求严格 JSON，done 必须含结构化 `outputs.pr_url`，与发布补报共用结果契约。合法成功结果查询并核验实际 PR 后直接进入 await_merge，不增加 publish.report；缺失或格式错误仍按本文资格补报。最小验证见 [WPR-A5](../acceptance/worker-publish-report.md)。
