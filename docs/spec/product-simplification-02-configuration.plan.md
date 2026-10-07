# Plan：固定原生沙箱与自动审批，移除附加需求

绑定[第二批Spec](product-simplification-02-configuration.spec.md)，实施基线`172cea6c6530b6693b742324a9c1d4f9aeb17b6d`，分支`codex/product-simplification-configuration`。第一批已合入。采用工作区中用户确定的单一默认方案，仅实施第二批。

## 实施与消费者

1. 项目输入删除execution_policy/execution_requirements，严格JSON在创建资源前拒绝旧字段（包括空/null）；全局Codex输入只保留executable/model/reasoning_effort，拒绝args/profile/windows_sandbox及审批覆盖。保留Git/PR argv、Orca shell/连接与多profile。
2. 删除backend的自由组合ExecutionPolicy、FilesystemPolicy与ExecutionRequirements/匹配值；删除执行目录Normalize/selection/lifecycle中的产品权限参数。各adapter自己生成原生设置。Windows local及Orca/Codex固定workspace-write、unelevated、on-request、auto_review、network allow，保留临时目录限制与精确候选/Git根，删除elevated/完全访问回退。非Windows/其他agent/远端沿用实际原生支持边界。
3. 删除附加需求评估、工具/凭据查找、认证命令、文件报告登记及快照/运行错误字段；保留必要版本检查、原生CapabilityReport/生命周期与会话采集、readiness、目标连接、固定资源身份与worker发布核验。消费者清单随实现更新，不按名称删除必要协议。
4. 冻结profile继续保存implementation envelope/显示/摘要；实际生成参数存入现有PolicyResources并核验。同步受影响codec/fixed-profile/数据库版本，拒绝旧开发数据并保留现场，不迁移、不双写。反馈/重启仅消费原绑定设置，配置修改影响新候选。
5. 更新示例、当前产品设计和fixtures；自查删除消费者、同口径行数和必要断言，提交推送并创建独立PR。本批不实施第三批资源/CLI精简。

## 测试计划

证据根目录`docs/validation/product-simplification-02-configuration/`，每次runner使用新子目录；失败证据保留。现有测试入口见[测试指南](../testing.md)及[E2E约定](../../tests/e2e/README.md)。

| 级别/组与场景 | 环境与命令 | 时点 | 通过条件 | 证据 |
| --- | --- | --- | --- | --- |
| 定向PS2-01/02/03 | Windows临时home，`go run ./cmd/test-feedback -out=<新目录> -- ./internal/app ./internal/execution/adapters/codex ./internal/execution/adapters/orca ./internal/product/query ./cmd/steerline -run 'Test(PS2|Policy|Profile|GlobalExecution|Config|Terminal|NativeTask|ExecutionPolicy|Managed|Launch|Publication)' -count=1` | 配置与adapter完成后 | 拒绝所有旧输入，固定参数/当前格式冻结及发布准入正确，审核失败沿原路径 | `targeted/summary.json`与日志 |
| 定向PS2-02/03/04 | 临时SQLite/可控worker，`go run ./cmd/test-feedback -out=<新目录> -- ./internal/execution ./internal/storage/sqlite ./tests/integration -run 'Test(PS2|RestartPreflight|Fixed|WorkerFeedback|ExecutionProfiles|ExecutionRegistry|Remote|Supervised|Candidate|Capability|Policy|Readiness)' -count=1` | 生命周期接通后 | 原身份/摘要/根/readiness保持，远端缺证仍拒绝，其他agent不误注入参数 | `wiring/summary.json`与日志 |
| L1固定quick | Windows，`go run ./cmd/test-feedback -level=L1 -out=<新目录>` | 定向通过后 | passed/complete=true，无退役测试零匹配，生产所有权边界保持 | `l1/summary.json`与日志 |
| 原生PS2-05 | Windows隔离候选、临时本机Git远端，使用当前adapter构建的启动设置分别启动local/Orca-Codex | 当前构建与定向通过后 | 文件修改、一条构建/测试、commit/push；非升级兄弟写入拒绝；各一次有界原生审核，真实决策记录；Orca恢复后设置保持；记录UAC情况 | `native-local/`、`native-orca/`原始通知、启动参数、回执与摘要 |
| 静态/格式 | `git diff --check`、`go vet ./internal/app ./internal/execution/... ./internal/product/query ./cmd/steerline`、`go build -o bin/steerline-configuration.exe ./cmd/steerline`、`python scripts/check_doc_links.py` | PR前 | 构建/格式/文档通过，退役路径无消费者 | 验收README与消费者清单 |

