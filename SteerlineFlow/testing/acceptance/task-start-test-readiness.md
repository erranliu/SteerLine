# 任务启动与测试安排就绪衔接

2026-11-05，用户授权的普通修复。源码基线 `df18050e87a2ef7852c71224063480407527c6c8` 加本工作树差异；没有部署安装目录、重启原服务或重新启动真实任务。

## 问题与修改

安装项目 SteerlineTest 的 #101 在测试安排生成请求为 `accepted` 时允许用户点击启动，后端按现有材料门禁实际拒绝；生成完成后，执行页继续把旧拒绝原因作为主要提示。只读核对原生成 Run `22e718d87d8c6050f5557fedc34db0da`，北京时间 15:13:26 创建、15:14:14 完成，结果 succeeded、资源 stopped；核对时 #101–#105 材料均为 ready。这份现场证据仅用于定位，不计为修复版本的验收。

- 首次手动启动和新增并行候选均读取当前测试安排；只有查询成功且状态为 ready 才开放入口和确认按钮。已打开的确认面板遇到重新准备或查询失败时同样阻断。环境门禁的开发者开关不关闭测试安排门禁。
- 页面直接显示读取中、等待、排队、生成中、校验中、失败、待核实或已就绪。在线可见页面每两秒观察，按应用 home、项目、服务实例、任务隔离缓存。
- 在既有 `/maintenance/tasks/<ID>` 下增加严格的 `?view=status` 视图，仅返回 commit/status/diagnostic，不执行 Git 或读取完整文档；无查询参数仍按原路径按需读取完整文档。两种视图共用远端观察新鲜度判断，远端变化或观察失败时不把旧材料报为 ready。
- 已拒绝启动继续保留原请求及诊断，卡片标注“历史”，详情折叠展示“上次启动结果”。当前准备状态独立更新；就绪后可发起新请求，不改写原回执、不自动重试旧启动。
- 后端原启动校验保持，处理用户点击与材料变化之间的竞态。未修改固定提示词、执行方案或持久化结构。

## 回归与测试映射

沿用现有测试登记，不增加级别、场景或执行入口。

| 范围 | 断言 |
| --- | --- |
| `web.unit` 中任务详情 | accepted 阻断→轮询 ready 放行；确认面板打开后 queued 再阻断→ready 恢复；首次/并行启动均覆盖；生成失败、材料无效、取消、未知及未识别状态不放行；刷新失败即使缓存 ready 仍禁用；环境开发开关关闭时仍要求测试安排 ready。 |
| `web.unit` 中候选启动 | 真实 useCandidateStarts hook 的拒绝回执保留为历史；当前 accepted/ready 独立显示；旧诊断可以展开；不自动重发；再次确认使用新请求并保留原卡片。既有本地日志恢复、首次领取排序与五候选上限回归保留。 |
| App 定向 owner | `TestTaskReadinessWithoutDocumentReads`：无 Git/文档/进程 host 时仍读取状态，accepted→ready、远端变化/观察失败撤销就绪，缺失任务不借用其他任务状态；`TestTaskReadinessRejectsUnsupportedQueries`：重复 view、其他 view 或多余参数返回 400。同时运行原 TestPM/TestPMS 材料、版本和预留门禁回归。 |
| `go.http` | 原七个 HTTP owner 全量执行，验证既有身份、鉴权、查询转发与快照边界；新增状态视图的业务参数边界由上列 App owner 实测。 |
| L1、L2 | 固定最小集与受控完整交付冒烟，L2 清理必须完成。 |

## 实际结果

原始报告根：`C:\Users\lmas1\AppData\Local\Temp\steerline-start-readiness`。以下路径均相对该根；报告记录脏工作树源码，不导出为干净提交的 Git 测试历史。

| 命令 / 范围 | 结果与证据 |
| --- | --- |
| `go run ./cmd/test-feedback -out <目录> -- ./internal/app -run <本次正则> -count=1` | 23 项通过，零跳过；326.669 秒。`diagnostics/20261105T074402237Z/summary.json`、`go-test.jsonl`。 |
| `go run ./cmd/test-feedback -level=L1 -out <目录>` | 通过，准备 0.682 秒、执行 51.662 秒。`test-results/L1/20261105T074536546Z/summary.json`。 |
| `go run ./cmd/test-feedback -level=L2 -out <目录>` | 通过，准备 59.047 秒、执行 227.022 秒。`test-results/L2/20261105T074703585Z/summary.json`；`scenarios/smoke.delivery/scenario.json` 的 cleanup_complete=true。 |
| `go run ./cmd/test-feedback -level=L4 -suite=web -scenario web.unit -out <目录>` | 35 个测试文件、281 项断言通过，零失败/跳过；43.497 秒。`test-results/L4-web/20261105T074706641Z/summary.json`、`web-unit/vitest.json`。 |
| `go run ./cmd/test-feedback -level=L4 -suite=go -scenario go.http -out <目录>` | 七包、164 项通过，零失败/跳过；86.990 秒。`test-results/L4-http/20261105T075220267Z/summary.json`、`go-owner/test.jsonl`。与本轮 L2 串行。 |
| `npm --prefix web run build` | TypeScript 检查和 Vite 生产构建通过。 |

App owner 的实际 `-run` 参数为 `Test(TaskReadiness|PM)`，匹配本次新增状态视图及原 TestPM/TestPMS 回归。

预备默认沙箱 L1 未通过：返回 quick-core/quick-boundary 失败，并在写 managed 报告时出现 Access is denied；返回的沙箱临时目录后续已不可读，未取得完整原始失败日志，不能确认这些失败的具体根因。该轮不计入通过；上表正式 L1 在正常权限的独立临时目录重新执行并完整保存报告。初轮前端历史用例因模拟服务仍返回 ready 而覆盖模拟 accepted 失败，已修正为随阶段改变服务状态；状态视图夹具的多余 files 字段也经构建检查清理，上表记录最终版本。

没有运行真实模型、远端 PR、完整 Go 全集或浏览器人工体验。安装版仍需后续部署才能呈现上述行为。
