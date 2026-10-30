# 旧 Runner Check 接入退役

2026-10-29，已实施并验收。[Plan](../plans/legacy-environment-check-retirement.plan.md) · [验收](../testing/acceptance/legacy-environment-check-retirement.md)。普通开发，不进入 Roadmap；本轮按审计后方案实施，不部署安装环境。

## 1. 问题与目标

本任务删除 SteerLine 对 Runner 旧 `environment-check start/inspect/stop` 能力的完整接入。它与当前项目环境准备是两套业务，不能按名称合并或互相作为备用路径。

本 Spec 经本轮实施与验收，替代旧[环境检查 Spec](environment-check.spec.md)与[旧 Plan](../plans/environment-check.plan.md)中针对 Runner Check 接入的现行要求；原版本方案和证据保留作历史追溯。

开工时未提交的[项目 × 环境准备](project-environment-readiness.spec.md)核心已通过普通 Run 执行项目材料并读取 result，但外围存在旧依赖：项目启动核对旧 Check 绑定，开始准备时读取旧 Check 占用，接管/停机/reset 枚举旧 Check，HTTP/Web 暴露旧入口。因此本轮整体删除这些消费者，不能只删 Runner 命令或隐藏页面。

目标是保留现有项目准备与派发门禁，删除旧 Check 的行为、状态、契约与资源消费者，双方按退役后的固定 Runner 契约协作。以当前工作树为实施基线，不能重置、覆盖或回退此前未提交的 readiness、提示词、测试与取证修改。

## 2. 职责与保留范围

项目提供版本化环境文档和脚本；SteerLine 固定源码、执行方案和材料引用，以固定提示词提交普通 Run；Agent 按文档调用脚本并写 result；Runner 提供普通执行及通用结果通道；SteerLine 校验原结果、维护项目 × 环境资格和派发门禁。

必须保留：

- `internal/environmentadaptation`、当前活动引用与准备依据、原项目环境 HTTP/命令、正式准备页面和所有派发入口的资格复核。
- 普通 Run 的提交、身份、原请求对账、观察、progress/result、停止、接管、重启和结算。result 仍是最终业务报告来源，progress 只表达执行进度，不授予资格。
- 固定提示词与同版本项目文档/脚本标准，原 checks/L2/PR 报告内容、轮次/期限、未知结果及原资源收尾规则。本任务不改变准备成功标准或重新解释历史失败。
- 实际仍被其他功能使用的配置、权限、认证、进程、工作区及存储工具。目录名、`Check` 单词、checkpoint 或报告中的 `checks` 不构成删除依据。

三种事实分别说明：有效检测完成且要求不满足；执行/检测/结果读取未完成而环境能力未验证；完整正向验收通过。前两种都不授予 ready，但不能把检测流程错误写成项目缺依赖。此处澄清职责，不新增结果分类平台或持久状态机。

## 3. 删除范围

| 所属层 | 退役内容与已定位入口 | 收口要求 |
| --- | --- | --- |
| 业务服务与模型 | `internal/environmentcheck/`、`runnerstate/environment_check.go`、旧 app port | 删除 start/stop/detail/catalog、旧请求与快照处理、后台观察及旧资源 owner；不把实现搬进新项目准备服务 |
| Runner caller 契约 | `runnerclient/environment_check.go`、`runnercontract/environment_check.go`、catalog/client 分发及旧固定资产 | 不再读取/校验/要求旧能力、schema、fixture 或发旧命令，不保留空壳 DTO/字段/接口 |
| 启动与占用 | `project_runtime.go`、`client.go`、`environment_admission.go`、`state_preflight.go`、`runner_build_binding.go` | 移除旧 Check 绑定与占用读取；保留新环境活动和原候选的占用、身份及准备门禁 |
| 接管及停机 | `runner_takeover.go`、`application_verify_stop.go`、`runnerbinding/`、`runnerclient/verify_stopped.go`、service binding | 删除旧 Check 枚举、匹配与 pending stop 分支；原 Run、生成/环境活动及服务实例的核验仍完整，不能因删除 Check 一并放宽 |
| 持久化 | SQLite `environment_checks`、`environment_check_requests` 及读写端口 | 删除表定义、读写方法、索引、计数和 reset 清单引用；不留空表和旧 payload 读取 |
| 产品边界 | `product/api`、`client`、`httpapi`、`httpcontract` 的旧路由、命令意图和权限注册 | 删除旧 `/environment-checks` 及旧 start/stop 授权；原项目认证、CSRF、实例和版本守卫保持；未知入口沿现有未知路由/命令行为拒绝，不加重定向或兼容空响应 |
| Web | 旧 `environment-checks.tsx`、专用 types、Query、失效分支、路由/懒加载/导航和提示 | 删除旧页面及“基础环境检查与历史”链接；保留 `project-environment.tsx`、`use-readiness.tsx`、任务与派发 blocker。不得整目录删除，不为纯命名做大规模搬迁 |
| 测试与工具 | 旧 owner/HTTP/Web 用例、checkfixture、公开 Runner fixture、debug reset 及清理器 | 删除旧能力的专属测试；共用 fixture 使用退役后的单一契约。把共用测试改成真实 Run 安全断言，不能删掉共用收尾反例后以绿灯交付 |
| 现行文档 | 协议、接入、执行配置、数据模型、生命周期、架构和测试入口 | 当前说明不再要求旧 Check；旧 Spec/Plan 标为退役并指向本 Spec。历史验收与原始证据保持原版本事实，不做全仓关键词抹除 |

