# generate 接管：合入修复后的固定主干复验

归属 [TM-A04 验收](../../test-management.md#2026-10-20-合入修复后的固定主干复验)。SteerLine `f139e4a9`、Runner `13ef12312944317e7b78d4de44977b92e65e70c2`，执行时工作树均干净。

[汇总](summary.json)、[构建身份](runner-build.json)与[公开 version/resource digest](version.json)固定本批来源；EXE SHA256 为 `482aafcffd8b61516f26e842361602741643f52e2d77d2be5ccea6461da4292f`。

- [调用方原始 summary](caller-summary.json)：七包定向 33 项通过，complete=true、零失败/skip，69.822 秒；原命令及源码状态在原文件内。
- [Runner 定向日志](runner-directed.log)：application/resultacquisition 共 27 项（含子用例）通过；workflow 包未命中，不计其整包通过。
- 原失败现场：[历史拒绝](original/original-rejected-response.json) → [本轮公开接管响应](original/verify-takeover.json)，[绑定及只读检查](original/verification.json)确认两项原 Run/revision/input/request 不变、181 个文件前后摘要一致；[服务检查前](original/status-before.json)/[检查后](original/status-after.json)均未运行。
- 新隔离真实 Codex：[身份](native/native-identity.json)、[目录](native/isolation.json)、[原 submit](native/submit-request.json)/[回执](native/submit-response.json)、[原结果](native/content-result-response.json)、[显式 stop](native/explicit-stop-response.json)/[原结果保留](native/stopped-inspect-response.json)、[服务 stop](native/service-stop-response.json)、[结算](native/verify-stopped-response.json)、[同 EXE takeover](native/verify-takeover-response.json)、[数据库不变](native/database-unchanged.json)。[驱动日志](native/integration.log)/[退出及非零场景数](native/result.json)/[驱动构建身份](native/source-identity.json)对应同一次执行，30.25 秒、一项通过。

复现命令（Runner 仓库，输出必须为新目录）：

```powershell
./scripts/test-foundation.ps1 -Pattern '^TestSRContentGenerateTakeoverNative$' -OutDir <新目录> -ContentNativeExecutable <固定Codex绝对路径> -ContentNativeModel gpt-6.1-sol
go test ./internal/application ./internal/execution/workflow ./internal/execution/resultacquisition -run '^TestContentFrozenTakeoverContracts$|^TestServiceGenerationTakeover|^TestTakeover|^Test.*ResultAcquisition|^TestRAConfig' -count=1 -timeout=3m -v
& <固定RunnerEXE> service verify-takeover --candidate <同EXE> --state-dir C:/slr/y6mgtaht4xl5jc2j3mufxchxoe --json --protocol steerrunner-cli/v1
```

完整原始输出、CLI stdout/stderr、现场前后文件清单及只读校验脚本保留在 `C:/Users/lmas1/AppData/Local/Temp/steerline-generate-recheck-20261020-1792476800182`。仓库复制必要原始公开响应，未改写历史现场或模型结果；[归档文件摘要](manifest.json)仅用于核对本目录复制字节。

本批只证明固定主干的生成契约接管、原记录读取资格及调用方定向边界。上层 A/B、报告/API/浏览器及 i/j 清理沿用原任务已有证据，不将旧 dirty 构建的通过换记到本批。未运行 Full、前端或真实 conversation，未部署；TM-M01 人工待验。
