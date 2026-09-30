# #59 项目执行权限策略验收

基线：`601279ba77fb7b2e69199c8155f4a5db6d42f16c`，包含已独立复审合并的 PR171。开工和提交前均 fetch origin/main 并核对硬前置。范围止于 #59；未实施 #60 发布责任，未部署或合并本 PR。

## 最终所有者

- execution/policy.go 拥有纯项目策略及校验，contracts 的同一 FixedExecutionProfile v2 / ExecutionRecord 引用它。contracts → execution，无反向依赖。
- 中心 config/execution_profiles 受理项目策略与全局执行方案；execution_snapshot 组合版本/来源/完整摘要和资源阶段证据，native_claim_command、worker 附件事务仍是唯一持久写者。
- codexworker 拥有策略到 native CLI/thread/turn 的唯一权限编译、已核实版本和安全参数检查。workspace 仍唯一解析 Git 的 objects/refs/logs/linked admin 四个写根。orcaworker 只包装完整命令，解释原 terminal/当前 screen/工具事实。
- orca_remote_worker.CheckPolicy 拥有远程策略拒绝：诊断包含请求策略、目标和版本观察范围。原协议、中心/远端身份、消息和对账链路保留；现有 worker-start 无权限落实证据，不能据 mock 启动放行或降级为本机。
- query.Service / NewQueryService / projectReadRecords 沿用同事务 ProjectView；checkpoint 保存受理要求，Domain 保存固定策略/资源，控制台显示冻结来源、版本、写根与设置/工具核验范围。未新增 SQL 查询或存储类型断言。

## 配置与冻结

项目文件必须显式 execution_policy，包括必填 bool git_metadata_write。删除旧顶层 sandbox/windows_sandbox JSON 读取；Windows 实现只读 codex.windows_sandbox。native Profile、权限/目录/model/effort/配置/子命令和未知 args 在外部查询之前拒绝；本机 Orca 不能通过 args 改投 remote。允许的独立展示参数仍为 --no-alt-screen、--no-daemon、--search。

local workspace/Git 分散根保持默认 elevated 和 unelevated 拒绝；Orca 保留显式 elevated/unelevated。local none + network deny 映射到 readOnly，可显式 unelevated。其余缺证组合明确拒绝，host 写入与 network deny 不会映射成全权限。model/effort 仍独立，local high 默认和 Orca 省略语义保留。

领取先核对已受理配置/完整方案摘要、静态选项和固定短命 Codex 版本查询，冻结策略、来源和版本。资源阶段在既有意图/附件事务记录真实 workspace、Git 根、native executable、版本与设置。准备、后续输入、问题回复和恢复都使用原快照；摘要、资源、实际设置/版本不一致或已绑定资源快照缺失时阻止输入，不回填。当前 checkpoint 格式为 11、固定快照格式为 2；旧格式拒绝并保留原现场，不迁移或双写。#60 应串行扩展此实际快照及组合摘要，保留全部版本/策略/资源检查。

## PO4 原生证据

两个验收均使用新策略、新 Execution 身份、新 linked worktree 和新 native session。调用本轮生产编译器及资源绑定，运行同一脚本：读取宿主 hosts，工作区写/读/删临时文件，git add/reset 索引，创建临时 ref/reflog。真实 native CommandExecution 退出码为 0；不是口头报告或旧 readiness 收据。

| 路径 | 新 Execution | 原生身份 | 设置 |
| --- | --- | --- | --- |
| local | `43bde523ffca2d7a1aeccbbbe796ead7` | thread `01a0f361-d5e1-71c3-a10f-a01ed1c52130`，turn `01a0f361-d6ea-7e12-b9bf-155253e3dbf9`，进程见 [原始证据](local-native.json) | Windows elevated，approval never，workspace/Git 写根，network allow，隐式临时根排除 |
| 本机 Orca | `c2b8fa1bcbe909f149df0229255723cb` | terminal `term_888b08e2-71ce-40c0-8df6-59428257e96d`，incarnation `d6f94638-0979-4438-8523-b4a0dbd42412`，session `01a0f370-e0b0-7850-9d0d-aadbabfd110c` | Windows unelevated，其余权限同 local，见 [原始证据](orca-native.json) |

