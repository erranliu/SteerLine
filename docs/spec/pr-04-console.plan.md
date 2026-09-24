# PR 4 Plan：执行控制台 TUI

依据：[Spec](pr-04-console.spec.md)。依赖：PR 1–3。

## 实施步骤

1. **整理 UI 依赖。** 将现有 viewer 主模型迁入 `internal/steerline/console`，拆成观察接口和控制接口；前者读 DTO，后者返回 request_id/回执。保留小型 viewer 兼容包装或直接调整 CLI 路由，避免复制两套 UI。
2. **加入多项目模型。** 复用 `ListProjects`，维护项目独立的选择、筛选、错误、日志 cursor 和在途请求。所有 tea.Msg 带项目身份与 generation。读取配置错误成为项目行状态，不阻断其他项目。
3. **实现主页面。** 顶部服务/模式/新鲜度，项目导航，ready/runs 筛选列表，详情与 activity/attempt 切页。按稳定 ID 保存选择，窄屏改用单面板切页。
4. **接入服务生命周期。** 把 PR 3 连接/停止结果和 Windows“需要服务窗口”入口变成明确 UI 状态；窗口初始化期间显示日志入口和 startup_id；退出只结束 tea.Program，不创建或关闭服务窗口。
5. **接入任务与派发动作。** 用 PR 2 client 提交 start/mode/pause/resume/stop。操作键和文本输入焦点分离；跟踪同一 request_id，独立呈现请求状态与 run 状态。服务条件变化时刷新并显示拒绝原因。
6. **保留决策能力。** 现有弹窗改用重新读取 + 指定 decision_id 的回答入口，成功文案改成答案已保存。新决策页与并发冲突不在本 PR 重复实现。
7. **修改入口与说明。** `main.go` 增 console 子命令及配置选择逻辑；修改嵌入式 launchers、仓库根 viewer.bat、projects 菜单和 launcher README，加入 console.bat；调整 launcher 测试和用户 README。

## 验证

- 用 fake QuerySource/ControlClient 验证页面状态和用户意图：Enter 不启动、正确 task_id、双击只产生一次请求、迟到回执隔离、stale revision 不自动重试。
- 使用 fake clock 驱动前台/后台轮询，确保慢请求不重叠，退出清理轮询。
- 用 PR 2 的假服务/Beads/worker 完成 UI 模型端到端测试，不仅断言视图字符串，还检查实际请求内容和 Claims/Starts 计数。
- 真实 Windows TTY 冒烟覆盖多项目、固定入口提示、用户独立打开服务窗口、选择执行、暂停、退出重开、停止；使用测试项目和协议替身，避免消耗真实任务。
- 查看 80×24 与宽屏，验证中文、长问题、长路径、错误换行和焦点；记录交互结果，不能仅凭静态字符串单测确认可用。
- 运行 console/viewer、projects/launchers、watch 与 CLI 路由相关测试；无需再次做没有受影响的全套宿主审计。

## 完成条件

- U-A1～U-A9 全部满足，控制台可引导打开服务窗口，并完成日常“连接服务 → 指定任务执行 → 状态查看 → 停服”。
- UI 不直接写 runtime、不通过 shell 拼接任务 ID，也不引入第二套业务守卫。
- 原 watch 的流式接口与 viewer/project 批处理别名有回归验证。

## 风险与回退

多项目和多种异步消息会放大对象错配风险，先实现消息身份，再接动作。入口可临时回退到旧 viewer，但新服务的 v4 状态仍需新版读取器；不能回退到不认识 v4 的二进制。
