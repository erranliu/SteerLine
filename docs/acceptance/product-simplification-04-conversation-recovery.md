# 会话与恢复精简验收

仅归属[本项 Spec](../spec/product-simplification-04-conversation-recovery.spec.md)，实施与命令见[Plan](../spec/product-simplification-04-conversation-recovery.plan.md)。本轮用户直接授权工作区实施，未创建外部 Task/DAG 或声明工具交付。

## 实现与范围

中心删除独立 JSONL 会话重放、raw 字节表/读接口、终端正文与普通消息/工具过程映射、增量 current-message 表及无消费者 mapper/位置端口。完整 final 只由明确原生 phase 和完成时间产生，正文不可变；必要 turn/session、wait/gap、问题/答案、Activity 摘要、操作日志及投递/停止/PR 对账继续由原所有者保存。准备轮与已由迟到 final 解除的缺正文提示不进入业务 final 索引，其他采集缺口继续显示。

恢复只允许原 session；删除 new_session 及无原 session 的人工新建 worker。原会话无法核实时保留历史并进入人工处理，不展示可创建替代会话的继续选项。核实旧进程树停止后仍可用新进程/Attempt 恢复原 thread。SQLite 写入同时拒绝恢复更换 session 或从空 session 新建恢复代次，原子失败保留原 Attempt。

SQLite schema17、ConversationEvent3、产品会话游标3；旧开发格式明确拒绝，不迁移。checkpoint19、UI 操作 journal2、原生协议和配置保持。本次没有安装新二进制、清空业务数据库或操作用户 worker。

## 自动验收

日期：2026-10-07；Windows，当前工作区；源码基线 `4eb722f16ceaccdcf3c19828213727e66bee403f`。原始产物根为 `C:/Users/lmas1/AppData/Local/Temp/steerline-ps4-20261007-221207041/`，只保留本文件的最小摘要，不提交完整日志/数据库/原始输入。

| 项 | 实际结果与证据（相对产物根） |
| --- | --- |
| PS4-01/02 采集、正文与 UI | `directed-final/summary.json`：passed/complete，331项中330通过、1项原生 opt-in 跳过。五个受影响包完整定向回归，覆盖混合 turn 只留完整 final、中文/空 final、缺口、幂等、不可变正文、分页摘要、准备轮过滤、候选隔离和 UI；跳过不记为 native 已验 |
| PS4-01/02 最终采集边界 | `final-collection/summary.json`：6项通过，原生历史迟到 final、普通输出不持久化、host wait 不冒充业务问题；最后补充 SQL 三值逻辑/准备缺口检查后由 directed-final 复验 |
| PS4-03 替代会话拒绝及旧问题刷新 | `missing-session-fixed/summary.json`：2项通过；拒绝 new_session、不产生新 Attempt、缺失/不可核实会话转换为无继续选项的人工问题，旧问题保留历史 |
| PS4-03/04 恢复事务 | `same-session-transaction-fixed/summary.json`：4项（含父项）通过，原 session 新代次提交、checkpoint 失败回滚、更换 session 被 SQLite 拒绝并回滚。夹具明确已有原 session |
| PS4-03 原终端/冻结配置/原问题反馈 | `original-terminal-core/summary.json`：通过，215.502秒。Windows 原生协议 fixture：服务重开保留原 terminal/incarnation/session/workspace/dispatch，当前配置改变不改冻结快照；重新确认后原问题 reply/ack 不重复；没有新交付。该回归独立结束于原会话重连与反馈；原完整故障注入测试范围和期限保留 |
| L1 | `l1-final/summary.json`：passed/complete，12固定组通过，准备1.1秒、执行173.4秒。先前 L1 亦通过；增加 SQLite 同 session 约束后按需复验最终代码 |
| PS4-04 受控重启交付 | `controlled/summary.json`：selected-L3 / recovery.waiting-merge-restart 通过，准备8.879秒、执行311.449秒，cleanup_complete=true；原身份、一次 Delivery 和后继释放保持。controlled，不是模型/远端/native 认证 |
| 编译/静态 | 产品构建、受影响 go vet、E2E tag 全场景编译、文档链接及 diff 检查通过。编译不记为运行验收；无 Full/完整 L2/live |

