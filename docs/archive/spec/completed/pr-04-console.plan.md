# PR 4C Plan：执行控制台与轮次折叠

任务 ID：2；任务名称：执行控制台与轮次折叠。

依据：[Spec](pr-04-console.spec.md)。依赖：PR 1–3、4A、4B 审核合并。保留原 PR4 文件路径，本阶段为 4C。

本次实现与验收记录：[PR 4C 控制台验证](https://github.com/erranliu/SteerLine/blob/172cea6c6530b6693b742324a9c1d4f9aeb17b6d/docs/validation/pr-04c-console.md)。

## 实施步骤

1. **整理 UI 依赖。** 从最新 origin/main 开始，将现有 viewer 主模型迁入 `internal/steerline/console`，拆成观察接口和控制接口；前者读 4A/4B 的执行、会话、能力 DTO，后者返回 request_id/回执。复用已落地 Query/Command 接口，不新增宿主分支或文件权威。提供一个当前格式的 UI 入口。
2. **加入多项目模型。** 复用 `ListProjects`，维护项目独立的选择、筛选、错误、日志 cursor 和在途请求。所有 tea.Msg 带项目身份与 generation。读取配置错误成为项目行状态，不阻断其他项目。
3. **实现主页面。** 顶部服务/模式/新鲜度，项目导航，ready/runs 筛选列表，详情与 activity/attempt 切页。按稳定 ID 保存选择，窄屏改用单面板切页。
4. **接入服务生命周期。** 把 PR 3 连接/停止结果和 Windows“需要服务窗口”入口变成明确 UI 状态；窗口初始化期间显示日志入口和 startup_id；退出只结束 tea.Program，不创建或关闭服务窗口。
5. **接入任务与派发动作。** 用 PR 2 client 提交 start/mode/pause/resume/stop。操作键和文本输入焦点分离；跟踪同一 request_id，独立呈现请求状态与 run 状态。服务条件变化时刷新并显示拒绝原因。
6. **展示决策。** 显示待决策问题及固定身份，不保留旧答案文件提交入口；回答与恢复交由 PR 5 的唯一命令入口。
7. **修改入口与说明。** `main.go` 增 console 子命令及配置选择逻辑；修改嵌入式启动器和 projects 菜单，加入 console.bat；调整启动测试和用户 README。
8. **实现统一会话页。** 使用稳定复合身份分组，区分过程/最终回复/执行摘要；处理 turn 终态、late final、final_absent/unknown、宿主等待和 gap。复用 4B 事件与正文分页，不在 View 中解析平台字符串或修改业务状态。
9. **实现折叠状态。** 使用独立 reducer 管理 auto/manual、保持展开和终态已处理标记；新增 UI 私有偏好文件、各窗口视图身份及版本/损坏恢复。后台刷新不能覆盖手动意图；按 U8 维护正文/组锚点、历史阅读和显式跟随。
10. **完成可见性。** 错误、问题和缺口置于折叠区外；加入正文完整性、分页失败与能力降级提示。普通正文安全处理终端控制字符；更新用户文档中的 commentary/final、turn 与业务完成说明。

## 验证

- 用 fake QuerySource/ControlClient 验证页面状态和用户意图：Enter 不启动、正确 task_id、双击只产生一次请求、迟到回执隔离、stale revision 不自动重试。
- 使用 fake clock 驱动前台/后台轮询，确保慢请求不重叠，退出清理轮询。
- 用 PR 2 的假服务/任务源/worker 完成 UI 模型端到端测试，不仅断言视图字符串，还检查实际请求内容和 Claims/Starts 计数。
- 真实 Windows TTY 冒烟覆盖多项目、固定入口提示、用户独立打开服务窗口、选择执行、暂停、退出重开、停止及关窗后的离线/重启提示；使用测试项目和协议替身，避免消耗真实任务。
- 查看 80×24 与宽屏，验证中文、长问题、长路径、错误换行和焦点；记录交互结果，不能仅凭静态字符串单测确认可用。
- 运行 console、projects/launchers 与 CLI 路由相关测试；无需再次做没有受影响的全套宿主审计。
- 折叠 reducer 用 4B 两来源真实 fixtures 跑同一脚本，断言组身份、终态仅处理一次、手动覆盖、late final、无 final/未知、等待与 gap 可见。折叠动作不得发控制指令；测试断言实际调用数为零。
- 多项目/同 session 多轮/多窗口/重连重开测试同时验证消息、回执、折叠偏好和阅读锚点隔离；大正文分页校验完整文本与 UTF-8 边界，不只做快照字符串匹配。
- 真实 Windows TTY 的 80×24 和宽屏分别运行 local 与 Orca 已验证输出路径：结束收起、手动展开、保留阅读位置、完整 final、失败和等待提示；保存版本、交互步骤与结果。协议缺口仍按 4B 报告，不能由 UI 静态样例代验。
- 运行 `go test ./... -count=1 -timeout 10m`、`go vet ./...`、平台构建和 `git diff --check`；在 docs/validation/ 下按 U-A ID 记录证据与未验证项。

## 完成条件

- U-A1～U-A15 全部满足；Windows 控制台可引导用户独立打开服务窗口，再完成“连接服务 → 指定任务执行 → 状态查看 → 停服”，双链路共用消息折叠与完整回复页。
- UI 不直接写 runtime、不通过 shell 拼接任务 ID，也不引入第二套业务守卫。
- 当前 console 与 projects 入口有回归验证。

## 风险与回退

多项目和多种异步消息会放大对象错配风险，先实现消息身份，再接动作。不支持的旧 UI 或状态格式应明确拒绝，不能让其发出有副作用命令。

## 测试计划（2026-10-07 导航维护）

本次仅将已清理的证据导航改为固定 Git 快照，不改原实现契约或追认历史通过。最小范围为文档静态 `doc-links`、`task-identities`、`whitespace`：当前 Windows/Git/Python 3 环境，交付前执行 `python scripts/check_doc_links.py`、`python scripts/roadmap_task_id.py check`、`git diff --check`；通过条件为无缺失本地链接、任务身份一致及无空白错误。固定历史链接的目标另用 `git cat-file --batch-check` 核对存在；结果见[本次清理验证](../../../acceptance/history.md#测试计划与本次结果)。不选择 L1/L2/L3、Full 或原生模型组；导航变更不需要这些验证，不新增场景或环境能力。
