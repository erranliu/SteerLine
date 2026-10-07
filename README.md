# SteerLine

SteerLine 是一款面向工程 AGI 的研究型软件开发工具。它以 roadmap 组织规划、任务依赖与 AI 执行，让人掌握产品目标、关键决策和 PR 合并。

长期目标是让 AI 能自主完成普通人需要长期借助 AI 才能开发的复杂软件项目。项目目前尚未发布。第一阶段只支持和验收本地 Codex 与本机 Orca；远程执行及本机/远端混合流程后续另行规划，不作为当前交付门槛。

[文档入口](docs/README.md) · [产品愿景](docs/roadmap/product-vision.md) · [当前 Roadmap](docs/roadmap/roadmap.md) · [当前代码架构图](docs/architecture/current-code-architecture.md) · [测试总入口](docs/testing.md)

## 测试

日常最小入口为 `go run ./cmd/test-feedback -level=L1`。Full、定向回归、L2/L3、测试环境与独立原生验收统一见[测试指南](docs/testing.md)；其中完整 L2 和 L3 live 会消耗模型 token，完整 L2 会创建并合并真实测试 PR。

## 构建

在 Windows 上，双击仓库根目录的 `build.bat` 即可构建；窗口会显示结果并等待按键关闭。命令行可运行 `./build.ps1`，或调用 `build.bat --no-pause`。脚本从仓库根目录构建 `cmd/steerline`，固定输出到 `bin/steerline.exe`；构建失败时保留原文件，目标程序仍在运行时拒绝替换。此命令只生成本地二进制，不安装到 `STEERLINE_HOME`，也不切换运行中的服务。安装与源码调试共存的近期范围见[当前 Roadmap](docs/roadmap/roadmap.md)；完整 stable/dev 版本管理已移出当前规划。

## 执行控制台

运行 `steerline console` 打开控制台。默认读取 `<STEERLINE_HOME>/etc/*.json` 中的项目配置，并为所有项目读取 `<STEERLINE_HOME>/execution.json` 全局执行配置；使用 `steerline console -config <path>` 打开一份显式项目配置，也可用 `-run-id <run-id|task-id>` 定位当前项目中已有的运行。Windows 安装目录提供 `console.bat`；`steerline projects` 菜单也可进入同一控制台。安装目录只保留 `console.bat` 和 `service-window.bat` 两个批处理入口；后者默认使用 `etc\SteerLine.json`，其他 native 项目可传配置路径。

控制台通过服务查看中心 native 任务候选、运行和回执。普通 Enter 转到右下运行与反馈，有当前问题时原地激活回答；d 才进入完整只读详情。Ready 任务按 s、进行中的任务按 b，在右下选择已配置方案并按 Enter 明确开始；z 在右下确认放弃候选，worker 停止独立核实。m 切换派发模式，x 明确确认后停服。Windows 服务仍需用户从 service-window.bat 独立打开，退出控制台不会停止服务。答案选项只是预选，F2 是唯一提交键；保存、送达及原会话新轮次分别显示。u 在右下显示只读核验摘要，r 重读报告，F8 按需阅读完整报告。

手动模式下，可以继续选择其他 Ready 任务按 `s` 启动；SteerLine 不设置人工并行数量上限，各任务分别冻结执行配置与 worker 身份。同一任务不能重复领取，DAG 前置仍须满足。自动模式保持一次一个活动执行；切换到自动模式不会停止已人工启动的任务。

当前生产配置使用 `worker-flow-v1` 和中心 native Task。旧配置或运行状态在外部副作用前拒绝；需要人工核实旧现场，不自动迁移或接管原 worker。