以上父/子项计数与耗时可能重叠，不累加为独立场景数或总耗时。L1-final、directed-final 在最终业务代码完成后执行；先前代表场景的后续变化仅为已由最终定向覆盖的查询/事务边界及测试格式，没有重标原运行时间或范围。

## 失败及基线限制

- 首次 `directed/summary.json` 失败：退役过程的旧 fixture 仍试图保存 commentary；修正为验证拒绝，保留原失败，`directed-fixed` 与 `directed-final` 通过。
- `final-guards/summary.json` 发现已有 pending 问题仍显示继续选项；修复为保留旧问题、另建绑定当前恢复状态的人工问题，`missing-session-fixed` 通过。
- `same-session-transaction/summary.json` 的原提交夹具没有原 session，与新恢复契约冲突；明确原 session 后正向提交/回滚仍通过，新增替代 session 拒绝没有降低事务断言。
- 扩大选择的 `recovery/summary.json` 不完整（Go 10分钟超时），其中完整 Orca fixture 在 lost-task 变更阶段超时；原范围单独隔离 `original-terminal-isolated/summary.json` 仍失败。以未修改 HEAD 归档源码运行相同测试，`baseline-native.jsonl` / `baseline-native.stderr.txt` 同样在该故障环节失败（约360秒），不是仅以负载推测归因。未修复或将该完整故障注入测试重标通过；原生故障/安全重建待验继续见[原验收](worker-service-recovery.md)及正式清单。PS4 的原会话重连/反馈代表范围由独立 core 回归验证。
- 全仓 `roadmap_task_id.py check` 仍报告既有 registry #49 与共享 Git ref 不一致；本项没有改编号或外部 DAG，不列为通过。

无独立人工验收活动；本次最小自动要求已验。未运行 live/真实模型，不声称原平台的全部恢复/故障已验。

## 同口径行数

`line-summary.json`：全部已跟踪 Go 加本次新 Go 的物理行，含空白/注释；`*_test.go`、tests 与 internal/testkit 归测试/支撑，其余归实现（含命令工具）。不计 Markdown、JSON、二进制；没有移动文件或压缩排版计收益。

| 类别 | 修改前 | 修改后 | 净减 |
| --- | ---: | ---: | ---: |
| 实现 | 61,268 | 59,481 | 1,787 |
| 测试与支撑 | 48,620 | 47,806 | 814 |
| 合计 | 109,888 | 107,287 | 2,601 |

## PR 基线同步与最终复核

提 PR 前将实现提交同步到 `origin/main` 的 `5fc86f3e9d924b00aa83909a2f1c7aa585435daf`，保留主分支 PR 合并等待门槛修复、测试期限和验收归档导航。Spec 索引冲突按当前主分支结构合并，只增加本轮导航。此前已记录的 Orca 故障对照属于原 `4eb722f1` 时点；主分支已有新的完整回归/原生验收记录，不用旧失败覆盖其实际状态。

同步后证据根：`C:/Users/lmas1/AppData/Local/Temp/steerline-ps4-pr-20261007-231205931/`。

- `l1/summary.json`：12个固定 L1 组均 passed/complete。追加的历史 fixture 组最初因工具混排样本仍读取旧正文索引而失败，含追加项的整份报告保留失败，不重标。
- `history-fixed/summary.json`：修正为只检查完整 final 的实际原生样本后，6个顶层场景及父/子8项通过；验证两个本机后端历史、真实工具混排、取消/失败、迟到 final、缺正文/显式空正文和幂等。原生样本来自固定历史 Git 快照，不是本次 live 模型运行。未重跑已通过且未受测试夹具变化影响的12个固定组。
- 普通受影响包及 E2E tag 全场景编译、文档链接、差异格式通过。实现与原生请求协议无额外变更，不重复受控交付或完整模型验收。
- 最终相对上述 main 基线的同口径 Go 净减：实现1,787行、测试与支撑819行，共2,606行。前表保留实施时点的原始统计，不把本次历史夹具修正回写为原时点结果。
