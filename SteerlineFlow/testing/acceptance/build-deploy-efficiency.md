# 构建与部署效率验证（2026-10-26）

用户授权实现依赖复用、分阶段计时，再实现前端与 Go 流水线并行；保留原部署核验和 ready 检查，不部署到安装环境。普通开发，不启动 roadmap。

受测源码为 `C:\Users\lmas1\.codex\worktrees\0e4a\SteerLine`，基于 `57623186b63c16be2a5748665e63d52ae615adbf` 的本次未提交修改。L1/L2 summary 的 environment 记录各次实际工作树状态；这些结果不导出为干净提交的产品测试历史。Windows PowerShell 5.1.26100.9711、Go 1.26.4、Node v24.13.0；保留现有 Go/npm 缓存。

## 实现边界

- `build.ps1` 默认将 Web 依赖检查/必要安装及前端构建放在独立 PowerShell Job，与 Runner（如选择）和 SteerLine 的串行 Go 构建重叠；Web EXE 在两条流水线成功结束后构建。`-SerialBuild` 可切换为串行。Go 失败也等待前端结束后才释放锁。
- Web 复用依据为 package/锁文件、Node/npm/平台、有效 npm 配置、npmrc、成功标记、隐藏安装锁摘要、包目录及构建工具。安装前作废标记，安装成功且输入未变、目录核对通过才记录；`-ForceWebInstall` 强制重装。配置仅在内存中哈希。未逐文件检测依赖内容损坏，手工损坏使用强制重装。
- 原源码输出路径防护提取到 `scripts/build-paths.ps1`。源码及显式 Runner 构建分别使用独占文件锁；锁内冻结独立 EXE 快照供部署复制。原接管预检、单次 prepare-deployment、安装后摘要校验、启动和全部项目 ready 检查保持。
- 构建与部署各自打印阶段结果并写 JSON；部署报告合并内部构建阶段，包内保存两份报告。失败阶段和跳过原因保留。并行阶段不能直接相加为总耗时。

## 自动验证

