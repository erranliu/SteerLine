# 新建 worktree 启动边界实施 Plan

对应 [Spec](../spec/fresh-worktree-startup.spec.md) 和[专项验收](../testing/acceptance/fresh-worktree-startup.md)。调查基线为 SteerLine `a589f618762c8913fcee8ae1858f777631d64b9d` 与 SteerRunner `1df8e26da6132ae75915f85931edc6cda7e85e12`。Runner 实施从主干新建独立 Git worktree。[SteerLine #442](https://github.com/erranliu/SteerLine/pull/442) 只用作任务 101 现场和测试夹具参考。

## 1. 固定跨仓接口与反例

- 从 SteerLine 的 `internal/taskrunner/base.go` 和各 Git 型执行请求入口列出固定 commit 来源。记录候选、outbox、Runner 创建意图、创建回执、业务步骤各自的证据。区分新建成功、创建未知和现有工作区恢复三个分支。
- 对照 Runner `internal/application/freeze.go`、`internal/execution/workspace/prepare.go`、Codex/Orca 创建、权限及 runtime 文件接线。列出成功路径的 Git 命令及调用者，明确哪些用于对象获取、资源绑定、内部 Workflow 或恢复，哪些是待移除的启动资格复查。
- 固定策略槽位的请求形状及空集合语义。优先复用能表达阶段引用且保留 Runner 默认步骤正文的现有字段。现有字段若无法区分调用方已解析的空集合与调用方未指定，可在开发 Workflow 请求中增加一个显式槽位选择，提交空集合也表示调用方已完成准备。字段只承载现有槽位，Runner 只检查结构并按现有阶段映射路由。独立提交方未启用该契约时沿现有公开行为。禁止用关联 ID、任务名或调用路径推断模式。

## 2. Runner 收敛新建成功路径

- 将创建所需的固定 Git 对象获取与目标分支祖先判断拆开。验证冷缓存、暖缓存、commit 仅由固定对象引用可达及对象不可用。私有 Run clone 中要能解析同一 commit，后续 Git 操作所需的目标 ref 按实际需要准备。目标 ref 刷新结果不再决定 SteerLine 固定基线是否有启动资格。
- 本机成功创建直接以创建事实构造回执，撤掉随后 `Verify(initial=true)` 和 `Inspect` 的重复启动检查，状态中清楚区分创建事实与实际扫描过的工作树状态。沿用 Orca 创建回执的身份和固定 commit 检查。保留权限 write roots、原文件通道的目录和实例绑定；本机首次文件初始化若仍经通用恢复校验进入 HEAD 或祖先探测，改用原新建回执所绑定的资源路径，保留恢复和结束采集分支的现有保护。
- 为显式提供策略槽位的新请求跳过项目策略文件读取和资格判断，复用现有阶段映射。维持 Workflow 原状态转移。文件通道保持普通尽力初始化和原诊断语义，禁用 #99 的必读字段、确认回执、启动门禁和相关重校验。保留创建失败、响应丢失、原资源对账及停止分支。

## 3. SteerLine 统一准备、就绪与派发

- 在候选准备层复用固定 commit 读取，核验任务依赖、Spec、Plan、项目环境和适用项目策略。执行预检在 Runner 调用前完成。策略槽位与固定 commit 一同冻结并明确进入请求。
- 从已生成测试安排冻结有序 ID、原始字节和 SHA-256，按固定清单、规则及 writer 预检。让候选「已就绪」与启动共用同一份冻结安排和能力判断。正式 outbox 保存原 `result_file.initial_content_base64` 字节及引用，保留独立进度通道、候选版本 CAS 与原请求对账。
- 对齐固定短指令、安排的读取定位和项目原测试协议的来源优先级。安排在此次使用中只读，进度及 writer 仍按原规则使用，禁止路径或正文兜底。组合启用策略引用、安排及进度时，按目标执行链路验证最终提示词的归属和既有预算，尤其是 Orca Codex 的原生包装。任何受影响的固定正文、接线、资产或测试按照两仓各自的提示词治理登记实际正文差异、前后字节数、原因及验证，提交独立审核。

## 4. 回归、任务 101 和交付

| 时机 | 入口 | 通过条件 |
| --- | --- | --- |
| Runner 定向回归，FWS-A03、A05 | `go test ./internal/execution/workspace ./internal/execution/adapters/codex ./internal/execution/adapters/orcatransport ./internal/application ./internal/contracts ./internal/resources -count=1` | 在真实 Git 夹具中验证成功路径的命令与回执，覆盖固定对象不可用、身份不符、创建未知和恢复。覆盖显式有槽位及空槽位时均无项目文件准入读取，未启用时符合原契约。 |
| SteerLine 定向回归，FWS-A01、A02、A04 | `go run ./cmd/test-feedback -- ./internal/app ./internal/taskrunner ./internal/storage/sqlite ./internal/runnerclient ./internal/prompts -count=1` | 就绪和启动使用同一字节，覆盖候选变更、原请求重读、旧回执、缺能力、超限、策略/规则/writer 错误、错误顺序及安排读取失败。 |
| 两仓固定构建受控验收，FWS-A01、A02、A04 | Runner 正式公开 CLI + 原生协议受控夹具，SteerLine 正式候选启动入口。把任务 101 用例纳入受控执行入口，另运行 `go run ./cmd/test-feedback -level=L3 -suite=testing -env=controlled -scenario testing.runtime-progress -out <绝对输出目录>` | 在没有旁置 JSON 的任务 101 基线上，用公开回执、独立 Git 检查和受控 Agent 读文件轨迹核对正向及传输故障。原始失败单独保留。 |
| 交付前原生及正式回归，FWS-A01、A03、A05 | SteerLine 正式 L1/L2，并在隔离本机 Codex 环境执行任务 101 的正式启动，命令随夹具固定。Runner 按测试指南使用 `local-codex.smoke`、`local-codex.audited`、`orca-local.smoke` 和 `orca-remote.smoke` | 任务 101 取得真实 Agent 读取证据。本机和受支持 Orca 组合分别证明从新建回执进入 Workflow，Runner 自主推进。缺原生配置的路径登记未验，受控结果只用于对应层级。 |

测试使用隔离 Git 来源、Runner state、worktree 及输出，按[路径清单](../paths.md)和[测试指南](../testing.md)将产品证据放入显式的 `C:\SteerlineDebug\<scope>\test-results` 路径。实际构建、命令、时间、证据、失败与清理写入[专项验收](../testing/acceptance/fresh-worktree-startup.md)。两仓的产品变更配对验收后再更新接入文档的实现状态及受支持的构建。
