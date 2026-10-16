# Runner 短状态路径实现 Plan

## 目标与范围

SteerlineTest 101/C1 的安装现场在 `prepare_session` 因 `authentication_api_socket_path_too_long` 阻断。原 `home/state/<项目名与摘要>/runner` 使认证 socket 达到 124 字节，Runner 上限保持 106。SteerLine 改为本地卷根 `slr/<128-bit base32 key>`；key 取完整应用 run-dir 的 SHA-256 前 128 位，因此绑定 home 和项目，不随 home 深度增长。Windows 标准本地卷下 state-dir 固定 34 字节、当前 Runner socket 98 字节。

只调整调用方目录布局，不修改 Runner、执行权限、W01–W18 推进、重试或恢复授权。统一规则见 [worker 生命周期](../product-design/worker-lifecycle.md)；本次差异为 W02 目录准备和 W18 资源定位。旧目录不迁移、不双读、不重新绑定原 Run，已有现场保持原始证据。

## 实施

1. 建立共享路径函数，按完整 run-dir 固定命名空间；查询验证不创建目录，显式服务启动才创建当前用户私有目录。拒绝网络卷、目录别名及超长路径。
2. 启动、查询、环境检查、候选绑定校验和离线 verify-stop 使用同一目录；SteerLine Task 数据库继续在原 home。
3. 模拟包路由显式登记短 Runner 根；重置仍先核验原服务与 Run 停止，再只清理本项目摘要目录。测试在证据完整且停止成功后回收外部目录，失败时保留。交付 smoke 先等待异步 stop 的公开确认，再断言最终请求数；新路径用例纳入固定 L1。
4. 更新当前数据布局、集成说明及验收文档。

## 测试计划

验收项归属：[专属验收](../acceptance/runner-short-state-path.md)。日志归 `.evidence/runner-short-path/`，调试包证据另登记实际路径。

| 级别 / 场景 | 自动项 | 环境、命令、执行时点 | 通过条件与证据 |
| --- | --- | --- | --- |
| L1 定向 / 深 home socket、命名空间及别名 | RSP-A01、A02 | Windows 隔离临时 home；实现后 `go run ./cmd/test-feedback -out .evidence/runner-short-path/paths -- ./internal/runnerstatepath -count=1` | 深 home 下真实 AF_UNIX bind 成功、路径不超 106；跨 home/项目独立；别名在创建/删除前拒绝；兄弟 sentinel 不变。summary/jsonl 登记 A01/A02 |
| L1 定向 / caller 入口及停止证明 | RSP-A03 | 独立 fixture；接线后 `go run ./cmd/test-feedback -out .evidence/runner-short-path/unit -- ./internal/app ./tests/e2e/internal/controlled ./tests/e2e/internal/driver -run 'Test(Runner|Deployment|DebugReset|EnvironmentCheckDebugReset|SharedHostRoutes)' -count=1` | preflight 不启动 Runner，停服离线核验使用原目录且不改原证据，双项目 provider 路由正确；summary 登记 A03 |
| L2 controlled / smoke.delivery | RSP-A03 | 既有分级隔离夹具；接线后 `go run ./cmd/test-feedback -out .evidence/runner-short-path/gate -level=L2 -env=controlled` | 候选启动、查询、交付、停止和短目录清理通过；gate-only summary/scenario 登记 A03 |
| 调试包定向 / 双项目与 reset | RSP-A04 | 先运行 `scripts/new-debug-release.ps1 -DebugRoot C:\SteerlineDebug\short-runner-path -SteerRunnerExe <独立构建EXE> -WebProjects 2`，等 manifest ready；用既有 `scripts/independent-test.ps1` 建立独立用户进程，设 `STEERLINE_DEBUG_SECOND_TEST_ROOT` 为该包，再运行 `go test ./tests/e2e/internal/driver -run '^TestDebugReleaseWebProjects$' -count=1 -timeout=6m -v` | 显式 simulate；深目录新包的两个项目独立、活动时阻止 reset、停止后只重置本包两个项目并保留任务定义；包 evidence 登记 A04 |

不跑 Full；不选择 L3/L4 整组，路径边界用上述定向用例验证。真实 socket bind 验证本次 OS 长度缺陷，不需模型/PR；controlled gate 与 simulate 不计完整真实 L2。无人工验收项，安装环境未部署不计现场 C1 恢复。
