# SteerLine

SteerLine 是一款面向工程 AGI 的研究型软件开发工具。它以 roadmap 组织规划、任务依赖与 AI 执行，让人掌握产品目标、关键决策和 PR 合并。

长期目标是让 AI 能自主完成普通人需要长期借助 AI 才能开发的复杂软件项目。项目目前尚未发布。

[产品愿景](docs/roadmap/product-vision.md) · [当前 Roadmap](docs/roadmap/roadmap.md)

## 执行控制台

运行 `steerline console` 打开控制台。默认读取 `<STEERLINE_HOME>/etc/*.json` 中的项目配置；使用 `steerline console -config <path>` 打开一份显式配置，也可用 `-run-id <run-id|task-id>` 定位当前项目中已有的运行。Windows 安装目录提供 `console.bat`；`steerline projects` 菜单也可进入同一控制台。安装目录只保留 `console.bat` 和 `service-window.bat` 两个批处理入口；后者默认使用 `etc\SteerLine.json`，其他 native 项目可传配置路径。

控制台通过服务查看中心 native 任务候选、运行和回执。选择 Ready 候选按 `s` 会打开已配置执行方案列表，选定后按 Enter 才提交开始请求；在普通详情页按 Enter 只打开详情。`m` 切换手动/自动派发，`p` 暂停/恢复派发，`x` 在确认后停止服务。Windows 服务仍需用户从固定的 `service-window.bat` 独立打开；退出控制台不会停止服务。待决策问题显示完整 question/context/选项详情和 decision_id；按 `a` 后，只有当前 native WorkflowRun、Execution 和 attempt 身份有效时才打开 Unicode 答案编辑器。选项只是预选，按 `F2` 才提交；答案被服务接受不等于已应用，应用须由原执行的绑定证明。服务核对原 workspace 和等待中的步骤后，会以新 attempt 继续同一 native workflow。`u` 打开只读恢复核验面板；当前格式的恢复命令会按原 attempt、Operation 和外部身份重新核对，不确定时保留 blocker 与现场。

手动模式下，可以继续选择其他 Ready 任务按 `s` 启动；SteerLine 不设置人工并行数量上限，各任务分别冻结执行配置与 worker 身份。同一任务不能重复领取，DAG 前置仍须满足。自动模式保持一次一个活动执行；切换到自动模式不会停止已人工启动的任务。

保留旧执行的 Beads 项目在控制台中以只读视图显示。任务和硬前置从原 Beads 库查询，旧运行投影只供诊断；这类项目不能在新版控制台按 `s/m/p/x` 控制。旧服务结算完成后再切换到 native DAG。

native 项目控制台还会显示中心任务关系、阻塞原因、Spec 路径和交付定位；离线详情显示采样时间，Ready 缓存不会变成启动许可。完整固定引用保留在中心任务契约中。命令行可用 `steerline task graph -config <path> -task-id <id> -direction upstream -depth 3` 查询关系，或用 `steerline task export -config <path> -format json -output <path>` 导出；覆盖已有导出文件需显式传 `-overwrite`。

控制台首屏仅显示当前项目、服务与派发状态、任务列表、阻塞和可用操作。任务详情显示 Spec 路径，不展开 Plan、文档 SHA 或提交号；这些固定引用仍在中心任务契约中。按 `?` 查看完整快捷键。项目左右切换时，标题始终显示当前项目编号和名称，后台项目的提示不会覆盖当前项目。

选中 Ready 任务时，详情显示服务实际加载的执行位置、Agent、Model、Thinking effort 和 Sandbox；Orca 远程还显示固定环境与连接。项目没有显式指定模型时标明由 Codex 配置决定。控制台与服务的配置版本不一致时，界面不猜测实际模型或后端，并阻止启动与自动派发。

本机 Orca 配置在 Orca worktree 的终端中运行无交互的 `codex exec --json`。Orca 仍会在对应 worktree 下记录 Codex agent session；它不是可继续输入的 Codex TUI。若 Orca 将该 worktree 放入项目的“隐藏的已发现工作树”，先展开该分组并选择 worktree；查看右侧会话历史时也要选对本机主机和工作区范围。控制台运行详情显示原 Orca 终端 handle，可用 `orca terminal switch --terminal <handle>` 打开终端。终端暂时没有输出只记需关注提示，服务继续核对原终端与固定步骤结果；不能据此认定执行结束或重派任务。

可在同一项目内预设多套[执行配置](docs/execution-profiles.md)。按 `s` 选择一套并确认；不同任务可以混用，本次任务的全部步骤固定在所选配置上。附加配置存在时，自动派发需要显式指定默认配置。

项目、所选 run/attempt、活动日志、控制请求以及折叠状态按各自身份隔离。活动会话只有明确的 turn 终态才自动收起过程消息；`final_answer` 正文单独分页阅读，过程完成、执行结束和业务任务完成分别展示。来源没有确认轮次或 final 时，控制台会显示能力原因和未识别状态。

键盘：`↑/↓` 选择或滚动，`←/→` 切项目，`d/a/c/f` 切详情/活动/会话/正文，`v` 看决策历史，`u` 看恢复核验，`[`/`]` 切 attempt，`,/.` 选择轮次，`e` 折叠/展开已验证轮次，`=` 保持当前轮展开，`o` 阅读过程正文，`n` 读取下一条 final，`y` 读取下一页会话记录，`i` 查询原 request_id，`q` 只退出控制台。窄屏用 `t/d/a/c/f` 切换单面板。

[会话事件与完整正文](docs/conversation-events.md) · [Activity 读取契约](docs/activity.md)
