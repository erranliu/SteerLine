# 活动中心任务候选原会话阅读

2026-11-12，基于 SteerLine `805cfcae` 与 SteerRunner `37de4694` 的工作树改动。项目活动独立页按需读取候选的结构化 Orca 原会话。列表和摘要继续使用原本地预览。原生会话按最近一页开始，加载更早消息后按原顺序显示。长消息在用户展开后读取选中消息的一段。原会话不可读时显示重试入口，已记录交互仍能独立展开。

- SteerLine L1、L2 均通过。报告位于 `C:\SteerlineDebug\activity-conversation\test-results\6907ceb3e4364ba3b2597d9fcf872716\L1\summary.json` 和同目录下的 `L2\summary.json`。固定集合后追加的改动由活动页及后端定向测试复验。
- 活动页 9 项通过，覆盖点击前零历史请求、每次操作请求一页、前插更早消息、部分历史提示和按条展开正文。后端定向验证通过，覆盖项目和候选约束、固定 Runner CLI 参数、一次查询、禁止写入信封和拒绝另一会话响应。
- Runner Orca 定向测试通过，覆盖原 epoch 和 Attempt 校验、工具与组装指令过滤、原生分页和逐段恢复 UTF-8 正文。Runner 的定向应用、服务身份绑定和 Orca 读取用例均通过，原始输出在同批次的 `runner-targeted.txt`。较广包回归补验的 CLI、契约、服务、资源与 Orca 通过，application 包在已有用例 `TestRABudgetReportOnce` 的 Git 子进程中触发 10 分钟总超时。
- `merge-h-308-code` 任务 #18 的 Runner 现场无可绑定服务实例，未在该现场读取原生历史。当前公开原生读取适配结构化 Orca。更早 Attempt 在有持久交互时可查看已记录交互，未验证的原生部分显示缺口。本轮未部署。
