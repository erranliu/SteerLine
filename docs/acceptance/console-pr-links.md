# 控制台 PR 链接

> 2026-10-15 TUI 退役：本文保留原交付/验收事实，TUI 专属操作和未验项不再适用，不记为通过。共享后台/API 要求继续归原记录；当前范围见 [TUI 退役](../spec/tui-retirement.spec.md)。

本聊天直接授权：候选与执行中的 PR 编号可跳转，不新增快捷键；摘要、详情和历史 PR 地址采用同一终端超链接渲染。

2026-10-09，`go run ./cmd/test-feedback -- ./internal/console -count=1` 通过，9.758 秒。报告：`C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T061323.088331000Z/summary.json`。覆盖窄窗口截断时保持完整目标及关闭链接边界、PR 更新/缺失、非法协议和源文本控制字符；`git diff --check` 通过。源码为 `a3701e0a6654899ce8436b732aa14b6133033a24` 加本次未提交修改。

运行文件：`C:/SteerlineRuntime/bin/steerline-console-pr-links-20261009.exe`，SHA-256 `d82faa4ba7142f31c19ae2af62f2860f8e91f15c9d18875d726c032614244aba`，source_dirty=true。`C:/SteerlineRuntime/console.bat` 已指向此文件，旧入口已备份；已打开的控制台需要重开加载。

实际鼠标跳转未人工验收；链接使用 OSC 8，由支持该协议的终端处理点击，不由 SteerLine 添加键盘动作。

PR 发布前已 rebase 到 main 26f98994b1f5234a61270fe0de53e0139c530294，在源码 commit 5784e4df755a29cf516e746a5dcd48f2ee5ae783 上再次执行同一控制台定向命令，通过，4.168 秒。报告：C:/Users/lmas1/AppData/Local/Temp/steerline-test-feedback/20261009T062343.786789700Z/summary.json。随后仅补充本验收记录。
