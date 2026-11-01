# Runner v8 接入与安装清理（2026-11-01）

用户直接要求适配新 Runner、提 PR，随后授权清理 `C:\SteerlineInstall` 的旧安装执行进度，不用备份。此前 v7 适配已通过 PR #419 合并；本次发现当前独立 Runner 已发布 state/v8，因此将接入格式、home 派生命名空间、部署脚本及公开夹具同步到 v8，Run 投影保留公开 `stop_evidence_scope`。不增加旧格式读取、迁移或兼容路径，不修改固定提示词、材料或黄金样本。

## 受测版本与证据

SteerLine 受测来源为 `067de7056569ac586cd76715d994f1639642d858` 加本次 v8 工作树差异，source_dirty=true。安装包冻结于工作树 `bin/install-v8-20261101-3701`，后续文档修改不重新改写该包。Runner 来源 `a06c2806c17056d76cb092b6a1045a0473765fb8`，version `0.2.0-dev.4`，SHA256 `4eaa3776f9352b6e634e23cfe3cf146b97d0fe8c763812bbbf58cf00cce0baf8`；state/v8，资源版本 3，digest `febc0f93e64f725b8d48b82c404786ffcee35b1fc001b6a352a080b041d4ff01`。保留 Runner 公开证据范围，不将 Orca 停止证据扩大为全部后代资源不存在。

原始证据根为 `C:\SteerlineDebug\install-v7-20261101-3701`，目录名保留首次维护尝试时的 v7 名称，以下路径均相对该根。

| 验证 | 结果 | 证据 |
| --- | --- | --- |
| 完整 v8 构建与候选离线升级预检 | 构建 succeeded，12.91s；空 home 预检通过 | `v8-build-timings.json`、`v8-build.log`、`fresh-v8-preflight` |
| 固定 L1 | passed/complete；准备 0.516s，执行 14.582s | `v8-L1/summary.json` |
| 固定 L2 | passed/full-L2，cleanup_complete=true；准备 8.133s，执行 119.853s | `v8-L2/summary.json` |
| 真实 Runner 与真实 applicationRunner | 两项 passed/complete、零跳过；11.858s | `v8-public-contract/summary.json` |
| 升级与命名空间定向 | 25 项 passed/complete、零跳过；10.994s | `v8-upgrade-tests-complete/summary.json` |
| PowerShell 5.1 部署停止、部署流水线 | exit 0；流水线 8 场景通过，全部隔离假安装 | `v8-deployment-stop-test.log`、`v8-deployment-pipeline-test.log` |

真实契约选择 `TestPublishedRunnerApplicationLifecycle`、`TestLegacyCheckRetirementPublishedContract`；只进行隔离空闲服务启动、同构建重启、停止及公开结算，不启动业务模型、远端执行或 PR。定向命令为 `go run ./cmd/test-feedback -out <证据目录> -- ./internal/app ./internal/runnerstatepath ./internal/runnerclient ./cmd/steerline -run '^(TestRunnerUpgrade|TestRunnerNamespace|TestRunnerSocket|TestRunnerRejects|TestSharedRunnerBuildPinBeforeService|TestSharedRunnerRejectsBuildBeforeStart).*' -count=1 -timeout=240s`。此前 `v8-upgrade-tests` 的锚定表达式只匹配完整短名称，不充作此完整选择的通过证明。未执行 go.full 或真实模型交付链。

## 安装维护边界

保留三份项目配置、Web 配置、七个执行方案及默认方案、全部 20 个任务定义/8 条依赖和项目 Git 文件；清理旧执行状态、日志、旧归档/迁移/维护目录、旧部署包和唯一原 Runner namespace `C:\slr\beret3eg7x3tdkzggckgqkuhmy`，不复制或备份数据库。新库由当前格式创建，任务经正式 validate/create 入口重建为 open，派发保持暂停；原 schema36 不伪装为当前 schema39。

原版正常停服后，宿主/Web 退出、全部项目锁释放，原 Runner `not_running`；公开 `service verify-stopped` 确认 resources_settled=true，覆盖原调用方记录的七个 Run。完整任务定义使用正式 domain 校验器和正式 Git 文档验证器，在空目标图中验证，没有忽略任何 issue。维护期间不改动另一个调试包的 Runner/agent 或其 namespace。

每个递归清理路径均先验证绝对边界、祖先及子项无链接。旧归档中唯一 Junction 指向仍需保留的 `tools/dsh/node_modules`；仅非递归解除该归档链接，并核实工具目录仍有原 247 个顶层项，再重新完整检查所有清理目标。安装/启动期间分别持有部署独占锁与启动协调锁，使用本次冻结的三个 EXE，并校对安装后摘要。

实际安装于 2026-11-01 21:22（Asia/Shanghai）完成替换，21:27 完成任务重建和运行核实。三个项目均 ready/online，后端 PID 47480、Web PID 43184、Runner PID 47556，Web `http://127.0.0.1:18742` 返回 HTTP 200，degraded=false。当前 Runner namespace 为 `C:\slr\rehnosf7lwgci2qaqusuz6z56u`，原 namespace 已不存在，安装三个 EXE 摘要均与冻结包一致。

20 个任务全部 open、原编号不变，8 条依赖逐条一致；三项目均 manual + paused。最终只读检查确认三个库均 schema39，runner_candidates/test_generations/project_environments/roadmaps 全为零；新建的 task create 回执属于本次任务定义重建，不是旧执行进度。保留全部七个 profiles 与原默认 project-default、配置和 tools；失效的旧根目录 `MIGRATION.md` 一并删除。原调试 Runner PID 43596、agent PID 35288 仍为原路径，未操作其进程或 namespace。

原始核验、清理目标、维护入口、安装包来源、启动及任务正式回执保留在上述证据根，未备份原数据库或 Runner 状态。`installed-ready.json`、`status-final.log`、`current-state-verified.json` 为最终证据；`finish-install.log` 及三项目的 `tasks-after-*`/`graph-after-*` 为正式重建结果。临时任务定义输入在全部核实后删除。

### 原失败保留

- 初次构建发现实际 Runner 已从 v7 变为 v8；先完成代码对齐及隔离验证，再处理原安装。
- PS5.1 默认文本解码导致配置读取失败；改为显式 UTF-8，尚未删除数据。
- 原旧图对同 ID/边任务重建报重复。忽略 duplicate_dependency 的重试被自动审批拒绝，未执行；改为正式完整空目标图验证，全部 issues 为空，未过滤或绕过任何门禁。
- 第一次完整清理前的路径扫描发现上述 Junction，未删除任何目标；解除链接后重新执行安全检查。各原失败日志均保留，不计为通过。
- 新服务首次状态查询落在创建身份尚未登记的瞬间，原维护入口立即失败并保留任务输入；随后独立正常启动退出 0，正式 status 确认三项目 ready。仅继续任务导入，未重复清理或替换新服务。
- 首次任务创建使用了带前缀的请求 ID，被正式入口明确拒绝、未写入任务；按公开要求改为 32 位 hex，再次确认空目标并完整 validate/create 成功，原拒绝日志保留于 `finish-before-request-id-fix.log`。
