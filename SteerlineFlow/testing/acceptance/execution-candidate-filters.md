# 执行页多候选状态筛选

2026-10-26，普通前端开发。基线 `6e337edda39ff72bbb3a01cecd1f63022c20ed07` 加本工作区未提交修改；不修改任务业务状态或后端执行控制。

## 行为与覆盖

按候选的运行、问题、待合并及需处理事实分别筛选，任务去重聚合。任务卡保留任务业务状态，增加候选状态分布和命中数；工具栏显示任务数 / 候选数。当前页面仅加载前 100 个任务时明确提示统计范围。

选择筛选时优先定位当前任务的命中候选，否则定位首个命中任务；点击任务按候选编号稳定选择命中项。候选栏保留全部可见候选并标出命中项。刷新不根据筛选强制切换候选；不再命中时提示并保留当前内容。初始选择和显式深链均固定候选身份。

| 验收项 | 自动覆盖与结果 |
| --- | --- |
| ECF-A01：同任务运行 / 待回答 / 待合并混合状态，跨任务去重与候选分别计数 | 通过，Runs 组件夹具覆盖两个任务、六个独立候选 |
| ECF-A02：筛选切换及任务点击定位命中候选，多个命中按编号选择，保留兄弟候选 | 通过，反序快照验证稳定定位，并可主动查看未命中的候选 |
| ECF-A03：异步非阻塞问题与运行事实并存，显示可稍后回答；合并前可选反馈不误计待回答 | 通过，状态规则及组件交互覆盖 |
| ECF-A04：刷新保留选择及回答草稿，状态退出筛选后保持当前候选，深链不被默认项覆盖 | 通过，组件快照更新及回答编辑交互覆盖 |
| ECF-A05：失败、受阻、环境阻塞及恢复要求纳入需处理；结束 / 停止中的候选与终态任务不计当前待办 | 通过，状态规则及可见历史反例覆盖；原执行事实继续保留 |
| ECF-A06：未确认启动请求不计运行中，旧实例请求不变成新候选或占用名额 | 通过，启动待核实夹具与既有 candidate-starts 回归 |

新增 16 项状态 / 组件测试；四个相关文件合计 38 项通过。原候选停止、迟到回执、启动占位和任务依赖阻塞测试继续保留。

## 实际执行与证据

统一外部 scope 为 `C:\SteerlineDebug\wt-39fe-candidate-filters\test-results\`，下列相对路径均在该目录中。每轮使用全新目录；这些结果属于当前脏工作区验证，不导出为干净提交的 Git 测试历史。

| 命令 | 结果与证据 |
| --- | --- |
| `go run ./cmd/test-feedback -level=L1 -out <本轮目录>` | 通过；`L1/20261026T045507300Z-1003a6dcf9a349e8a78a2211144df6d4/summary.json` |
| `go run ./cmd/test-feedback -level=L2 -out <本轮目录>` | 最终源码受控交付冒烟通过；`L2/20261026T045527049Z-8c6b0fdc025a47db8336a7d4cefa094a/summary.json` |
| `./scripts/test-web.ps1 -EvidenceDir <本轮目录>` | 构建、30 文件 / 216 项 Vitest、HTTP owner 均通过；`web/20261026T045419856Z-b8d466ceb9254eb7abdd40e57b3390cb/` 中的 `build.log`、`client-tests.log`、`server-tests.log` |
| `npm test -- src/features/runs/state.test.ts src/features/runs/runs.test.tsx src/features/runs/candidate-starts.test.tsx src/features/runs/run-task.test.tsx --maxWorkers=1`（web 目录） | 补强运行中筛选断言后，4 文件 / 38 项通过；`diagnostics/20261026T045537854Z-0678b95f3623495cbd4638632781a3e0/client-tests.log` |

Web 包装器中的 HTTP 测试由 test-feedback 自建报告，实际入口为 `C:\Users\lmas1\AppData\Local\Temp\steerline-test-feedback\20261026T045439.567914300Z\summary.json`，不是外层 Web 日志目录。

`python scripts/check_doc_links.py` 检查 582 个 Markdown 文件、3640 个本地导航链接，0 错误；`git diff --check` 通过。

初次执行缺少工作区 Web 依赖，在 `diagnostics/20261026T045012073Z-76d1dba1a50548a7a3982f13a08001e5/client-tests.log` 留存启动失败；随后 `npm ci --no-audit --no-fund` 按锁文件安装。首轮定向与首轮全 Web 检查分别暴露既有启动测试对旧工具栏文案的断言，更新为同时核对任务卡候选数及工具栏任务 / 候选计数后通过；原失败日志分别保留于 `diagnostics/20261026T045141962Z-6dfc0f72544244c5a7e6f684072c5413/` 和 `web/20261026T045327633Z-0ff330f431684d0ea056a68387be02e3/`。

未运行真实浏览器视觉验收、真实模型或真实 PR 流程；未部署安装环境。Web 的交互结论来自 jsdom 组件测试，L2 仅代表受控交付冒烟。
