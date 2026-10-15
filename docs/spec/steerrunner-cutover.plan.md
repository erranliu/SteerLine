# SteerRunner 接入与旧执行域删除 Plan

用户直接授权的跨仓开发。输入为 SteerRunner 仓库 `docs/spec/steerrunner.spec.md` 与 `steerrunner.plan.md`；执行代码来源固定为 SteerLine `d20a72284705b4daac476d4fa5770151dca03e56`。不修改 roadmap，不兼容旧数据库或配置，不提供旧 worker 回退入口。

## 边界与实施步骤

1. Runner 独立拥有 workflow/reliable、原生适配器、执行工作区、会话、问题、输入、原 final、发布核验与恢复。SteerLine 不引用 Runner 的 Go 包、私有 IPC 或数据库。
2. 实现 `runnerclient`：仅通过已固定 EXE 的 `steerrunner-cli/v1`、stdin JSON、退出码和 stdout JSON 调用。每次调用验证 EXE SHA，显式服务启动核对常驻实例的 EXE/版本/资源摘要；读配置不启动服务。采用 Apache-2.0 的 jsonschema/v6 验证 Runner 公开的自包含 schema，禁止 schema 文件/网络读取，不复制原生配置 codec。超时与不完整响应按是否可能接受区分，未知保留原 request_id/body。
3. `runnerbridge` 在 Task 自有库保存候选绑定、CLI outbox、投影和游标。Task 领取与完整固定请求原子保存；原请求接受后不可改 Run/输入摘要/分支/PR。停止义务与 Task 终态同事务保存，迟到接受继续对账停止；旧答案/反馈/resume 不自动重新授权。
4. `taskrunner` 负责领取资格、固定契约、候选共同 base、前置 merge 祖先检查和 Task 交付。交付前经 CLI 刷新所选 PR；Task 关闭、唯一交付、后继释放、所选和同组候选停止义务一次提交。不将 pr_ready 或兄弟候选完全结算作为交付门槛。
5. 替换实际 app service、配置、命令、查询投影和调试/分级测试 driver；固定 Runner 版本及 SHA。删除 SteerLine 旧执行 actor、worker、适配器、写表及对应专属测试。保留 Task/DAG、候选参与关系、唯一交付及 UI 的业务入口。切换和删除必须在同一 SteerLine PR 中完成。
6. 两仓最小验证及文档审计通过后提交 Runner，再以可获取版本固定 SteerLine；分别创建并关联 PR。未完成实际切换或未验的支持目标，不声明跨仓交付完成。

生命周期规则只链接[统一入口](../product-design/worker-lifecycle.md)。本次差异是执行 owner 迁到独立进程，Task 侧只对原 CLI 操作记账，不再维护原生轮次状态机。

## 测试计划

验收真相源为[专属验收](../acceptance/steerrunner-cutover.md)。各项均须实际执行并登记证据，不将 Plan 当运行结果。

| 验收项 | 级别、场景与命令 | 环境、执行时点 | 通过条件与证据位置 |
| --- | --- | --- | --- |
| SLC-A01 | unit：`go test ./internal/runnerclient ./internal/runnerbridge -count=1 -timeout=2m` | 隔离 CLI 子进程、内存提交故障；桥接实现后 | stdout/ID/超时语义正确；保存失败零调用，未知不换请求，终态不重新提交；`.evidence/steerrunner-cutover/client-bridge.log` |
| SLC-A02 | unit + SQLite：`go test ./internal/storage/sqlite -run '^TestRunner(Candidate|TaskService|Actor|ApplicationSnapshot|Dispatch|Control)' -count=1 -timeout=2m` | 新建临时库、WAL 并发提交及命令回执故障；绑定/调度实现后 | 原子领取/请求、重试原回执、不可变绑定、事件/游标 CAS、终态 stop、单事务页面/Runner 快照、调度/回执全回滚及真实 host scheduler；`candidate.log`、`snapshot-dispatch.log`、`control.log` |
| SLC-A03 | unit + SQLite：`go test ./internal/storage/sqlite -run '^TestRunnerTaskDeliveryWithoutWorkflowReady$' -count=1` | 新建临时库、事务中注入失败；交付实现后 | 非 pr_ready 的正确合并候选可交付，兄弟未知不阻塞；失败全回滚，成功唯一关闭/释放/停止，重试不查询 PR；`delivery.log` |
| SLC-A04 | integration：新包后独立启动 `go test -c ./tests/e2e/internal/driver` 的 EXE，以 `STEERLINE_DEBUG_TEST_ROOT` 选择包，运行 `-test.run=^TestDebugReleaseRuntime$ -test.timeout=12m`；脚本入口 `scripts/test-debug-release.ps1 -SteerRunnerExe <absolute>` | 隔离 Git 与当前用户进程；app 切换后 | 实际产品命令经过 Runner，未知/重启/停止及反馈沿原 Run，日志归项目 runner 目录；`cli-integration/` |
| SLC-A05 | L3 controlled：沿用 Runner Plan R1 的五个登记场景，由步骤 5 将 driver 改为新 CLI 后执行 | 既有隔离分级环境；旧 owner 删除后 | 场景非零且通过，产品入口与 UI 投影不依赖旧执行库；`regression/summary.json` |
| SLC-A06 | build + owner audit：`go build ./...`、两仓 `git diff --check`、实际引用/写表删除审计 | 新配置及固定 Runner 包；最终 PR 前 | 无旧执行写者/回退、无 Runner 私有依赖；实际 EXE 版本/SHA 可追溯；`deletion-audit.md`、`compile.log` |

Runner 真机验收登记在 Runner SR-A11/SR-A12。先运行 `scripts/new-debug-release.ps1`；仅 SteerlineDebug 下独立包、SteerlineTest 专属 base/head。远端用户授权主机为 Orca「台式机64G」。当前已知本地 Codex Windows 沙箱失败须保留原始证据并解决，不扩大权限来制造通过。需要补齐的工具和 driver 属于步骤 5，不留作无明确安排的“后续补验”。

不默认跑 full 或全部 L2/L3；这里只选择本次切换涉及的最小恢复、交付和 CLI 场景。人工安装体验验收可后置，不代替自动集成验收。


实施补充：受控 R1 仅通过候选保存的公开 Run/Execution/Controls 断言身份；原生状态、启动/业务投递次数由外部协议夹具提供，不从旧执行表重建。Task 中心崩溃时 resident Runner 保持；原生恢复场景显式停止/重开 Runner 服务，再通过产品 `runner_resume` 决策提交新授权。业务窗口 10 分钟、Go 父进程每场景 12 分钟包括清理，单命令仍有原时限。停止未证明时保留现场；先停止/围栏原服务，必要时显式创建仅清理 epoch，停止已绑定原 Run，禁止用父进程退出代替原生回执。

新调试脚本通过 `-SteerRunnerExe` / `STEERRUNNER_EXE` 选择固定发布 EXE。`-Live` 使用实际 Runner/Codex 工作流验收，认证仅使用宿主已有登录；禁止从生成包读 token、导出凭据、调用宿主发布脚本代替 worker。其 native 前置/证据仍登记 Runner SR-A11/SR-A12 和相应独立现场，不将脚本语法检查当作 live 通过。


2026-10-15 用户明确放弃本机 Orca 路径：移除 terminal-local-reconnect 必验项；当前 R1 为基础交付、waiting-merge-restart、本机 Codex resume、远端 environment unknown-send 四项。历史冻结批次包含的本机 Orca 失败只保留证据，不再重跑或阻塞验收。远端原生环境修复后，继续按新包完成 environment 真机闭环。
