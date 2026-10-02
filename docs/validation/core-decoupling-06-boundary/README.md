# #67 主包隔离守卫与测试归位：受限执行交接

记录日期：2026-10-02。依据 [Spec](../../spec/core-decoupling-06-boundary.spec.md)、[Plan](../../spec/core-decoupling-06-boundary.plan.md)、[共同约束](../../spec/core-decoupling-constraints.md)。

**状态：#67 未完成，不能合并；仍需完成 root 测试归位、quick 接入、完整映射与最终组合验收。第三中性实现的原输入/重启人工确认/唯一交付及资源结算闭环已在协调的完全访问环境定向通过，但不代替 #67 整体交付。**

## 协调接管的阶段记录

2026-10-02 22:57 UTC：执行 turn 因上下文窗口耗尽失败，平台已确认停止；协调在同一 f775 工作区接管，保留全部修改，没有替换执行聊天或增加并发写者。执行者未完成的迁移保存为本地阶段 commit `604cbc0`。本节之后的首次工作区身份、源码摘要和权限限制均为历史快照，不代表当前候选。

- 原生注册 agent 的冻结测试迁到应用包，使用真实目录；通用“两种显式 agent 均保持原固定选择”断言由主包单一中性实现承接。真实目录不注册的 agent 仍拒绝，不宣称扩大原生支持。两个定向命名用例通过。
- 第三实现先绑定最终隔离 provider，再通过公开入口建立任务权威，修复原 fixture 更改仓库身份后的 `authority_mismatch`。
- 创建会话回执补齐版本化原资源绑定封套，保存原 session、generation、process 和 target；重启观察验证封套与实际固定身份，保留 SQLite 一致性校验。缺失封套曾导致真实交付事务报 `contract_mismatch`，已修复 fixture。
- 合并后经公开任务详情查询核对关闭和权威交付，保留原 selected workflow、PR 和 merge commit、仅一次输入/答案、唯一 TaskDelivery 与仅一次资源结算断言。
- 定向报告：`C:/Users/lmas1/.codex/task-state/roadmap-runner/task67-third-original-binding/summary.json`，2 个 parent/subtest 全通过、0 fail/skip、complete，29.650 秒。测试基于 dirty `604cbc0` 加上述修复；不是完整 #67 的已审核 head。此前失败报告同目录 `task67-third-positive-parent604cbc0`、`task67-third-provider-bound`、`task67-third-task-detail`、`task67-third-delivery-diagnostic`、`task67-third-delivery-error` 保留，不豁免失败。

当前已获用户授权在 #67 真实交付后继续 #68–#72；下述首次限制“不启动 #68”表示前置未交付前不启动，不表示新的总范围止于 #67。

## 工作区和源码身份

- 实际工作区：`C:/Users/lmas1/.codex/worktrees/f775/SteerLine`。
- HEAD / `origin/main`：`a670e9d1510dfdfd04c1aeb032addd23a0d45941`，#66 合并提交；开始时工作区干净。
- detached HEAD；首次创建 `codex/67-core-boundary` 分支被 `C:/Project/SteerLine/.git/refs/heads/codex/67-core-boundary.lock: Permission denied` 阻断。没有再次操作 refs，没有提交、推送或 PR。
- 当前 Go 源码集合 SHA256：`f08ee646fc1b128c9dc214bfa73a4daeb9f74e77657c57e3f3c3126bd908912a`，465 个文件（全部现存项目 Go 文件及 go.mod/go.sum）。算法及逐文件摘要见 [source-manifest.json](source-manifest.json)。这是未提交源码摘要，不是 commit。
- 原始报告、迁移脚本和工作区 Go cache 保存在忽略目录 `bin/cd67/`、`bin/go-cache/`。报告对应上述 Go 源码；此后只追加验收文档，不改变 Go 内容。

## 已实施内容与所有者

`internal/coreboundary/check.go` 位于中心之外；无视构建标签扫描全部直属 Go 文件名/文本（包括 tests），解析全部项目生产源码图并加入 root 测试 import 边，检查中心/execution/contracts 经任意 helper 的传递禁用依赖以及具体实现反向中心依赖。另运行 Windows/Linux 的实际生产 `go list -deps -json`。直属目录无例外名单；版本未知、go list 失败和空扫描不能视作通过。