[native schema 依据](native-schema-basis.json)记录实际 Codex 0.159.2 导出的 SandboxPolicy 及完整生成文件 SHA-256；配置键来自该版本的[官方 schema](https://raw.githubusercontent.com/openai/codex/rust-v0.159.2/codex-rs/core/config.schema.json)。版本不匹配时拒绝。原始 argv、原生设置与真实工具结果分开保留，成功编译不作为环境 Ready。

local 原生 thread/settings 明确 networkAccess=true、approvalPolicy=never、两个 temp 排除项及四个 Git 根；cwd 是固定工作区。Orca 的原 session_meta/turn_context 明确 root 读取、只有 workspace 和四个 Git 根可写、network enabled 和 never；未固定模型仍记录实际观察到的 gpt-6.1-sol/high。

Orca 复用的是既有隔离验收仓库的环境，创建了本轮独立 worktree/会话。未复用或改写原 ProfileDigest、probe nonce、收据或旧摘要，也未重跑完整 readiness。采集阶段曾查错 native home，随后从事先保存的本轮快照收集原 terminal/incarnation 的实际日志；没有重发输入或创建替代会话。

原生检查发现初次 local 的临时 ref 删除需要写未授予的 packed-refs.lock，保留了[拒绝证据](local-native-initial-rejection.json)。未扩大写根：后续工具仍只创建 ref，宿主精确删除本轮 ref。Git 分支/ref 删除若需要这个锁，继续属于宿主清理范围。另一初次 Orca 新克隆停在 trust 菜单，未计为工具证明；这些尝试的 terminal 均已精确关闭。

成功验收的脚本、结果文件、临时数据、索引、临时 ref 和对应新 blob 均精确清理，native agent 和自动创建的空 shell 已关闭。失败尝试的工作区文件、索引与 ref 已清理；失败现场日志和少量不可达临时对象保留，强制删除初次只读 Git 对象的命令被自动审批以 blocked by policy 拒绝。未扩大清理范围，也未删除/改写旧验收材料。生成 schema/CLI 发现的临时文件已清理。无业务 commit/push、PR 发布或生产部署。

## 自审与实际验证

- Plan 两个纯包执行通过，并补运行 orcaworker / orca_remote_worker。新增策略枚举/必填、local/Orca 支持与拒绝、长短/等号/分离 args 冲突、未知版本、无 model/独立 effort 核验。
- Plan 六项中心回归全部命中通过；两项 console 回归全部命中通过。额外覆盖缺失/旧项目与全局格式、静态冲突先于 executable/repository 查询、当前配置不改原策略、损坏摘要/设置/版本拒绝、丢失绑定资源不回填。
- TestWorkerEnvironmentER1ProbeAndSettlementGate 五个子例全部通过，保留已修复的真实 fixture 分支身份、准备/业务分流、原收据/工具/结算及失败门槛。新增要求读取 --screen 的回归；不把 stream、trust 菜单或 parent shell 当作新输入入口。
- TestFiveGlobalLunaProfilesLoadAndFreeze 与本机/remote 配置库存用例通过；remote 方案可加载、原协议保存，但缺请求策略证据时不可领取。
- 原生 local 工具检查通过；原生 Orca 工具已成功退出，原快照采集用例通过。TestProjectPolicyNativeReportsRetainOriginalSnapshotDigests 核对两个报告与最终实现的原摘要完全一致。
- go list -deps 验证 execution/codexworker/remote 传递依赖，无中心、query 或 SQL 反向依赖。go test ./... -run '^$' 全仓编译/链接通过，未运行全量测试。git diff --check 通过。

远程双机权限认证仍缺原协议 producer 的落实证据；mock/Service 回归不会被列为真机支持。未提前交付 #60/#61，也未引入旧版兼容或 fallback。
