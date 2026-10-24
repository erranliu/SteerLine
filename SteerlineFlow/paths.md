# SteerLine 流程路径清单

本文是 SteerLine 自身的流程路径入口，定义文档、测试输出、隔离调试和运行状态的定位方式。目录所有者维护实际内容；这里维护路径、用途及隔离关系。产品安装和操作步骤仍见 [独立调试指南](../docs/runtime-install-debug.md)，测试选择仍见 [testing.md](testing.md)。

## 路径中的身份

| 标记 | 含义 |
| --- | --- |
| `<repo>` | 本次操作的源码 checkout/worktree 根目录；不能假定所有聊天使用同一路径。 |
| `<scope-id>` | 一项开发/验证工作的短目录名，例如 `wt-c751-paths`。不同 worktree 或同时推进的工作使用不同值，同一工作后续查证沿用原值。 |
| `<test-run-id>` | 一次实际测试调用的唯一标识，建议 UTC 时间加随机 UUID；补测生成新值，不覆盖原结果。 |
| `<release-id>` | 调试生成器返回的 `dev-yyyyMMdd-HHmmss-<短commit>[-dirty]`，其中时间为北京时间；从 stdout/manifest 读取，不自行猜测。 |
| `<test-id>` / `<scenario-id>` | 来自项目机器清单及 E2E manifest 的稳定 ID；不要用表格序号或包执行顺序代替。 |
| `<home>` | 显式选定的应用 home。调试时为新包自己的 home，不能沿用安装版或另一现场的 `STEERLINE_HOME`。 |

稳定的是目录层级、入口文件和 ID 的含义，每次执行的叶目录必须独立。源码版本、实际输出绝对路径、原测试命令和调试包身份应登记到本任务验收记录，不能只写“最近一次”或可被覆盖的 `latest`。

## Git 中维护的流程路径

下表均相对 `<repo>`。这些材料随 Git 固定版本读取；移动路径时同时调整导航、程序消费者及夹具。

| 路径 | 职责 / 读取者 |
| --- | --- |
| `AGENTS.md` | 当前项目规则及按需读取入口。 |
| `SteerlineProjectGuideDoc/` | SteerLine 统一提供的固定规范；本项目流程定制归 SteerlineFlow。 |
| `SteerlineFlow/README.md`、`SteerlineFlow/paths.md` | 流程内容索引与本路径清单。 |
| `SteerlineFlow/spec/`、`SteerlineFlow/plans/` | 独立 Spec、实现 Plan；执行时还须固定实际 commit。 |
| `SteerlineFlow/Roadmap/execution-map.md` | 已确认任务、依赖、执行及验收汇总。 |
| `SteerlineFlow/Roadmap/RoadmapGuide.md` | 本项目定制的 roadmap 维护规则。 |
| `SteerlineFlow/Roadmap/drafts/`、`SteerlineFlow/Roadmap/task-ids/` | 候选讨论与永久任务身份；身份分配不等于执行授权。 |
| `SteerlineFlow/testing.md`、`SteerlineFlow/testing/test-list.json` | 程序固定读取的测试规范和机器定义。与 Plan 一起作为系统测试安排的输入。 |
| `SteerlineFlow/testing/l1.md` 至 `l4.md` | 可读测试列表；ID、级别、环境及入口须与清单一致。 |
| `SteerlineFlow/testing/reports.md` | 报告格式、Git 历史导出和读取规则；不是某次原始报告。 |
| `SteerlineFlow/testing/records/<完整受测 SHA>.json` | 按受测提交保存的简洁历史，随 PR 合入；SteerLine 只读已核实主分支中的文件，删除文件可清理页面历史。 |
| `SteerlineFlow/testing/acceptance/` | 专属验收计划、已验/待验、历史结果摘要及原始证据定位。 |
| `SteerlineFlow/archive/` | 过期流程材料；固定历史证据按其原 commit/路径读取。 |
| `tests/e2e/suites.json`、`tests/e2e/testdata/` | E2E 执行登记与只读夹具来源，不保存某次运行现场。 |
| `debug-template/` | 新调试包的复制源，不能混入数据库、认证、EXE 或正在运行的现场。 |
| `docs/runtime-install-debug.md`、`docs/integration/` | 产品安装、独立调试和接入说明，继续维护于 docs。 |

任务编号还使用同仓各 worktree 共享的 Git ref `refs/steerline/roadmap-task-ids`，由 [编号工具](../scripts/roadmap_task_id.py)维护。通过 Git 查询 common dir/ref，不把某个 checkout 的 `.git/` 当作固定物理目录。

## 仓库外的工作与测试目录