此表是入口清单，实施时须沿调用关系补全实际消费者，不以清单外即无依赖为假设。

## 4. Runner 交接与本仓格式

Runner 旧能力由另一仓库退役，本任务不修改 Runner 源码或安装文件。涉及公开 wire 字段的实现与真实联验前，固定其移除后的公开契约快照，确认 capability/schema、service binding、takeover、verify-stopped 和相关版本；不自行假定对方字段、版本或停止范围。旧 Check 字段退役后，本仓不能仍要求 `checks: []` 才认可正常停机，也不能把原 Run 所需的字段改为可缺。

caller、fixture、示例只消费一份确认的新契约。不为旧 Runner 增加版本分支、旧字段兜底或检查命令占位。Runner 实际构建尚未交付时，可完成已确认契约的受控验证，但真实 EXE 对接项必须保留未验，不因旧构建含旧能力而推测新构建兼容。

当前工作树 SQLite 格式为34；删除旧表后更新到35。如果实施前基线版本变化，按原版本规则重新固定并记录，不并行使用两种新格式。不增加 ALTER 迁移、旧表读取、双写、归档导入或清库回退；旧格式继续明确拒绝。

原自有旧 Check 若尚未结算，应在原版本、原隔离现场正常收尾并独立保存证明；不得在新版本忽略/删除旧记录来宣称资源已停止。此项不授权读取、停服、归档或改写安装环境及他人现场。保留历史文件证据不等于在生产保留旧格式读路径。

## 5. 非目标与结构约束

- 不修 Runner 本机 Orca 执行分流，不扩通用 artifacts/workspace-git 或原工具回执协议；它们不是本任务的新检测前置。
- 不把旧 Check 的探针逐项迁入项目脚本，不改变项目准备原标准，不重新运行或修改安装环境。
- 不新增通用能力注册表、资源平台、结果状态机、旧历史归档器、兼容 UI 或“旧/新检测模式”开关。
- 本轮默认不改模型可见正文。如实现确需改提示词、接线、资产或相应测试/CI，遵循[提示词治理](../../docs/prompt-governance.md)，提供实际 diff、UTF-8 字节前后、业务原因、验证和独立审核；不能把更新黄金样本当批准。

## 6. 验收条件

| ID | 必须证明的结果 |
| --- | --- |
| LCR-A01 | 当前生产代码、公开契约、资产及活动文档无旧 Check 功能消费者；保留引用均有明确的历史/通用含义，无不可达旧实现、空壳字段或双轨 |
| LCR-A02 | 新建项目与多项目共享 Runner 的启动、准备占用及新候选预检不再读取旧 Check；新活动/候选冲突仍拒绝，未准备/失效仍零新候选 |
| LCR-A03 | 原 Run 的身份/摘要/实例、停止、重启、接管、pending stop 与结算失败/未知仍有效；新的停机证明不要求旧 Check 字段，缺失或不匹配的 Run 证明仍拒绝 |
| LCR-A04 | 旧 HTTP/命令/Web/Query 入口退出；新 `/environment` 页面、材料、结果、停止、任务和派发 blocker 正常；关闭/轮询不发新执行 |
| LCR-A05 | 新格式不创建旧两表，无旧方法/reset 查询；旧格式明确拒绝，无迁移或数据清除；reset 仍只允许已核实的自有 Run 资源 |
| LCR-A06 | 公开受控 Runner fixture 不发布旧能力和旧字段时，固定 L1/L2、相关 owner、共享生命周期/恢复及项目准备 L3 通过，且非零命中、清理完整 |
| LCR-A07 | 固定退役后的真实独立 Runner EXE，核实版本/摘要/公开契约及空闲服务正常启停/结算；不启动 Agent、不操作安装 home；缺构建时明确未验 |
| LCR-A08 | 独立复审确认删除范围、共同安全校验、未提交新功能保留和无兼容债；文档/索引/测试登记一致，未将旧原生证据记成本次通过 |

实际步骤、状态与证据归[专属验收](../testing/acceptance/legacy-environment-check-retirement.md)。本任务不以三条原生环境均 ready 为完成条件；原三条正向 L4 的缺口仍归原 readiness 验收，不能借退役更改其结果。
