# SteerLine

SteerLine 是一款面向工程 AGI 的研究型软件开发工具。它以 roadmap 组织规划、任务依赖与 AI 执行，让人掌握产品目标、关键决策和 PR 合并。

长期目标是让 AI 能自主完成普通人需要长期借助 AI 才能开发的复杂软件项目。项目目前尚未发布。第一阶段只支持和验收本地 Codex 与本机 Orca；远程执行及本机/远端混合流程后续另行规划，不作为当前交付门槛。

[产品愿景](docs/roadmap/product-vision.md) · [当前 Roadmap](docs/roadmap/roadmap.md) · [当前代码架构图](docs/architecture/current-code-architecture.md) · [测试总入口](docs/testing.md)

## 测试

日常最小入口为 `go run ./cmd/test-feedback -level=L1`。Full、定向回归、L2/L3、测试环境与独立原生验收统一见[测试指南](docs/testing.md)；其中完整 L2 和 L3 live 会消耗模型 token，完整 L2 会创建并合并真实测试 PR。

## 构建

在 Windows 上，双击仓库根目录的 `build.bat` 即可构建；窗口会显示结果并等待按键关闭。命令行可运行 `./build.ps1`，或调用 `build.bat --no-pause`。脚本从仓库根目录构建 `cmd/steerline`，固定输出到 `bin/steerline.exe`；构建失败时保留原文件，目标程序仍在运行时拒绝替换。此命令只生成本地二进制，不安装到 `STEERLINE_HOME`，也不切换运行中的服务。正式的 stable/dev 固定版本发布与选择由[固定版本 Spec](docs/spec/versioned-runtime.spec.md)定义，目前尚未交付。

## 执行控制台

运行 `steerline console` 打开控制台。默认读取 `<STEERLINE_HOME>/etc/*.json` 中的项目配置，并为所有项目读取 `<STEERLINE_HOME>/execution.json` 全局执行配置；使用 `steerline console -config <path>` 打开一份显式项目配置，也可用 `-run-id <run-id|task-id>` 定位当前项目中已有的运行。Windows 安装目录提供 `console.bat`；`steerline projects` 菜单也可进入同一控制台。安装目录只保留 `console.bat` 和 `service-window.bat` 两个批处理入口；后者默认使用 `etc\SteerLine.json`，其他 native 项目可传配置路径。

控制台通过服务查看中心 native 任务候选、运行和回执。选择 Ready 候选按 `s` 会打开已配置执行方案列表，选定后按 Enter 才提交开始请求；在普通视图按 Enter 只打开完整只读详情。`m` 切换手动/自动派发，`x` 在确认后停止服务。Windows 服务仍需用户从固定的 `service-window.bat` 独立打开；退出控制台不会停止服务。待决策问题显示完整 question/context/选项详情和 decision_id；按 `a` 打开 Unicode 答案编辑器。选项只是预选，按 `F2` 才提交。中心分别显示答案保存、送达回执与原会话新轮次开始，不要求 agent 回传“已应用”证明。`u` 打开只读执行核验面板。

手动模式下，可以继续选择其他 Ready 任务按 `s` 启动；SteerLine 不设置人工并行数量上限，各任务分别冻结执行配置与 worker 身份。同一任务不能重复领取，DAG 前置仍须满足。自动模式保持一次一个活动执行；切换到自动模式不会停止已人工启动的任务。

当前生产配置使用 `worker-flow-v1` 和中心 native Task。旧配置或运行状态在外部副作用前拒绝；需要人工核实旧现场，不自动迁移或接管原 worker。

业务状态只持久化在 SQLite 的 Task、WorkflowRun、Execution、Attempt、Decision、Operation 和交付记录中。服务 checkpoint 只保存服务实例、心跳、未完成控制操作及认领前的预留；Runtime/Run 是在同一读取事务中生成的视图，不参与业务写回。工作区准备意图、轮次观察及当前决策绑定由 Execution 保存；回答事务直接校验 WorkflowRun 的持久控制版本，心跳和日志更新不会改变该版本。当前数据库 schema 为 13，runtime/checkpoint schema 为 18，旧格式直接拒绝，不自动迁移。