原始诊断根：`C:\SteerlineDebug\wt-0e4a-build-efficiency\diagnostics\20261026T034134619Z\`。下表路径相对此根，除明确列出的 L1/L2 外。所有假 EXE 只模拟命令，不启动后台业务服务；真实构建只写源码及诊断产物，没有执行真实安装部署。

| 验证 | 命令 / 证据 | 结果 |
| --- | --- | --- |
| 构建子进程夹具 | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-build-pipeline.ps1 -OutDir <诊断根>\fixtures-second`；`fixtures-second/result.json` 及各场景 output/timings | 21 项通过：首次/复用/串行/强制、package/锁/Node/npm/配置/npmrc 变化、缺工具/包、隐藏锁变化、坏标记、安装失败/重试、Web/Go/Runner 失败不产快照且不遗弃前端、源码/Runner 锁冲突。 |
| 部署连接夹具 | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-build-deploy-pipeline.ps1 -OutDir <诊断根>\deployment-fixtures-first`；其 `result.json` | 8 项通过：空 home、固定 Runner 复用及 profiles 保留、NoStart、构建/停机/启动/ready/接管预检失败。预检失败无 stop/install；构建/停机失败无 install。使用 result 中专属 `%TEMP%/steerline-deploy-pipeline-<UUID>` 假安装 home，未使用真实安装目录。 |
| 原部署停止围栏 | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File scripts/test-deployment-stop.ps1`；本次终端输出 exit 0 | 通过；单次 Go 准备调用、调用号/包绑定、pin 复用、接管门槛。 |
| 源码路径拒绝 | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File <诊断根>\test-paths.ps1`；`path-fixtures/result.json` | 通过；实际执行修改后的 build.ps1，bin junction、EXE hardlink 在构建前拒绝，独立受保护夹具摘要不变且未创建锁。 |
| PS5 语法与计时路径 | `powershell.exe -NoProfile -ExecutionPolicy Bypass -File <诊断根>\test-final-timing.ps1`；`timing-fixtures/result.json` | 通过；所有本次构建/部署/专项脚本经 PS5 Parser，正常/跳过阶段 JSON 正确，报告 hardlink/junction 拒绝且目标不变。 |
| 固定 L1（最终代码） | `go run ./cmd/test-feedback -level=L1 -out C:\SteerlineDebug\wt-0e4a-build-efficiency\test-results\L1\20261026T0405-final` | passed=true；准备 1.09s、执行 19.59s，summary/managed 为准。 |
| 固定 L2（最终代码） | `go run ./cmd/test-feedback -level=L2 -out C:\SteerlineDebug\wt-0e4a-build-efficiency\test-results\L2\20261026T0405-final` | passed=true；准备 30.84s、执行 154.62s；场景 cleanup_complete=true、cleanup_errors=null。 |
| 文档 | `python scripts/check_doc_links.py`、`git diff --check` | 最终链接检查 580 文件 / 3634 链接零错误，diff 检查通过。 |

首轮 L1（`...\test-results\L1\20261026T0355-build-efficiency`）和 L2（`...\test-results\L2\20261026T0358-build-efficiency`）均通过；首轮 L2 场景执行 221.64s。`fixtures-first` 在第二个场景因测试过度要求前端与 Runner 单阶段重叠而失败；实际与后端 Go 有重叠。修正断言为完整 Go 流水线后重跑得到上述 21 项通过；原失败目录保留，不算产品失败或通过。

## 真实构建与耗时

执行 `powershell.exe -NoProfile -ExecutionPolicy Bypass -File build.ps1 -Web`，按表追加选项及对应 `-TimingPath <诊断根>\<文件>`；每次 `.json` 对应同名 `.log`，均成功。涉及 Runner 的调用显式传 `-RunnerRepo <诊断根>\runner-source`，该仓由 `git clone --no-hardlinks --no-local C:\Project\SteerRunner <诊断根>\runner-source` 独立复制，不构建或改写原 Runner checkout。

| 报告 | 范围与选项 | 总耗时 |
| --- | --- | --- |
| `real-cold-serial.json` | SteerLine + Web，`-SerialBuild -ForceWebInstall` | 78.83s；其中依赖安装（含安装后验证）49.46s、前端 9.70s。 |
| `real-warm-serial.json` | 三 EXE，`-SerialBuild` | 54.18s；依赖安装跳过。 |
| `real-warm-parallel.json` | 三 EXE，默认并行 | 28.70s；依赖安装跳过。 |
| `real-warm-parallel-final.json` | 三 EXE，默认并行，另传 `-StageDirectory <repo>\bin\verification-snapshot` | 33.31s；快照 0.14s，依赖安装跳过，前端与 Go 时间区间重叠。修复了 PS5 隐藏 worker 的 UTF-8 解码，Vite 输出字符正常。 |
| `real-warm-serial-final.json` | 三 EXE，`-SerialBuild` | 33.56s；依赖安装跳过。 |

快照与随后串行重建的 SteerLine/Web EXE SHA256 一致。A 的实际收益明确：依赖不变时省去本次约 49 秒安装；B 的并行时序已验证，但两次复测总耗时均约 33 秒，不能把较早 54→29 秒全部归因于并行或承诺稳定百分比。Go/前端阶段本身也受缓存、磁盘和机器争用影响；首次安装仍需执行 npm ci。未测真实安装环境的端到端部署耗时，保留约定核验和 ready 等待。

## 提交 PR 前更新主干与复验

用户随后授权提 PR。分支 `codex/build-deploy-efficiency` 更新到主干 `80f98a37b55ebcf0376a355027dda0bb04f1bda5`（启动性能优化已合入），只解决验收 README 同位置新增导航的冲突，保留两项记录；另清理提取脚本末尾空白。受测干净代码提交为 `2fc450c507982054326e2e75c6831cf48aaa4fb2`，补测命令如下，两个 summary 的 worktree_status 均为 clean：

- `go run ./cmd/test-feedback -level=L1 -out C:\SteerlineDebug\wt-0e4a-build-efficiency\test-results\L1\pr-rebase-2fc450c5`：passed=true，准备 0.89s、执行 15.13s。
- `go run ./cmd/test-feedback -level=L2 -out C:\SteerlineDebug\wt-0e4a-build-efficiency\test-results\L2\pr-rebase-2fc450c5`：passed=true，准备 28.20s、执行 142.79s，场景 cleanup_complete=true、cleanup_errors=null。

两个报告均通过 `go run ./cmd/test-feedback record -report-dir <对应报告绝对目录>` 导出到 [该受测提交的 Git 测试历史](../records/2fc450c507982054326e2e75c6831cf48aaa4fb2.json)。`git diff origin/main...HEAD --check` 与导航检查通过；更新主干后的导航检查为 581 文件 / 3638 链接零错误。此次补验同样没有部署真实安装环境。
