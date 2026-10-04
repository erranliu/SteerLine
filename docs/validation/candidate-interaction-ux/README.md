# 并行交互流程增量

用户反馈：五候选信息过载、输入/提交空间不明确，主界面缺少并行数、待交互数和短编号。

本地分支 `codex/candidate-interaction-ux`，以 origin/main `a8ff0f01d38299d77ecc9cd4463b78a1286e1bec` 为起点。此前保留为本地改动；现按用户要求 rebase 后另提新 PR，不修改已合并的 PR #208；用户已有 `candidate-console/windows/manual-20261004-171421/` 现场保留。

改动：主界面显示参与并行数、待交互数、当前 P 编号；短编号按同任务完整 WorkflowRun 的 CreatedAt/WorkflowRunID 排序计算；跨窗口相同，退出历史仍占原位置，F6/F7 切换（原花括号继续可用）。a/Enter 打开当前反馈，无选项直接输入；默认显示 session、问题/选择、输入焦点、固定提交预览，F8 查看完整背景/选项/身份，d 保留完整历史/资源详情。F2 保存原命令并结束本次编辑，回到所选会话等待原回执，原问题不重复邀请输入；新 Decision 仍可交互。保存不代表送达或已处理，业务 Decision 不由客户端改写。迟到拒绝不抢另一个候选的输入页。

验证：

- [console-verified/summary.json](console-verified/summary.json)：71 项通过，无 fail/skip；包括 UX1–UX4、CC1–CC4、原编辑/分页/选择与守卫。
- [wiring-final/summary.json](wiring-final/summary.json)：2 项通过，无 fail/skip；真实应用/客户端/SQLite/Git/受控后端接线、原目标反馈及原合并/放弃守卫。早期 wiring 失败为旧测试使用 Enter 读取完整历史，已按新的明确 d 入口更新操作，退出/停止断言保持。
- [l1/summary.json](l1/summary.json)：本增量一次固定 L1 全部通过，无 Full/L2/L3 或真实模型请求。

CC5 现已由用户人工确认通过，见[人工确认记录](manual-acceptance-20261004.md)。Go fixture、文字布局、准备日志和历史工具拒绝均不作为原生通过依据。

编号计算补充：[trigger-order/summary.json](trigger-order/summary.json) 全部受影响 console 回归通过。新增 UX5 核对完整任务历史的触发顺序、独立窗口一致性、部分可见 run、退出历史和同时间戳稳定排序；移除窗口私有编号持久化。编号读取 WorkflowRun.CreatedAt，时间相同以 WorkflowRunID 排序；显示和导航按同一触发顺序，操作仍使用原完整 ID。

Rebase：已整合最新 main `39059035152a76a8a0d8f0036eb3fa2f6155ac90`（#73 类型归属重构），冲突解决保留新的产品 API / execution model 类型归属及本次简化呈现；用户运行中的手工 fixture 文件从变更提交中分离并保留，未停止或改写验收进程。

最终 main：rebase 期间 main 又合入 #212 的一期本机范围调整，本分支再次对齐 `80469d8a7f861d3cdddc0a2debef2b7e27e94b2e`，保留 #39 的新任务名称与本机范围，只更新 #38 CC5 人工通过状态。

最终验证选择：`final-main/console`、`final-main/wiring` 与 `final-main/l1`；`final-main-build.json` 为当前 Linux/amd64 编译代表，Windows 由 Go 测试和产品 fixture 构建覆盖。历史 rebase/ 和此前 l1/ 为各自源码快照，最终报告以后列选择为准。人工通过依据为用户确认，自动结果不替代人工结论。

最终验证结果：console 72 项、真实接线 2 项及最终 L1 7 组全部通过，无 fail/skip；任务身份校验 83 文件通过，git diff --check 通过。新 PR 对 main，CC5 按用户人工确认登记为通过，不合并 PR。
