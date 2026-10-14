# Worker 三层职责重构验收

唯一归属：[Worker 三层职责重构 Spec](../spec/worker-three-layer.spec.md)与[Plan](../spec/worker-three-layer.plan.md)。普通开发，无 Task ID，不覆盖后续初始化重试任务。状态：文档已规划，产品自动验收全部待执行；旧任务证据不计为本次通过。

## 自动验收项

步骤与命令 G1–G3/R1–R3 以 [Plan 测试计划](../spec/worker-three-layer.plan.md#测试计划) 为准。每项执行后登记实际源码/文档版本、命令、日期、证据路径、覆盖分支和结论；缺入口、零匹配、意外 skip、证据或 cleanup 缺失不算通过。

| ID | 验收步骤和通过条件 | 命令映射 | 当前状态与证据 |
| --- | --- | --- | --- |
| TL-A01 | 运行三层依赖/符号/传递依赖及负例；检查中层不接业务/Service、Workflow 不读原生字段、adapter 无中心写权限 | G1；结构源码核对 | 待执行 |
| TL-A02 | 用纯 adapter fixture 验证准备/发送/观察/恢复规则及部分成功、未知、身份、期限；Workflow fixture 无模拟 thread/terminal | G1、G3 | 待执行 |
| TL-A03 | 组合提交成功/回滚/重放、意图前置、结果与后继一次提交、新旧 Attempt/原绑定、迟到与兄弟隔离 | G2、R1、R2 | 待执行 |
| TL-A04 | 两 adapter 的正常/错误契约和实际中层接线；Orca local 重接、environment 未知请求对账，原生身份/结果保持，零重发/补 Enter/屏幕门槛 | G3、R3 | 待执行 |
| TL-A05 | 原问题答案、业务反馈、服务换代及停服/原会话恢复；授权只消费一次，未知/旧身份/失效目标拒绝 | G2、G3、R2、R3 | 待执行 |
| TL-A06 | 原 implement/report source、格式次数、publish/固定 PR、合并前后依赖、唯一 Delivery、closed 后零查询/新输入 | G2、R1、R2 | 待执行 |
| TL-A07 | 检查旧路径/转发删除、app/查询/fixture/driver 接线，公开/持久行为和文档归属保持；完整最小回归非零匹配 | G1、R1、R3；结构源码核对 | 待执行 |

## 环境与证据

- G1–G3：现有 Windows/Go 定向 runner、隔离测试 fixture及受控进程，无模型或安装状态；新包用例/边界负例由 P1–P4 补齐。
- R1–R3：已注册的六个 L3 controlled 场景，真实产品、独立 Git/SQLite/home、受控 app-server/Orca native state 与 loopback PR provider；不访问安装环境或真实 merge 项目。
- 原始产物保留在 runner 临时目录；仓库仅登记最小摘要，不复制认证、完整会话过程或业务数据库。
- 记录 input/request/turn/Attempt/资源绑定与计数、事务前后事实及 owned cleanup。各场景可复用既有证据格式，不能手写成功回执冒充原生事实。
- controlled 覆盖不声明本轮真实模型、真实远端、硬件断电或新的代理恢复能力已验。

## 人工验收与阻塞范围

本次未设强制人工项：公开 UI/命令与业务行为保持。发现需要改变公开行为时，先明确新的验证安排；不能将未知问题登记为“人工后置”以绕过必须通过的自动项。真实原生接口变化的验证要求见 Plan。

## 文档静态检查

文档生成后仅登记 D1 的链接、范围/验收映射、场景登记、编号和差异检查。静态检查不改变 TL-A01–TL-A07 的待执行状态。

2026-10-14 文档生成后执行 D1：`python scripts/check_doc_links.py` 通过（379 份 Markdown、2305 个本地链接、0 错误）；`python scripts/roadmap_task_id.py check` 通过（92 个身份文件、0 个草稿行、1 个正式行，未修改规划）；`git diff --check` 通过。

同时静态核对 TL-A01–TL-A07 均映射到 Plan 的测试表，六个 R1–R3 场景均已登记且为 controlled；Spec/Plan/验收正文的编号、索引和空白检查通过。G1 的新包/用例与 G2 的组合提交用例仍按 Plan 待实施。没有运行 Go、L3、模型或原生场景，产品验收状态保持待执行。

## 实施与验收结果

待实施后按上述自动项填写。当前不记录实现完成、模型/PR 通过或安装任务恢复结论。

适用性专项验收（归属 TL-A02/TL-A04）：中层表驱动覆盖 `not_applicable`、`unsupported`、`unverified`、零值与声明/事实冲突；两 adapter 定向用例覆盖各自必要流程和 Codex 跳过 Orca 专属启动/初始化/连接机制。原请求未知只对账，不能由机制跳过扩大权限。待执行后登记真实证据。
