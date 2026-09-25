# SteerLine

SteerLine 是一款面向工程 AGI 的研究型软件开发工具。它以 roadmap 组织规划、任务依赖与 AI 执行，让人掌握产品目标、关键决策和 PR 合并。

长期目标是让 AI 能自主完成普通人需要长期借助 AI 才能开发的复杂软件项目。项目目前尚未发布。

[产品愿景](docs/roadmap/product-vision.md) · [当前 Roadmap](docs/roadmap/roadmap.md)

## 执行控制台

运行 `steerline console` 打开控制台。默认读取 `<STEERLINE_HOME>/etc/*.json` 中的项目配置；使用 `steerline console -config <path>` 打开一份显式配置，也可用 `-run-id <run-id|task-id>` 定位当前项目中已有的运行。Windows 安装目录提供 `console.bat`；`steerline projects` 菜单也可进入同一控制台。

控制台通过服务查看任务候选、运行和回执。只有选择 Ready 候选并按 `s` 才会提交开始请求；按 Enter 只打开详情。`m` 切换手动/自动派发，`p` 暂停/恢复派发，`x` 在确认后停止服务。Windows 服务仍需用户从固定的 `service-window.bat` 独立打开；退出控制台不会停止服务。待决策仅展示问题、选项和 decision_id，本阶段不从控制台写入答案。

项目、所选 run/attempt、活动日志、控制请求以及折叠状态按各自身份隔离。活动会话只有明确的 turn 终态才自动收起过程消息；`final_answer` 正文单独分页阅读，过程完成、执行结束和业务任务完成分别展示。来源没有确认轮次或 final 时，控制台会显示能力原因和未识别状态。

键盘：`↑/↓` 选择或滚动，`←/→` 切项目，`d/a/c/f` 切详情/活动/会话/正文，`[`/`]` 切 attempt，`,/.` 选择轮次，`e` 折叠/展开已验证轮次，`=` 保持当前轮展开，`o` 阅读过程正文，`n` 读取下一条 final，`y` 读取下一页会话记录，`i` 查询原 request_id，`q` 只退出控制台。窄屏用 `t/d/a/c/f` 切换单面板。

[会话事件与完整正文](docs/conversation-events.md) · [Activity 读取契约](docs/activity.md)
