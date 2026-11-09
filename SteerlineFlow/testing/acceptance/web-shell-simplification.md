# Web 外壳与项目切换精简

日期：2026-11-09。普通聊天开发，用户已确认方案并授权实施、创建 PR；不进入 roadmap 调度，未部署安装现场。

## 范围与通过条件

| ID | 方法 | 通过条件 | 当前状态 |
| --- | --- | --- | --- |
| WS-1 | 隔离浏览器 | 仅右上角保留服务连接标记；删除侧栏在线标记、固定 LOCAL SERVICE、搜索框、重复项目卡片与刷新目录按钮。 | 已验收 |
| WS-2 | 隔离浏览器 | 项目下拉框位于侧栏配置下方；普通项目仅显示名称，同名项目附路径；切换保留当前页面类型。 | 已验收 |
| WS-3 | 隔离浏览器、截图走查 | 1440/800/375px 下拉切换可用、无横向溢出；收起侧栏后仍可用键盘切换；顶部保留当前项目名。 | 已验收 |
| WS-4 | 隔离浏览器 | 连接中断时唯一连接标记转为待核实并保留原因；启动失败诊断可见；空目录仍显示选择入口和说明。 | 已验收 |
| WS-R | L1/L2/web.unit、构建 | 固定回归集、前端全集与生产构建通过；L2 清理完整。 | 已验收 |

## 源码、环境与证据

实现提交：`a742901f0f5702b138e8831272851abc4f790e9f`。Windows amd64、Go 1.26.4、Node 24.13.0，浏览器为本机无头 Edge。

最终固定测试集在干净实现提交执行，原始报告位于以下独占目录，每个目录以 `summary.json` 为入口：

```powershell
go run ./cmd/test-feedback -level=L1 -out C:\SteerlineDebug\wt-8bef-web-shell\test-results\L1\20261109-final
go run ./cmd/test-feedback -level=L2 -out C:\SteerlineDebug\wt-8bef-web-shell\test-results\L2\20261109-final
go run ./cmd/test-feedback -level=L4 -suite=web -scenario web.unit -out C:\SteerlineDebug\wt-8bef-web-shell\test-results\L4\20261109-final
npm --prefix web run build
```

L1 通过（37.9 秒）；web.unit 380/380 通过、无跳过。L2 通过，`cleanup_complete=true`。生产构建通过，原日志为 `C:\SteerlineDebug\wt-8bef-web-shell\diagnostics\build-final.log`。

浏览器使用真实生产客户端和拦截 API 的只读夹具，不接入安装现场。临时检查脚本已保存为 `C:\SteerlineDebug\wt-8bef-web-shell\diagnostics\browser-check.mjs`，从本源码根执行 `node <该绝对路径>`。结果及四张实际截图位于 `C:\SteerlineDebug\wt-8bef-web-shell\diagnostics\20261109-browser-final2\`：`result.json`、`shell-1440.png`、`shell-800.png`、`shell-375.png`、`shell-collapsed.png`；8 项检查通过、无浏览器异常，桌面与窄屏截图已走查。

第一次浏览器检查缺少 Roadmap preparation 夹具，补齐后通过；扩大启动失败检查后发现诊断仅在下拉选项内显示，已将项目诊断接入原连接告警并复验通过。早期通过结果保留在同 scope 的 `20261109-first` / `20261109-smoke` / `20261109-web-unit` 和 `20261109-browser-fixture2` 目录，不替代最终报告。

## 边界

本次仅整理界面，项目目录继续沿原查询轮询更新，连接判断和操作保护沿原实现。未操作真实项目、安装或部署；真实目录和长时间运行体验未安排，不作为本次自动 UI 验收已通过的范围。