新增PS2纯规则场景和最小原生fixture，复用现有adapter/protocol/testenv；不创建测试专用生产启动能力。必要时opt-in test标签隔离原生模型检查，默认L1没有skip伪通过。PS2-05不使用管理员初始化/全局配置改写/ACL自动修复；审核拒绝记录实际原因，不提升或改完全访问。原生场景必须真实完成，缺少环境如实记录缺口，不能用纯参数断言冒充。

不选择Full或完整L2/L3/live：发布/唯一交付本批不变；所需原生审核与启动接线用PS2-05最小场景，远端与其他agent只做受影响路径定向。不新增跨平台等价认证或远端平台能力。

## 格式版本

实施时记录实际版本：fixed profile、Codex profile/resource codec和SQLite/schema仅因其字段或原生契约变化调整；项目JSON用严格字段拒绝，工作流身份协议和未变更原生/远端协议不升版。具体版本与拒绝验证填入验收记录。

实施格式已固定为SQLite15、checkpoint19、FixedExecutionProfile7、builtin-execution-v3、Codex profile/resource codec2；全局格式2、工作流/远端协议和未变更的资源格式保持。原生resource codec复查生成的固定沙箱/审批参数，变更/缺失/重复设置拒绝，复用原完整摘要与身份，不建立新证据系统。清理stop的必要检查使用原生adapter与原绑定/退出回执，不再消费退役文件认证报告。

## 已执行最小原生场景

新fixture `TestPS2NativeDeveloperAndAutomaticReview` 使用 `windows && ps2native` 标签，默认L1没有native skip。命令为设置 `STEERLINE_PS2_HOST=local|orca`、`STEERLINE_PS2_CODEX=<已核对的0.159.2直接可执行文件>`、`STEERLINE_PS2_OUTPUT=<新证据目录>` 后，`go test -tags=ps2native ./internal/app -run '^TestPS2NativeDeveloperAndAutomaticReview$' -count=1 -json`。环境为独立Git/候选/Codex home，仅复制认证（退出删除），临时Git HTTP接收端；Orca经实际CLI创建/观察原worktree和terminal。任务普通脚本完成文件、项目测试、commit/push并捕获未经升级的兄弟写入拒绝，再发一次有界native自动审核请求；审核拒绝按事实记录，不重试或改权限。Orca从原serialized profile/resource恢复，重新通过production recovery port核验原live terminal。核验和CLI/目录信任夹具失败如实保留，不将其冒充通过。

local71秒、Orca66秒通过，真实tool/context/review/最终事实在[验收记录](../acceptance/product-simplification-02-configuration/README.md)及selected native事件中；这不是完整业务PR/SQLite重启链验收，当前格式和控制回放由定向/L1覆盖。UAC“未观察”只限定本次输出与没有请求管理员初始化/提升启动。当前directed源码、native resource和remote回归通过；固定L1最终重跑单独记录。

最终固定L1 `l1-release/summary.json` passed/complete均true，12组全过；原生、配置/资源/远端接线定向通过。未修改runner包期限、用例覆盖或用失败/旧日志冒充通过；初次远端夹具编译失败与第二次包期限失败保留。构建、受影响go vet、原生tag编译及文档链接检查通过。生产实现同口径净减1211行，测试净减1234行，退役与保留消费者见验收记录。

## 主分支同步及最小补验

