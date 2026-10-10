# Orca E2E 覆盖补齐

当前范围调整（2026-10-10）：旧本机模型场景已随产品入口停用而退役。原生无模型工作区场景仍运行；当前 Orca 1.4.223 的 `worktree rm` 返回正式异步删除回执，但删除后不保证旧 `.orca-worktree-trash` 目录留存。OEC-02 现核对本次精确工作区的正式删除回执、原路径消失和 Git 注销，行政目录实际状态如实登记；OEC-03 的公开查询、控制台、原身份重启及无关任务启动检查保留。下面的旧版本真实 trash 记录只证明原构建，不作为当前 CLI 必须永久留下该目录的要求。

归属：本次 Orca 测试覆盖修复。产品问题修复见 PR #251；本记录维护新增测试与真实验证，不改变 Task/DAG 状态。

| 验收项 | 场景与通过条件 | 入口 |
| --- | --- | --- |
| OEC-01 | Orca smoke 显式使用 `args=[]`；冻结配置仍为空数组，真实 worker/worktree 启动并交付，沿用原 usage/PR/依赖断言 | L2 `smoke.orca-codex`；与 L3 `delivery.orca-codex` 共用断言 |
| OEC-02 | 原生 Orca CLI 创建无 agent 工作区，在服务运行中删除；默认 state/worktrees 根产生真实 trash，保留原项目/数据库/执行身份 | L3 native `workspace.orca-removal-restart` |
| OEC-03 | 删除后公开 CLI、真实控制台和服务重启成功；无关任务保持就绪且随后能启动，无真实模型调用 | 同 OEC-02 |

运行方式见 [E2E README](../../tests/e2e/README.md)。native 环境单独选择，不能混入 L2 或伪装为模型交付；环境缺失、零匹配、跳过或清理失败均不能通过。

## 验证记录

2026-10-09，Windows，本轮源码基于 `692a784e`，测试构建明确记录 `source_dirty=true`：

| 范围 | 实际结果 / 证据 |
| --- | --- |
| OEC-01，共用原生交付实现 | L3 live `delivery.orca-codex` 通过；冻结 `args=[]`、真实 worktree/worker、readiness、交付/合并及正数 usage 均通过。报告：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T015825.157888600Z/summary.json`。未执行完整 L2 双链路，不宣称 L2 全验。 |
| OEC-02/OEC-03 | L3 native `workspace.orca-removal-restart` 通过，真实创建/删除、默认 state/worktrees 根的 trash、删除后公开查询/控制台、原身份重启及无关任务交付均通过；`native_workspace_verified=true`、`usage_verified=false`、无真实 PR，清理通过。最终报告：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T015515.745121200Z/summary.json`。 |
| 确定性 / 边界 | L1 与 runner 定向回归通过：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T015611.056142800Z/summary.json`；native/evidence/helper/driver/testenv、Windows pathguard 单测及 E2E 分层边界通过。文档链接与 diff 检查通过。 |

原生 checkout 与受控 worker 工作区独立，均使用当前安装版默认根；场景在服务运行时删除原生 checkout，保留原有业务执行。准备过程通过原生 CLI 修改本次拥有的 setup，不修改用户已有仓库或显示设置。启停期间公开只读查询允许按场景期限等待；派发写请求不重试。受控 helper 已支持独立会话与 PR 的原身份查询，重复业务输入和同分支重复 PR 仍拒绝。

测试还复现了 Windows SQLite sidecar 创建/删除窗口的短暂 `ACCESS_DENIED` / `SHARING_VIOLATION`。文件属性和链接数量检查现对同一次读取做最多约 95ms 的有界重试；持续错误仍失败，路径越界和硬链接规则未放宽。已用失败闭合单测及最终无额外常驻 reader 的原生场景验证。

准备阶段的失败报告保留在原始临时目录，不作为通过证据。没有人工验收项；后续完整 L2 可按原固定门槛运行。
