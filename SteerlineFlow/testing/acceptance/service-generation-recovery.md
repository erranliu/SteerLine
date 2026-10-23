# Runner 兼容接管验收

2026-10-19，基线 `9554eb61eefa417f5cd900d335650dacff3d3311` 的工作树增量；[Spec](../../spec/service-generation-recovery.spec.md) / [Plan](../../plans/service-generation-recovery.plan.md)。以下保留当日受控结果；2026-10-20 的本机真实联验和剩余 Orca 边界见末节。安装/保留运行环境未用于验收。

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

## 2026-10-20 独立真实联验

SteerLine 基线 `0752537e` 的工作树增量；Runner #35 已合入的干净主干 `bf3ec581cb0b23a7358d95e5efa8ff5f607c2f55`。用户直接授权先修部署测试入口，再在独立调试环境补跑兼容更新、原结果补收及继续交付。全部现场由 `scripts/new-debug-release.ps1` 生成在 `C:\SteerlineDebug`，未使用或修改安装/保留运行环境。

- SLC-A04：修复 Windows PowerShell 5.1 首次自动加载 Utility 模块覆盖 `Get-FileHash` mock 的入口问题。在安装 mock 前显式导入模块，原断言保留。`powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-deployment-stop.ps1` 已通过，覆盖 strict settlement、兼容 takeover、复用 pin 及失败拒绝。
- SLC-A01/A05：第一个 live 包 `dev-20261020-101614-0752537e-dirty` 启动时发现产品缺陷：目录中合法的 conversation/generate 无固定 prompt steps，却导致整个 Runner catalog 被消费端拒绝。已删除此跨工作流假设，所选配置继续用公开 schema 校验；新增回归，`go test ./internal/runnerclient -count=1 -timeout=120s` 通过，5.687s。该包无原生任务/PR，停服后保留启动日志。
- 第二个包 `dev-20261020-102220-0752537e-dirty` 项目启动正常；专项脚本遗漏 resume_dispatch，领取被正确拒绝为 dispatch_paused，未创建候选。已修脚本并停服，使用新包重跑，不 reset 失败现场。
- 第三个包 `dev-20261020-102416-0752537e-dirty` 已验证原生工具在管理服务离线时完成提交、同一原 turn 的 completed/正式 final，以及 Runner B 补收原 final 后沿原 session 继续到发布报告。真实发布失败：本仓本机 `publish.ps1` 覆盖 Runner 注入的认证 helper/HTTP header，触发 MXC 内启动外部 credential helper 的拒绝；PR 未创建，Runner 正确 blocked，未记交付。已改为保留 Runner 作用域认证，以上发布接线定向回归通过，20.938s。旧包保持失败，`runner-cleanup` 返回 confirmed=true（`cleanup-original.log`），不篡改原候选或放宽沙箱。
- 第四个包 `dev-20261020-103407-0752537e-dirty` 已走通兼容接管、原正式 final 补收及真实发布，[PR #624](https://github.com/erranliu/SteerlineTest/pull/624) head 为离线提交 `1d9c52efabf864c4f9fe88b5beed5d915d6efe86`，仅变更 result.txt。专项驱动存在两个问题：首次 snapshot 与准备抢锁（无任务时沿相同冻结方案重新启动 launcher）；随后对 Git stdout 的比较被 PowerShell Out-String 转成 CRLF 而误报。原 Git blob 实际精确匹配（OID `cf9676d59334a554c66ecf63fc7c162f47b68ee5`，73 bytes，`git-blob-proof-corrected.json`）；已串行等待启动，并改为核对完整 blob 身份。该包依然记整体验证失败，未合并/交付；正式清理 confirmed=true，测试 PR 已关闭且保留历史，使用新包做干净复验。
- 第五个包 `dev-20261020-104424-0752537e-dirty` 的准备阶段先后暴露日志读取的 Windows sharing 异常及空输出调用；改为支持共享写者的轮询并显式处理空值。两次均在首次业务候选前失败，原始失败 summary 保留。最终以 `-ContinuePrepared` 验证相同冻结 live/ready 方案、服务 stopped、原两项 open 任务和零候选后继续，不重新生成或修改材料/合同；最终业务证据目录为 `evidence/runner-update-live-continue-20261020-024942`，`prepared-before-business.json` 保存准入实况。

本次入口是专项调试联验，不代表已登记的完整 L2/L3 场景或 Orca 原生矩阵通过；执行材料在首次启动前提交绑定。后续结果以本节增补和包内 `evidence/runner-update-live` 原始命令、公共快照与证明为准。

### 最终本机结果：SLC-A05 通过

最终命令为 `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-runner-update-live.ps1 -DebugPackage C:\SteerlineDebug\dev-20261020-104424-0752537e-dirty -RunnerCandidate C:\Project\SteerRunner\.evidence\sl-live-recovery-20261020\b\steerrunner.exe -AgentExecutable C:\Users\lmas1\.local\bin\node_modules\@openai\codex\node_modules\@openai\codex-win32-x64\vendor\x86_64-pc-windows-msvc\bin\codex.exe -ContinuePrepared`，退出码 **0**。

证据根目录：`C:\SteerlineDebug\dev-20261020-104424-0752537e-dirty\evidence\runner-update-live-continue-20261020-024942`。`summary.json` 为 passed；`cleanup.json` 和补充的 `postflight-verification.json` 均确认清理完成。冻结入口、原生只读 reader、构建 manifest、源码增量、原始命令及公共投影均在此目录。

| 核验项 | 实际事实 / 证据 |
| --- | --- |
| 固定真实环境 | Runner A/B 均来自上述干净 #35 主干，版本分别为 `0.2.0-recovery-live-a/b`；SHA 分别为 `33837c924dbba5af6f5ef50e3d6a270de19085858d00a2bcf6f364e535d86645` / `65a28dfd58be8ca9cdd5dc6db747ff174c4d291e69cb1d7dff20eb896f26d809`，公开 contract/resource digest 相同。Codex CLI 0.159.2、gpt-6-luna / medium，原 sandbox 权限保持 |
| 原生跨停服继续 | 原工具 PID 53788 在 10:51:39 创建，10:52:11 管理服务 not_running 时仍存活，10:53:12 完成原提交 `279f4e2b7aad0ea9d86d35e9d976694610ad3eb2`；`original-tool-offline.json` / `offline-implementation.json` |
| 原正式结果离线完成 | 原 session `01a15738-402f-7110-8b5e-97ba17a49164` / turn `01a15738-6591-7392-bcf8-109654df0d30` 在新服务启动前已 completed，具有完成时间及一个正式 final；`native-offline.json`。旧 A 的 resources_settled=false 原事实保留，另有兼容预检及 restart 证明 |
| B 接管及补收 | 原 Candidate `cf3a422df99240008b57bc2527a985d7` / Run `0333693b2badefd5daa0af4ace1e06c7`、session、完整 submit/input/build 事实保持，零 caller 业务 action、无需 resume；B 公开状态 SHA 正确。原 final 补收后沿同一 session 继续发布；`original-result-collected-b.json` / `published-b.json` / `service-b.json` |
| 真实 Git/PR | [测试 PR #625](https://github.com/erranliu/SteerlineTest/pull/625)，仅 result.txt；精确 head 为上述离线提交，blob 身份与指定两行及结尾 LF 精确相符。合并到本包专属 base，merge `392897611d27be29d24319486bbd15193a5b156c`；`git-blob-proof.json` / `pr-merged.json` |
| 唯一交付与后继 | Task 1 closed，唯一 Delivery 的 selected_workflow_run_id 为原 Candidate，PR 625/merge SHA 与真实 GitHub 一致；Task 2 open、dependency_ready=true、零候选；`delivered.json` / `postflight-verification.json` |
| 正式收尾 | cleanup exit 0、confirmed=true；最终 Runner not_running，B 自己的 last_shutdown.resources_settled=true（不同于旧 A 的未结算停服事实）；caller 停服后仍保留原候选与唯一交付。`cleanup.json` / `service-final.json` / `settled-caller-snapshot.json` |

业务领取至交付期间没有重复领取、替代 Run/session 或额外 caller submit/reply/resume；准备阶段失败及同冻结包的继续记录保留，未将失败现场改成通过。最终追加的 GitHub merged PR/Delivery 选中候选及 merge SHA 校验已对本次公开证据只读执行，通过证明保存在 `postflight-verification.json`，补充入口存为 `qualification-entry-postflight.ps1`。SLC-A01/A04 及发布接线的定向结果见本节前述记录；Windows PowerShell Parser 与 git diff --check 通过，不重复运行 full。当前本机兼容更新→补收→继续交付缺口已补验，Orca 原创建成功丢回执、其他原生路径和首次从旧 service-owned Job 升级的不可追溯窗口仍不在此次通过范围。未部署或改动安装/保留运行环境；源码修复及验收材料随本次 PR 提交。

### 提 PR 前主干同步

本次提交已 rebase 到 `412839e2`（主干 #361/#362），无冲突。联验入口为 `Assert-DeploymentRunnerBuild` 补传 staged SteerLine CLI，沿主干新增的 `verify-runner-takeover` Go 校验入口预检。

- `go test ./internal/runnerclient ./internal/runnerbinding ./internal/runnercontract -count=1 -timeout=120s` 通过，分别 5.838s / 0.853s / 0.816s。
- `go test ./tests/e2e/internal/driver ./cmd/steerline -run 'TestDebugPublicationReusesPR|Test.*Takeover' -count=1 -timeout=120s` 通过，分别 16.206s / 5.119s。
- Windows PowerShell 5.1 的部署测试入口与联验脚本 Parser、`git diff --check` 通过。

上述为 rebase 后的定向验证。真实联验仍固定到前述 `0752537e` 工作树增量及包内冻结入口，没有把旧证据改标为新基线，也未再次运行真实模型/PR。