业务状态只持久化在 SQLite 的 Task、WorkflowRun、Execution、Attempt、Decision、Operation 和交付记录中。服务 checkpoint 只保存服务实例、心跳、未完成控制操作及认领前的预留；Runtime/Run 是在同一读取事务中生成的视图，不参与业务写回。工作区准备意图、轮次观察及当前决策绑定由 Execution 保存；回答事务直接校验 WorkflowRun 的持久控制版本，心跳和日志更新不会改变该版本。持久格式及源码定位统一见[数据模型](docs/product-design/data-model.md#当前格式来源)；旧格式直接拒绝，不自动迁移。

native 项目控制台还会显示中心任务关系、阻塞原因、Spec 路径和交付定位；离线详情显示采样时间，Ready 缓存不会变成启动许可。完整固定引用保留在中心任务契约中。命令行可用 `steerline task graph -config <path> -task-id <id> -direction upstream -depth 3` 查询关系，或用 `steerline task export -config <path> -format json -output <path>` 导出；覆盖已有导出文件需显式传 `-overwrite`。

控制台宽屏保留三窗：左侧每 Task 一条主行，分别显示任务/执行状态及候选汇总待处理提示；右上显示所选候选、冻结方案、状态、阻断/PR和下一步；右下承接方案选择、确认、运行动态、Unicode 回答、核验与结果预览。当前焦点用双线边框、▶ 标记、选项高亮与输入光标表示，失焦列表仍保留选中行。状态使用语义颜色和文字，遵守终端颜色能力及 NO_COLOR。新问题和迟到回执不抢焦点。Tab/Shift+Tab 在列表、选项与输入间循环，输入 Enter 换行，F2 提交。切任务/候选先保存原草稿，并取消临时选择或确认；F6/F7 选择稳定编号的当前/历史候选。

Ready 任务按 s 后，在右下比较执行位置、Agent、Model、推理强度和权限等产品字段；F8 阅读完整方案与身份，返回仍是原选择。全局方案未固定模型时按原事实显示默认来源。原 Task/图/服务版本或所选方案摘要变化时，拒绝旧确认，需重新选择；配置不一致仍阻止新启动与自动派发。

本机 local Codex 使用固定原生 session ID 续聊。Orca 本机方案也可从全局执行配置中选择并固定 Codex 模型与推理强度，派发前须通过 Orca 连接和仓库注册检查。存量 `orca_remote_worker` 协议/适配代码与旧试验资料保留，但远端流程不在当前一期支持与验收范围。配置见[执行方案](docs/product-design/execution-profiles.md)。

可在全局文件预设多套[执行配置](docs/product-design/execution-profiles.md)，供所有项目选用。按 `s` 选择一套并确认；不同任务可以混用，本次任务的 worker 会话固定在所选配置上。多套方案存在时，自动派发需要显式指定默认配置。

控制台只显示最近最多 10 条规范活动，不提供工具正文、完整过程时间线、折叠或跨重启阅读位置。主界面从已采集且身份/阶段明确的 final 提供最多 800 字符、6 行的有界预览；通用 Summary、最后消息和 worker 自报测试不作为独立验收通过证据。f 按需读取完整最终回复并校验正文身份/版本/摘要，[ / ] 只在该阅读区选择历史回复。新轮次运行动态不会被上一轮 final 或旧终态回执遮住。准备轮、工具过程、未知阶段及采集缺口如实显示，不从原始日志回填；详细过程由用户手动查看可核实的原生定位。

键盘：Tab/Shift+Tab 列表/选项/输入（未编辑时列表/运行反馈），↑↓ 或 PgUp/PgDn 选择/滚动，←→ 切项目，Enter 到右下或回答，d 详情，F6/F7 候选，s 开始首候选方案选择，b 新增，z 放弃确认。f/v 按需阅读最终回复/历史决策；u 右下核验，核验焦点 r 重读报告、列表焦点 r 刷新项目，F8 完整报告。回答区 Enter 换行或从选项转输入，F2 显式提交，F8 完整背景。详情/F8 不处理主界面的操作键；Esc 返回原上下文。窄屏列表焦点 F2 先返回原草稿预览，再按 F2 才能提交；极小窗口缺少核对信息时禁止确认开始、放弃、停服和答案提交。? 查看帮助，q 只退出控制台。

UI 操作记录采用格式 2，存于 `<STEERLINE_HOME>/ui/views/<window>/view.json`，仅保存项目绑定、各候选/Decision 答案草稿及原请求对账记录。重开重置阅读布局，只查询未知请求，不自动重发。旧 `view.v1.json` 或损坏/不支持格式保留现场并阻止该窗口新外发，允许只读查看；须人工核实原请求后处理退役文件，不自动迁移或覆盖。多窗口继续独立锁定各自记录。

[会话事件与完整正文](docs/product-design/conversation-events.md) · [Activity 读取契约](docs/product-design/activity.md)

Windows Orca Codex 受管方案还要求在 `codex.worker-profile` 配置载荷中显式设置 `windows_sandbox`，先在原会话核验 readiness 探针与原生结算，再派发业务。宿主等待通过中心待决策显示来源及可信程度，提供打开原 terminal、重新检查动作。此实现已通过审计定向回归及隔离真机闭环，详见[环境验收记录](docs/validation/worker-environment-readiness/README.md)。