人工开发、AI 验证和持续保留的调试证据采用以下显式路径约定，使用现有入口的 `-out`、`-OutDir`、`-EvidenceDir` 或 `-DebugRoot` 传入：

```text
C:\SteerlineDebug\<scope-id>\
├── test-results\
│   ├── L1\<test-run-id>\
│   ├── L2\<test-run-id>\
│   ├── L3\<test-run-id>\
│   └── L4\<test-run-id>\
├── diagnostics\<test-run-id>\        # 定向 Go、build-only 等非完整级别结果
└── packages\<release-id>\            # 由统一生成器创建的独立调试包
```

`scope-id` 是目录隔离标识，不自动成为 Task、Run、PR 或远端分支身份。整个根目录须在源码 Git/worktree 和安装目录之外，使用普通真实目录；不要用 junction、符号链接、Git alternates 或指向另一现场的硬链接压缩路径。输出子目录由实际执行入口创建，不复制旧报告充当新执行。

当前程序不传输出参数时仍使用下列默认值；本清单中的按级别归档通过显式参数实现，不宣称程序已自动采用新的外部布局。

| 入口 | 当前默认与显式定位 |
| --- | --- |
| `go run ./cmd/test-feedback` | 默认 `<系统临时目录>/steerline-test-feedback/<UTC纳秒时间>/`；`-out <绝对路径>` 精确指定本次根目录，放在 `--` 之前。实际执行接受不存在或已存在但为空的目录，拒绝非空目录。 |
| `scripts/test-l2.ps1`、`test-live-delivery.ps1`、`test-audited-workflow.ps1` | `-OutDir` 直接传给该次 test-feedback；使用对应级别的独立叶目录。 |
| `scripts/test-runtime-isolation.ps1` | 在 `-OutDir` 下再追加 `L3/` 或 `L4/`。给它一份独占的包装器调用目录，实际报告根为 `<OutDir>/<级别>/`，不要按其他脚本的语义推断。 |
| `scripts/test-web.ps1` | `-EvidenceDir` 指定独占目录；默认 `<系统临时目录>/steerline-web-check-<时间>/`，保留 `build.log`、`client-tests.log`、`server-tests.log` 及实际产生的浏览器日志。该脚本的服务器测试另由 test-feedback 返回其报告路径，不能仅凭 server-tests.log 推断全部原始产物都在外层目录。 |
| `scripts/new-debug-release.ps1` | 默认 `C:\SteerlineDebug/<release-id>/`；本约定显式传 `-DebugRoot C:\SteerlineDebug\<scope-id>\packages`，返回本次包的精确 root。 |

这些包装器有的另建原生进程代理目录，不能把代理日志当成业务报告。未支持输出参数的专项沿其现有真实输出路径登记，不能手工搬动活动现场来伪装成统一布局。

### 已登记 L1–L4 的固定报告结构

test-feedback 每次输出根统一以 `summary.json` 为结果入口；受管报告使用 `managed-<test-id>.json`。文件名中的 test-id 是稳定测试 ID，JSON 正文的 `id` 则是本次随机报告 ID，两者不能混用。未登记的独立脚本沿上一节及 L4 列表中的真实产物定位，不推断它们已有受管报告。报告字段、摘要及 Git 历史导出规则唯一维护于 [reports.md](testing/reports.md)。

| 范围 | 正常执行的稳定路径 |
| --- | --- |
| L1 | `summary.json`、`managed-l1.minimum.json`；各分组在 `<group-id>/test.jsonl`、`<group-id>/stderr.txt`。 |
| L2 | `summary.json`、`managed-smoke.delivery.json`、`build/frozen-binaries.json`、`scenarios/smoke.delivery/scenario.json`；包级原始日志见下方 E2E 树。 |
| L3 | `summary.json`；每个实际选择的 ID 对应一份 `managed-<test-id>.json` 和 `scenarios/<scenario-id>/scenario.json`。 |
| L4 E2E | 与 L3 相同；受控/真实环境及所选范围由原 summary 及冻结定义表达，不仅凭目录名判断。 |
| L4 Go 全集 | `summary.json`、`managed-go.full.json`；分组原日志为 `<group-id>/test.jsonl`、`<group-id>/stderr.txt`。 |
| 定向 Go / build-only | `summary.json`、`go-test.jsonl`、`go-test.stderr.txt`；不生成完整登记测试集的通过报告，归 diagnostics。 |
| `-summarize-log <原日志>` | 只读取已有 Go JSONL 并写 `summary.json`，其中 `source_json_log` 定位原日志；不执行测试、不复制原日志或生成受管报告，也不适用实际执行的非空目录检查。应显式选择独立的 diagnostics 叶目录以免覆盖旧摘要。 |

E2E 的附加结构如下。构建、场景和清理失败时可能只产生其中一部分，缺失不能补造或当作通过：

