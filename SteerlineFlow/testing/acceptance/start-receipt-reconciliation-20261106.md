# 启动回执保存失败后的原请求核实（2026-11-06）

## 现场与范围

SteerlineTest 101 的启动请求 `d7580327aec14d6ab4f9cb1f036ded0b` 在替换 `application-commands/<request_id>.json` 时发生 FileRenameInfoEx Access is denied。只读原请求查询确认其在北京时间 11:25:01 applied，随后任务继续至 pr_ready。错误发生于 HTTP 回执摘要保存，不能据此判断业务启动失败或自动停止原任务。Windows 当时拒绝替换的具体占用者/权限原因尚未查明，本次不改变私有 ACL、原子替换或文件重试预算。

源码基点 `e9c04978e999f4fe75893bbdf242d9f1df08731c` 加当前工作区修改。独立证据根 `C:\SteerlineDebug\wt-5a39-start-receipt\`。

## 实现

- Web 只对 `request_receipt_persist_failed` 立即在 5 秒预算内 GET 原 request ID；持有原请求发送锁时直接只读查询，不再次获取同一锁或补发 POST。返回结果仍核对完整原信封、项目/实例/目标/版本。
- applied 按已生效处理并更新浏览器记录，清除提交诊断；明确 rejected 保留拒绝提示。提交/受理中结果继续查询。首次核实超时、不可用、暂时缺失或身份不符时保留 outcome_unknown 和诊断，启动卡显示待核实，全局不误报启动失败。
- 一旦已有 submitted/accepted/outcome_unknown 回执（包含回执保存失败的未知观察），重连、重复调用及后台核实即使遇到 command_not_found 也只读查询；无任何回执的可补交请求保留原规则。全部请求继续使用原 ID、正文和版本。
- 其他提交错误继续原样失败；项目切换后的晚到 applied 不得确认到新页面。没有新业务请求、协议或数据库迁移，也不回滚或自动停止已启动任务。

## 验证

| 范围 | 命令与结果 | 证据（相对证据根） |
| --- | --- | --- |
| 定向前端 | `npm --prefix web test -- src/shared/api/client.test.ts src/app/use-original-requests.test.tsx src/app/use-candidate-starts.test.tsx`，50 项通过 | 本次原生命令输出 |
| L1 | `go run ./cmd/test-feedback -level=L1 -out <证据根>\test-results\L1\run-01`，通过 | `test-results/L1/run-01/summary.json` |
| L2 | `go run ./cmd/test-feedback -level=L2 -out <证据根>\test-results\L2\run-01`，通过 | `test-results/L2/run-01/summary.json` |
| L4 web.unit | `go run ./cmd/test-feedback -level=L4 -suite=web -scenario web.unit -out <证据根>\test-results\L4\web-unit-01`，309 项通过、零失败/跳过，17.556 秒 | `test-results/L4/web-unit-01/summary.json`、`web-unit/vitest.json` 和 stdout/stderr |
| 清单与文档 | 正式清单一致性两项 Go 测试、`python scripts/check_doc_links.py`、`git diff --check` 通过 | 本次命令输出 |

覆盖立即核实的 applied/submitted/accepted/rejected、查询失败/缺失/身份不符/超时、重开客户端与多轮查询零额外 POST、项目切换晚到结果、无关错误不被隐藏，以及启动待核实→原请求 applied 的界面状态和真实拒绝提示。原候选启动编排回归保留。

首次定向测试中两项 hook 用例错误地给同一个已 applied 请求伪造后续 rejected，触发现有终态保护而失败；改用独立原请求测试真实拒绝后，50 项通过。未放宽终态保护或生产断言。

## 安装应用

在上述隔离验证通过后执行普通更新：

```powershell
.\scripts\build-deploy.ps1 -InstallHome C:\SteerlineInstall -RunnerRepo C:\Project\SteerRunner -NoPause -TimingPath C:\SteerlineDebug\wt-5a39-start-receipt\diagnostics\installation-01\deployment-timings.json
```

复用原固定 Runner，无 UpdateRunner/DiscardRuns。构建含 TypeScript 检查与生产 Web 构建；部署 succeeded=true，总耗时 42.49 秒，stop-and-verify 13.19 秒，全部 3 个项目、后端和 Web ready。安装包 `C:\SteerlineInstall\deployments\20261106-033843-28f15e9d\` 保存原停机/重启资格证明及本次程序摘要。

部署后再次只读核实：原 Runner SHA256 不变，实际 HTTP 首页与本次构建一致；101 仍为 in_progress/pr_ready，仅一个候选，原 workflow Run `d7580327aec14d6ab4f9cb1f036ded0b`、Execution `507c52e5c1d3f7831e8b2f5e8aa6b95d` 均保留，stop_requested=false，原启动请求仍 applied。未重复启动、停止或重建 101，没有清理任务库。

证据为 `diagnostics/installation-01/` 下的 `deployment-transcript.log`、`deployment-timings.json`、前后 status/task-101/original-request、原 Runner 摘要和 `final-verification.json`。未在真实任务上人为制造文件占用或回执失败；错误分支由隔离测试验证。用户刷新页面加载新 Web 资源后生效。
