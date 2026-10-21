# 2026-10-21 系统重启后的安装部署恢复

用户要求先 rebase，再处理部署的停机身份核验失败。当前分支 `codex/fix-runner-takeover-query` 已 rebase 到当时最新 `origin/main`：`9e02854ef66e1b1e4b1fbdff35a9eeca294b26a5`，原未提交维护文档全部保留。本次为用户授权的实际安装维护，未将安装环境作为回归测试现场；未清库、清理任务、停止原生业务终端或修改 Git/PR。

## 原因与证据

证据根目录：`C:\SteerlineInstall\maintenance\shutdown-recovery-20261021`。

- 原 `application-api/shutdown.json` 属于宿主 `c76ddc30…`，时间为 2026-10-20 18:00:57；最新 `instance.json` 属于其后启动的宿主 `c2c46334…`，时间为 18:01:02。两者身份不同，旧回执不能证明新实例的本地关闭。
- 最新 Runner 服务 `afd00fad…` 已退出，但其停服回执的 request_id 为空。原资源结算为 false，三个原 Run 的 native stop status 未核实。报错正文中只有一个 Run 来自更早的宿主停机回执，不能当作本次完整现场。
- Windows System 事件 1074 确认 2026-10-21 09:14:07 发起电脑重启，6006 为 09:14:31 日志服务停止，6005 为 09:15:09 重新启动；Runner 于 09:14:19 退出，Orca 查询于关机期间返回 0xc0000142。`reboot-events.json`、`runner-before.json` 与旧宿主三份归档共同解释最新正式停机回执缺失。
- 用户部署包 `20261021-012859-9707491b` 的兼容预检已为 compatible=true，覆盖三个原 Run。因此本次直接阻塞是最新宿主/正式停服证据缺失，并非候选不兼容；核验拒绝是既有身份围栏的正确行为。

## 恢复与部署

1. 归档当前身份、旧回执、Runner 状态及任务/候选原身份摘要，不改写历史失败记录。
2. 从原安装构建启动，等待全部项目 ready。随后正式 stop，生成宿主 `485fd97f…` 及 Runner `69642199…` / `sl-service-stop-6964219987fd0d49816fb1cf26eaea71` 对应的当前停机回执。
3. stop 的 strict settlement 仍返回 exit 1，因为资源结算未核实；原诊断保持。新候选 `verify-restart` 返回 exit 0 / restartable=true，核实原服务已退出、项目关闭/锁/checkpoint/候选绑定及三个原 Run 的兼容性，不把 resources_settled=false 晋升 true。
4. 沿正式 `scripts/build-deploy.ps1 -UpdateRunner -NoStart` 部署，在脚本内再次取得兼容及停服证明，然后正常启动。未增加旧协议回退、自动放宽身份验证或原生停止逻辑。

最终包：`C:\SteerlineInstall\deployments\20261021-013251-490f964a`。SteerLine 为上述 rebase 基线，dirty 仅包含本地维护文档；Runner 源码 `8261310d5f2d1edb6fa0c81ba05fa4119b561386`、版本 `0.2.0-dev.3-r2`、SHA256 `4dbf25c74ecc5d360fbfacc88fe599744cdc80a10648962ddc86f73c7d4b467b`。

## 验证结果

| 范围 | 命令 / 证据 | 结果 |
| --- | --- | --- |
| L1 身份与只读停机围栏，隔离 fixture | `go test ./internal/app -run 'TestUnifiedStartupLifecycleShutdownEvidenceFencesOriginalHost\|TestDeploymentRechecksFailedReceiptWithoutRestartOrRewrite' -count=1 -timeout=120s` | 通过，35.133s |
| 原安装恢复 | `original-ready.json`、`original-start-result.json` | 后端/Web/三个项目 ready |
| 新正式停机与接管 | `current-shutdown.json`、`verify-restart.json`、`normal-stop.log` | 当前宿主身份匹配，原 Runner 有固定正式 stop request；restartable=true、compatible=true，资源诊断原样保留 |
| 正式部署 | `build-deploy.log`、部署包 `shutdown-verification.json` / `runner-takeover-preflight.json` | exit 0，三份 EXE 摘要与记录一致 |
| 原业务事实 | `state-before.json`、`final-verification.py` / `final-verification.json` | 20 个原任务及契约/依赖保留；三个原 Candidate 的 Run ID/input/submit 信封及原构建 pin 均一致，零新增 Candidate；原 PR #640 保留；任务 101 仍 in_progress，其余任务 open；各项目 dispatch 完全未变，SQLite integrity/foreign-key 通过 |
| 更新后启动 | `status-after.json`、`start-result.json` | backend/Web ready，三个项目 ready 且 online，degraded=false |
| Web | `Invoke-WebRequest http://127.0.0.1:18742` | HTTP 200 |

三个原 Run：`770c2c38beb04bd2f3ee92c567f27d0a`、`ac849fa27a466b37288665f92f3e5cb1`、`6ffd5b89c8fa9d95950f15750fae40f8`。两项 Orca 创建未知仍沿原身份对账；本次没有通过 stop/reply/resume 或新 submit 代替原执行，也未将资源未核实登记为通过。

本次只新增安装说明与实际维护记录，未修改产品实现，未运行 full、L2/L3、真实模型或真实 PR 验收。正常恢复服务沿既有产品授权和持久意图运行；该维护结果不代替原生执行专项验证。