```text
<本次输出根>/
├── summary.json
├── managed-<test-id>.json
├── build/
│   ├── frozen-binaries.json
│   └── <实际构建的EXE、测试程序及构建日志>
├── e2e-<包序号>.jsonl
├── e2e-<包序号>.raw.log
├── e2e-<包序号>.stderr.txt
└── scenarios/<scenario-id>/
    ├── scenario.json
    ├── resources.json                 # 实际登记资源时产生
    └── <原始命令、断言、停止与清理证据>
```

分组 ID、包序号及附加日志不作为测试身份。读 summary 中的实际选择、分组日志路径和场景证据，不硬编码 `e2e-1` 或某个分组执行顺序。`test-feedback record -report-dir <本次报告绝对目录>` 将完整且固定干净源码的实际报告导出到 Git 内的 `SteerlineFlow/testing/records/<完整受测 SHA>.json`；同一提交的多次执行合并到同一文件，不复制原始日志。历史没有数据库登记或 registration.json 回执，具体格式与边界见[报告约定](testing/reports.md)。前置尚未通过或没有可执行项时，也不能要求凭空产生完整报告集合。

### 显式路径调用示例

下面仅设置本次输出，按需要选取对应命令，不是要求每次全跑 L1–L4。`scope-id` 换成本工作区的短名：

```powershell
$slScopeRoot = 'C:\SteerlineDebug\wt-c751-paths'
function New-SteerLineTestOutput([string]$Level) {
    $slRunId = [DateTime]::UtcNow.ToString('yyyyMMddTHHmmssfffZ') + '-' + [guid]::NewGuid().ToString('N')
    Join-Path (Join-Path (Join-Path $slScopeRoot 'test-results') $Level) $slRunId
}

$slL1Out = New-SteerLineTestOutput 'L1'
go run ./cmd/test-feedback -level=L1 -out $slL1Out
$slL2Out = New-SteerLineTestOutput 'L2'
go run ./cmd/test-feedback -level=L2 -out $slL2Out
$slL3Out = New-SteerLineTestOutput 'L3'
go run ./cmd/test-feedback -level=L3 -suite=recovery,runtime -env=controlled -out $slL3Out
$slL4Out = New-SteerLineTestOutput 'L4'
go run ./cmd/test-feedback -level=L4 -suite=runtime -env=controlled -scenario=runtime.path-isolation -out $slL4Out
```

候选测试进度统一写入 Runner 注入的 `STEERLINE_PROGRESS_FILE`，实际位置为原执行工作区 `.steerrunner/progress/<Run ID>/progress.bin`；入口只使用注入路径，不自行拼接。旁置 `.lock` 与原子替换临时文件只协调写者，整棵 progress 子树由 Runner 放入 Git info/exclude。Runner 只转发此文件，SteerLine 不再收集候选 managed 产物；原始报告继续使用正式 `-out` 独立目录，Git 历史沿报告导出机制。

## 可并行的独立调试包

调试生成器的复制源仍为 Git 内 `debug-template/`，实际运行目录必须在 Git 外。使用 `-DebugRoot <scope根>/packages` 生成，再读取返回的 root/manifest；启动、停止、重置的详细流程归 [调试指南](../docs/runtime-install-debug.md)。

```text
<scope根>/packages/<release-id>/
├── manifest.json                      # 源码/构建摘要、生成结果与包身份
├── scheme.json                        # 首次准备开始即冻结方案，ready 标记准备完成
├── bin/                               # 本包业务、Web、helper及固定Runner
├── home/                              # 本包独立应用home与运行配置
├── projects/SteerlineTest/             # 本包独立Git clone，不借用真实演示项目
├── projects/SteerlineTestB/            # 仅双项目包
├── seed/、provider/、provider-b/、remotes/
├── evidence/、logs/                    # 本包执行和准备/验证原始证据
└── start.bat、stop.bat、reset-SteerlineTest.bat
```

不同工作使用不同 scope 根和包，每包独立 home、项目 clone/common dir、数据库、provider、端口及清理所有权。同一业务 Git common dir 不能因换 home 就假定能被两个应用同时接管；使用各包独立 clone。Web 默认探测空闲端口，实际值读包内 `home/web.json`；显式指定时，各并行包不能占用同一地址。

源码同一 checkout 的 `web/node_modules/`、`web/server/dist/` 和 `bin/` 是共享可写构建位置，因此同一源码目录的包生成、Web 安装/构建及相关 L4 场景须串行。生成结束、构建输出已复制到独立包后，才按资源约束并行运行；独立输出目录本身不证明并行安全。