`TestCD67CoreBoundary` 在外层集成包。5 个负向 fixture 已实际证明大小写关键词、其他 OS 文件名/文本、额外构建标签下直接 app import、helper 传递 integration import、具体实现经 helper 反向 core import 均被拒绝。

6 个旧入口按实际职责迁移，见 [test-mapping.json](test-mapping.json)：

| 旧位置与入口 | 新所有者/位置 | 验证状态 |
| --- | --- | --- |
| activity / TestCodexNormalizer | codexworker/normalizer_cd67_test.go | 原断言 run/pass |
| activity / TestDetectNormalizer | steerlineintegration/normalizer_composition_test.go | 公共 normalizer 组合，原断言 run/pass |
| worker_local / TestWorkerTurnFindsPRBeforeFinalMessage | codexworker/outcome_cd67_test.go | 原 journal/PR 断言 run/pass |
| orca_worker_observation / TestWorkerOrcaWorktreeNameIncludesTaskNumberAndRunIdentity | orcaworker/identity_cd67_test.go | 原身份断言 run/pass |
| orca_worker_observation / TestFixedOrcaAgentTerminalRejectsReplacedIncarnation | orcaworker/identity_cd67_test.go | 原 incarnation 拒绝断言 run/pass；只去掉无业务用途的中心 Receipt DTO 包装 |
| worker_codex_live / TestCodexAppServerNativeQuestionLive | steerlineintegration/worker_codex_live_test.go | 保留完整断言及 STEERLINE_CODEX_APP_SERVER_LIVE opt-in；未运行 live |

中心 activity 文件中与存储 round-trip 有关的来源数据改为中性 fixture；原具体 normalizer/detection 断言已迁走。删除的直属文件仅 `worker_codex_live_test.go`，由外层完整承接。

app 新增 `NewServiceWithPorts`，注入现有目录、生命周期和 ConversationSourcePort；原默认入口仍构造原来源。没有向测试导出 Service 私有写者、Store/Runtime 替代接口，也没有修改任何 root 生产文件。

第三实现位于外层 `_test.go`：`additional_implementation_fixture_test.go` 和 `core_decoupling_67_additional_test.go`。已编译的候选通过 `LoadConfigWithDirectory`、正式 app 构造、公共 Claim/查询/答案入口接线；隔离 ledger 保存原输入/回执，重新构造目录/来源/服务后核对原身份和人工门槛。它只声明离线只读 fake 能力，工具查询返回 unknown、取消/替换会话明确拒绝，不产生原生权限认证。**闭环在 Git fixture 初始化阶段被阻断，所有后续业务断言未实际执行；这些代码还需要独立审查、实际调试和补全，不能视为 CD67-2 达标。尤其 TaskDelivery 的正向唯一交付与结算证明尚未交付。**

`TestCD67AdditionalImplementation` 已组合第三实现、既有 app-server 两阶段提交、terminal 原身份恢复以及 remote Protocol 代表入口。它尚未加入 quick 的默认覆盖，真实 remote Claim 拒绝仍需纳入同一最小入口，不能把协议 fake 当作权限认证。

## 实际最小验证

使用工作区内 `GOCACHE=.../bin/go-cache`。默认 Go cache 首次写入被拒绝后，改用已有可写工作区 cache；没有调整 sandbox 或权限。

定向诊断：

```powershell
go run ./cmd/test-feedback -out bin/cd67/initial-focused -- -p 1 -parallel 1 ./internal/steerlineintegration ./internal/steerline/codexworker ./internal/steerline/orcaworker -count=1 -run '^(TestCD67CoreBoundary|TestCD67AdditionalImplementation|TestCodexNormalizer|TestWorkerTurnFindsPRBeforeFinalMessage|TestWorkerOrcaWorktreeNameIncludesTaskNumberAndRunIdentity|TestFixedOrcaAgentTerminalRejectsReplacedIncarnation|TestDetectNormalizer)$/(negative_source_fixtures|neutral_fixed_input_restart_and_fresh_manual_answer)$'
```