native 项目控制台还会显示中心任务关系、阻塞原因、Spec 路径和交付定位；离线详情显示采样时间，Ready 缓存不会变成启动许可。完整固定引用保留在中心任务契约中。命令行可用 `steerline task graph -config <path> -task-id <id> -direction upstream -depth 3` 查询关系，或用 `steerline task export -config <path> -format json -output <path>` 导出；覆盖已有导出文件需显式传 `-overwrite`。

控制台宽屏首屏左侧显示任务列表，按任务编号、任务状态、执行状态、任务名称四列排列；列表获得更多宽度，右侧上方显示只读任务摘要、下方显示任务动态。未进入可开始候选的 open 任务显示“阻塞”；已完成任务的旧失败执行标为“失败（历史）”，仍有未核实执行的任务保留“需处理”。选中行以 `▸` 标记。`Tab` 仅在列表与动态之间切换焦点，摘要随选中任务更新，运行中的任务显示已固定执行方案的位置、agent、模型和推理强度；`d` 或 Enter 可阅读包含状态和运行 ID 的完整详情。窄屏在列表与动态之间切换。任务详情显示 Spec 路径，不展开 Plan、文档 SHA 或提交号；这些固定引用仍在中心任务契约中。按 `?` 查看完整快捷键。项目左右切换时，标题始终显示当前项目编号和名称，后台项目的提示不会覆盖当前项目。

选中 Ready 任务按 `s` 后，方案选择页显示执行位置、Agent、Model、Thinking effort 和 Sandbox。全局方案没有显式指定模型时标明由 Codex 配置决定。控制台与服务的配置版本不一致时，阻止启动与自动派发。

本机 local Codex 使用固定原生 session ID 续聊。Orca 本机方案也可从全局执行配置中选择并固定 Codex 模型与推理强度，派发前须通过 Orca 连接和仓库注册检查。存量 `orca_remote_worker` 协议/适配代码与旧试验资料保留，但远端流程不在当前一期支持与验收范围。配置见[执行方案](docs/execution-profiles.md)。

可在全局文件预设多套[执行配置](docs/execution-profiles.md)，供所有项目选用。按 `s` 选择一套并确认；不同任务可以混用，本次任务的 worker 会话固定在所选配置上。多套方案存在时，自动派发需要显式指定默认配置。

项目、所选 run/attempt、活动日志、控制请求以及折叠状态按各自身份隔离。活动会话只有明确的 turn 终态才自动收起过程消息；`final_answer` 正文单独分页阅读，过程完成、执行结束和业务任务完成分别展示。来源没有确认轮次或 final 时，控制台会显示能力原因和未识别状态。

键盘：`Tab` 在任务列表与动态之间切换焦点，`↑/↓` 选择任务或滚动动态，`←/→` 切项目，`d`/Enter 打开完整只读详情、Esc 返回，`A/c/f` 切活动/会话/最终回复；`a` 在有待决策问题时回答，否则切到活动。`v` 看决策历史，`u` 看执行核验，`,/.` 选择轮次，`e` 折叠/展开已验证轮次，`=` 保持当前轮展开，`o` 阅读过程正文，`n` 读取下一条 final，`y` 读取下一页会话记录，`q` 只退出控制台。

[会话事件与完整正文](docs/conversation-events.md) · [Activity 读取契约](docs/activity.md)

Windows Orca Codex 受管方案还要求在 `codex.worker-profile` 配置载荷中显式设置 `windows_sandbox`，先在原会话核验 readiness 探针与原生结算，再派发业务。宿主等待通过中心待决策显示来源及可信程度，提供打开原 terminal、重新检查动作。此实现已通过审计定向回归及隔离真机闭环，详见[环境验收记录](docs/validation/worker-environment-readiness/README.md)。