生成器 release-id 精确到秒，重复包路径会拒绝。不同 scope 中同秒生成的 release-id 仍可能相同，而 live 的 GitHub base 默认取 `codex/debug/<release-id>/base`；并行真实演示前还须确认 manifest 中实际 base_branch 不冲突，双项目第二分支另带 `-b`。发现重复时重新生成独立包，不手改已初始化包的身份或复用原远端资源。

## 应用 home、Runner 与临时现场

| 路径 | 归属及定位规则 |
| --- | --- |
| `<home>/etc/*.json`、`<home>/execution.json`、`<home>/web.json` | 项目列表、固定执行配置及 Web 地址；调试包 home 独立。 |
| `<home>/state/<project-state-key>/steerline.db` | 项目业务状态。state-key 由业务 Git common dir 派生，由程序核验，不手填一个任意 run_dir。 |
| `<home>/application-api/`、`<home>/ui/web/`、`<home>/logs/` | 应用/Web 身份、控制与日志现场；经原应用入口观察和停止，不当作第二业务权威。 |
| `<home所在盘>:\slr\<home-key>/` | Windows 的应用级共享 Runner 短目录，按规范化 home 派生；同 home 多项目共用，换 home 派生新目录。非 Windows 为 `<系统临时目录>/slr/<home-key>/`。实际位置从运行信息读取，不手拼哈希或读取 Runner 私有库。 |
| `<系统临时目录>/steerline-e2e-*/<scenario-id>/` | E2E 可写 scratch，与长期报告根分开。资源清理未核实时执行器保留现场并在 summary 记录路径；不能只删报告而丢失现场定位。 |
| `<系统临时目录>/sl-test-*/` | Go 分组使用的独占短临时 home/TMP，执行器负责清理；原日志仍在本次输出根。 |
| `<TEMP>/steerline-native-test-<UUID>/` | 独立进程包装器使用环境变量 `TEMP`，保存 `test.log`、`result.json` 等代理输出；实际测试报告仍读其命令返回路径。 |

当前 Windows `HomeDir()` 无显式环境时的代码默认仍是 `C:\SteerlineRuntime`，该目录已作为旧安装保留；非 Windows 默认为 `<用户home>/.steerline`，用户 home 不可得时退到 `<系统临时目录>/steerline`。因此调试必须使用新包明确选择的 home，不能依赖默认值。日常 Windows 安装为 `C:\SteerlineInstall`，部署历史为 `<安装home>/deployments/<ID>/`；安装目录均不作为普通测试或并行调试的可写根。

## Roadmap 调度与恢复状态

roadmap-runner 的仓库外状态目录约定为 `<CODEX_HOME>/task-state/roadmap-runner/`，未设置时为 `~/.codex/task-state/roadmap-runner/`。具体 JSON 文件名由本轮 `--state` 绝对路径固定，建议用唯一 run ID 命名；这是技能调用方约定，`state.py` 不会代用户自动选择目录。

同一调度 run 的聊天/worktree 身份、固定文档版本、PR/审核和外部动作回执共用原状态文件，不为每个 worktree 复制一份。`.lock` 与临时写入文件同目录；异常锁必须先核实写入者，不能按超时自动清除。该状态目录不得位于任何已记录的仓库或 worktree 中。具体规则见[调度状态约定](../.agents/skills/roadmap-runner/references/coordination.md)。

## 结果留存与路径维护

原 summary、JSONL、managed 报告和必要清理证据保留在本次独占运行目录；Git 中保存任务专属验收摘要、原始路径/版本/命令定位，以及 `SteerlineFlow/testing/records/` 下按受测 SHA 生成的简洁测试历史；不保存完整原始日志。失败与补测分目录保存，资源未确认停止或证据未登记完成时保留原现场。临时目录可能被系统清理，需要长期追溯时先保存完整证据并在验收记录更新实际定位，不仅复制一份“通过”文字。

源码中的 `.evidence/` 是已忽略的本地临时证据区，不是本约定的仓库外并行调试根；新建持续现场使用上面的外部 scope。`SteerlineFlow/testing/acceptance/` 和 archive/ 也不放运行中的数据库、认证、服务锁或大体积原始现场。

路径调整应同步本清单、调用说明、实际读取者和相关夹具。当前实现定位：[测试执行器](../cmd/test-feedback/main.go)、[分组日志](../cmd/test-feedback/scoped.go)、[E2E 输出](../cmd/test-feedback/e2e.go)、[受管报告](../cmd/test-feedback/managed.go)、[调试生成器](../scripts/new-debug-release.ps1)、[Runner 短目录](../internal/runnerstatepath/path.go)、[应用 home](../internal/app/config.go)。本次清单整理只统一显式路径和定位规则，不迁移已有运行现场或修改服务状态。