同步ae32f300验证资料布局和runner默认临时输出；Spec/行为边界保持。同步后定向检查使用固定历史快照验证旧格式拒绝、Orca当前testdata/runner输出选择，产品构建、go vet和文档链接。命令：`go test ./tests/integration -run '^TestProjectPolicyNativeReportsRetainOriginalSnapshotDigests$' -count=1`，`go run ./cmd/test-feedback -out=<新目录> -- ./internal/execution/adapters/orca ./cmd/test-feedback -run 'Test(Platform|NativeReadiness|Scoped|DefaultOutput|PS2|CandidateSession)' -count=1`；环境Windows、原生/模型不开启，时点同步后，通过条件非零pass/complete=true，证据post-sync/summary.json。不重跑L1/live：产品逻辑未因该同步变更，已有通过报告保留原采样来源。提交仅精选当前摘要和native公开事件，完整日志按主分支约定留本地；不新增全量产物目录。

## 最终主分支集成

PR [#236](https://github.com/erranliu/SteerLine/pull/236) 集成主分支6c16b8dc的控制台改进、文档归并与L1精简/并行实现。README沿主分支聚合入口，固定L1按其新范围保留，仅将退役权限/需求用例替换为PS2必要能力、原冻结身份/根与固定原生方案场景；不恢复已移出L1的长恢复/候选链。新控制台源代码及无关空白/import布局保持主分支原样。

最小补验计划：Windows、无模型，`go test ./cmd/test-feedback ./internal/console -count=1`（runner并行/覆盖守卫、三窗与输入交互）、`go run ./cmd/test-feedback -level=L1 -out=<新目录>`（当前12组passed/complete），`go vet ./internal/app ./internal/execution/... ./internal/product/query ./cmd/steerline`、产品构建、文档链接和diff检查；在合入主分支后执行，证据l1-main-sync/summary.json及验收摘要。不重复原生/长恢复定向：对应生产逻辑未因本次冲突解决改动，原通过记录保持其源码来源。行数按最新主分支6c16b8dc重新同口径对比，排除其他PR收益。

最终补验全部通过：runner12.008秒、console1.285秒；最新L1 12组passed/complete=true，准备0.608秒/执行113.919秒。产品构建、受影响go vet、230篇文档1410条本地导航及diff检查通过。精选当前报告见l1-main-sync/summary.json；生产净减1211行、测试净减1234行（基线6c16b8dc），未计算主分支其他改动收益。

## 自动验收覆盖与结果登记

任务专属记录：[验收文档](../acceptance/product-simplification-02-configuration/README.md)。原测试计划已有环境/命令/时点/通过条件与证据，以下补齐验收项映射；原结果仅绑定原源码，不因改 Plan 重跑或重标通过。

| 自动验收项 | 对应现有入口 / 覆盖要求 |
| --- | --- |
| PS2-01–04 | 现有配置/冻结/必要能力/远端拒绝定向与 L1，覆盖每项原身份和格式断言 |
| PS2-05 | 现有 ps2native TestPS2NativeDeveloperAndAutomaticReview，分别核对 local/Orca 的实际工具、审核/恢复/资源与推送事实，不用参数断言替代 |

执行安排沿本 Plan 的原测试表：实际变更触及对应验收项时运行其最小入口；未变范围先核对原证据，不默认重跑 Full/真实模型/PR 链。发现必要断言或能力缺口时先补最小场景再验收，零匹配/skip/编译不算通过。新原始报告写 runner 默认临时目录或新的空临时目录，具体命令/源码/摘要/日期、逐项结果与证据定位登记到上述任务专属记录并同步 execution-map，Plan 不作实时状态表。

原文中的历史命令/目录保持取证含义；新执行不再写 docs/validation。自动范围无需额外人工验收门槛；原生场景需人准备环境时预约窗口，不把自动失败/缺权限归类为后置人工。不新增验证矩阵或改原通过条件，本次 Plan 修订只做文档静态检查。
