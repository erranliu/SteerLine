# Runner 兼容接管验收

2026-10-19，基线 `9554eb61eefa417f5cd900d335650dacff3d3311` 的工作树增量；[Spec](../spec/service-generation-recovery.spec.md) / [Plan](../plans/service-generation-recovery.plan.md)。**实现与受控验证进行中，整体未完成**。未提交、未部署、未修改安装/保留运行环境。

| 项目 | 实际结果 | 边界 |
| --- | --- | --- |
| SLC-A01 | runnerclient 全包通过，5.071s | 公开进程协议 fixture；含新增 takeover 拒绝与 startup 原信封/确定拒绝 |
| SLC-A02 | app/client 定向通过，92.822s / 1.842s；项目身份解析修复后 app 重验通过 88.746s | 含新兼容构建预检与当前 pin 更新、旧 candidate/PR/unknown 保留；未使用真实 native |
| SLC-A03 | 首次新增场景失败：复用 fixture 缺 spec.md/plan.md；补齐后通过，28.697s | 原 Run/revision 的首次 stop 信封先保存，重复收集保持同一字节，不启动 Runner，不改原 submit/Run/build |
| SLC-A04 | scripts/test-deployment-stop.ps1 通过 | 同构建与跨构建、缺候选证明、原 resources_settled=false 均覆盖；未运行 build-deploy 或替换安装文件 |
| SLC-A05 | 未执行 | 尚无 SteerLine→真实 Runner/原生更新的同一现场证据；Runner 受控通过不能替代消费者联调 |

测试中发现新增启动收集器在 LoadConfigSnapshot(false) 后过早取得项目锁，会生成空身份的锁文件；已改为先 ResolveIdentity，再取既有项目/run_dir 锁。清除了本次测试产生的空临时锁，定向重验通过；没有放宽项目身份或 Store 绑定。

外部阻塞：Orca 1.4.224 的公开 worktree/terminal create 帮助未暴露持久原请求关联键或原创建结果查询；没有创建探针资源，没有按名字接管。需外部原生协议明确接口后才能验证成功但无回执窗口。完整原生/跨仓现场与安装更新仍未执行。

补充消费者边界：环境检查的历史 selection 作为原事实保留；包括非运行时的显式查询，也在核对原 Check ID/input digest 后经当前所选 CLI 读取，不依赖已经替换/移除的旧 EXE。`go test ./internal/app ./internal/environmentcheck -run 'TestEnvironmentCheck|TestStartupCancellationOutbox|TestSharedRunnerCheck' -count=1 -timeout=4m` 通过，58.710s / 3.767s，同时核对错 input digest 拒绝与原 selection/resource digest 未改。初次新增 fixture 编译因 client/contract capabilities 类型未转换而失败，已补显式转换；不算协议执行失败或通过证据。PowerShell 四个变更脚本的 Parser 检查和两仓 diff 空白检查均通过；脚本实跑日志在 `.evidence/service-generation-recovery/deployment-script.log`，其余定向结果以本页和会话命令回执登记。