实际报告 `bin/cd67/initial-focused/summary.json`：**complete=true、passed=false，3 包，12 pass / 2 fail / 0 skip（含 parent/subtest）**，5.390 秒。五个迁入的非 live 入口及负向守卫 parent/各样本均有实际 run/pass；第三实现 parent/子用例 fail。失败在隔离临时 Git push：

```text
sh.exe: *** fatal error - couldn't create signal pipe, Win32 error 5
fatal: Could not read from remote repository.
```

没有重试该 fixture，也没有改为忽略、修改断言、变更准入或伪造授权。两条已支持恢复和 remote Protocol 子用例在本定向选择中未运行，原先 #66 证据不认证本源码。

最终源码守卫诊断：

```powershell
go run ./cmd/test-feedback -out bin/cd67/boundary-current -- ./internal/steerlineintegration -count=1 -run '^TestCD67CoreBoundary$/^repository_all_tags_and_production_graphs$'
```

实际报告 `bin/cd67/boundary-current/summary.json`：**complete=true、passed=false，0 pass / 2 fail / 0 skip**，10.455 秒。检查覆盖 root 180 个 Go 文件、26 个项目源码包、Windows 232/Linux 221 个生产依赖。生产禁用依赖为零；当前 **51 个直属测试/fixture 仍有平台文本、4 条测试直接实现 import 越界**，守卫如实拒绝。见 [boundary-checks.json](boundary-checks.json)。开始时实际 baseline 盘点为 53 个平台测试/fixture 文件（早期进度消息中 56 的人工计数已由完整清点纠正）。

Windows 受影响 root/app/integration/两个实现包及 guard 已通过仅编译；Windows/Linux `go build ./cmd/steerline ./cmd/test-feedback` 通过；`git -c core.autocrlf=false diff --check` 通过。Linux 测试编译首次错误地使用 `-exec go`，包装器返回 unknown command，保留为失败诊断；正确纯编译使用 `GOOS=linux go test -c -o bin/cd67/linux-compile/`。未执行 Linux 测试体。

各包、parent/subtest 实际事件见 [test-hits.json](test-hits.json)，报告摘要及原始日志哈希见 [validation-results.json](validation-results.json)。这些定向检查不替代规定组合。由于 fixture 及 root 隔离仍失败，没有运行 quick/full 或重复平台矩阵，也没有调用真实产品 CLI、业务服务或任务数据库。

## 必须继续完成的 #67 范围

[root-test-inventory.json](root-test-inventory.json) 清点全部原平台直属文件、入口/辅助和 live/evidence 开关，并标记当前剩余；这是完整输入盘点，不是完成映射。剩余 51 个文件需要按业务/原生/跨层拆分，保留已编译原断言和 opt-in 工具/版本/证据要求。

优先拆除 `core_decoupling_66_test_helpers_test.go` 中复制的三个具体平台工厂；中心用一份中性 fake 覆盖 Decision/intent/receipt/原输入/人工门槛/唯一交付，原生命令与 codec 归实现，跨层与 live 经正式 app/公共端口归外层。`linked_workspace_fixture_test.go`、`process_fixture_test.go`、`requirements_fixture_test.go` 仍需完成所有者归位；不能靠统一替换名称或复制另一套工厂达标。

完全访问续做者须先核对本工作区和源摘要，审查并调试第三实现，完成剩余迁移、去重/旧到新映射、guard/第三实现/remote Claim 拒绝的 quick 接入；然后只运行一次稳定的规定最小组合，核对所有包与 parent/subtest run/pass/complete，构建产品并更新最终所有者文档。当前代码保留失败和未通过项，不能用 exit=0、正则零命中或生产依赖清零替代全部验收。

全部实现完成后，才建立分支、提交、推送和可合并 PR，交协调独立复审/亲自修复；执行者不自行合并。remote 原生权限证据缺口保持原支持限制。本轮没有修改持久版本、旧格式策略、准备记录拒绝或首个资源操作前的 Git 授权检查。
