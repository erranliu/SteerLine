# 并行交互流程增量

用户反馈：五候选信息过载、输入/提交空间不明确，主界面缺少并行数、待交互数和短编号。

本地分支 `codex/candidate-interaction-ux`，以 origin/main `a8ff0f01d38299d77ecc9cd4463b78a1286e1bec` 为起点。未提交/推送或修改已合并的 PR #208；用户已有 `candidate-console/windows/manual-20261004-171421/` 现场保留。

改动：主界面显示参与并行数、待交互数、当前 P 编号；短编号按同任务完整 WorkflowRun 的 CreatedAt/WorkflowRunID 排序计算；跨窗口相同，退出历史仍占原位置，F6/F7 切换（原花括号继续可用）。a/Enter 打开当前反馈，无选项直接输入；默认显示 session、问题/选择、输入焦点、固定提交预览，F8 查看完整背景/选项/身份，d 保留完整历史/资源详情。F2 保存原命令并结束本次编辑，回到所选会话等待原回执，原问题不重复邀请输入；新 Decision 仍可交互。保存不代表送达或已处理，业务 Decision 不由客户端改写。迟到拒绝不抢另一个候选的输入页。

验证：

- [console-verified/summary.json](console-verified/summary.json)：71 项通过，无 fail/skip；包括 UX1–UX4、CC1–CC4、原编辑/分页/选择与守卫。
- [wiring-final/summary.json](wiring-final/summary.json)：2 项通过，无 fail/skip；真实应用/客户端/SQLite/Git/受控后端接线、原目标反馈及原合并/放弃守卫。早期 wiring 失败为旧测试使用 Enter 读取完整历史，已按新的明确 d 入口更新操作，退出/停止断言保持。
- [l1/summary.json](l1/summary.json)：本增量一次固定 L1 全部通过，无 Full/L2/L3 或真实模型请求。

CC5 仍待人工补验。本次改变输入/渲染流程，Go fixture、文字布局和旧原生现场均不能代替新 Windows 中文组词、选项加备注、导航、窄窗口与滚动证据。

编号计算补充：[trigger-order/summary.json](trigger-order/summary.json) 全部受影响 console 回归通过。新增 UX5 核对完整任务历史的触发顺序、独立窗口一致性、部分可见 run、退出历史和同时间戳稳定排序；移除窗口私有编号持久化。编号读取 WorkflowRun.CreatedAt，时间相同以 WorkflowRunID 排序；显示和导航按同一触发顺序，操作仍使用原完整 ID。
